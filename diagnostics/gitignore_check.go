package diagnostics

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/marcelo-lipienski/halo/output"
)

// CheckGitignoreSecurity audits env files to ensure they are git-ignored and not tracked.
func (e *Engine) CheckGitignoreSecurity(ctx context.Context) []output.CheckResult {
	var results []output.CheckResult

	if err := ctx.Err(); err != nil {
		results = append(results, output.CheckResult{
			Group:      "Security Audits",
			Name:       "Check Timeout",
			Status:     output.CheckFailed,
			Error:      fmt.Sprintf("Gitignore check was cancelled: %v", err),
			Mitigation: "Verify system resources and responsiveness.",
		})
		return results
	}

	envFiles, err := findEnvFiles(ctx, e.ConfigDir)
	if err != nil {
		if ctx.Err() != nil {
			results = append(results, output.CheckResult{
				Group:      "Security Audits",
				Name:       "Check Timeout",
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("Gitignore check was cancelled: %v", ctx.Err()),
				Mitigation: "Verify system resources and responsiveness.",
			})
			return results
		}
		results = append(results, output.CheckResult{
			Group:      "Security Audits",
			Name:       "Env Files Discovery",
			Status:     output.CheckWarning,
			Error:      fmt.Sprintf("Failed to scan for environment files: %v", err),
			Mitigation: "Check read permissions on the configuration directory.",
		})
		return results
	}

	if len(envFiles) == 0 {
		results = append(results, output.CheckResult{
			Group:  "Security Audits",
			Name:   "Gitignore Security Check",
			Status: output.CheckPassed,
		})
		return results
	}

	gitAvailable := isGitRepository(ctx, e.ConfigDir)

	for _, path := range envFiles {
		select {
		case <-ctx.Done():
			results = append(results, output.CheckResult{
				Group:      "Security Audits",
				Name:       "Check Timeout",
				Status:     output.CheckFailed,
				Error:      "Gitignore security check timed out",
				Mitigation: "Verify local environment speed and resources.",
			})
			return results
		default:
		}

		relPath, relErr := filepath.Rel(e.ConfigDir, path)
		if relErr != nil {
			relPath = path
		}

		// 1. Check if tracked.
		tracked := false
		if gitAvailable {
			cmd := exec.CommandContext(ctx, "git", "ls-files", "--error-unmatch", path)
			cmd.Dir = e.ConfigDir
			if err := cmd.Run(); err == nil {
				tracked = true
			}
		}

		if tracked {
			if e.DryRun {
				results = append(results, output.CheckResult{
					Group:      "Security Audits",
					Name:       fmt.Sprintf("Tracked Env File: %s", relPath),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("[Dry-Run] Would remove '%s' from Git tracking (git rm --cached %s)", relPath, relPath),
					Mitigation: fmt.Sprintf("Remove it from Git tracking without deleting it from disk: run 'git rm --cached %s' and commit the deletion.", relPath),
				})
				continue
			}

			shouldFix := e.AutoFix || e.Interactive
			confirmed := true
			if e.Interactive {
				confirmed = promptConfirm(fmt.Sprintf("Environment file '%s' is tracked by Git. Remove it from Git index (git rm --cached)?", relPath))
			}

			if shouldFix && confirmed {
				cmd := exec.CommandContext(ctx, "git", "rm", "--cached", path)
				cmd.Dir = e.ConfigDir
				if err := cmd.Run(); err == nil {
					checkCmd := exec.CommandContext(ctx, "git", "ls-files", "--error-unmatch", path)
					checkCmd.Dir = e.ConfigDir
					if err := checkCmd.Run(); err != nil {
						results = append(results, output.CheckResult{
							Group:  "Security Audits",
							Name:   fmt.Sprintf("Tracked Env File auto-fixed: %s", relPath),
							Status: output.CheckPassed,
						})
						continue
					}
				}
			}

			results = append(results, output.CheckResult{
				Group:      "Security Audits",
				Name:       fmt.Sprintf("Tracked Env File: %s", relPath),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("Environment file '%s' is committed to the Git repository (tracked by Git)", relPath),
				Mitigation: fmt.Sprintf("Remove it from Git tracking without deleting it from disk: run 'git rm --cached %s' and commit the deletion.", relPath),
			})
			continue
		}

		// 2. Check if ignored.
		ignored := false
		if gitAvailable {
			cmd := exec.CommandContext(ctx, "git", "check-ignore", "-q", path)
			cmd.Dir = e.ConfigDir
			if err := cmd.Run(); err == nil {
				ignored = true
			}
		} else {
			// Fallback custom ignore parser.
			ignored, _ = isIgnoredCustom(ctx, path, e.ConfigDir)
		}

		if !ignored {
			relPath, relErr := filepath.Rel(e.ConfigDir, path)
			if relErr != nil {
				relPath = path
			}

			gitignorePath := filepath.Join(e.ConfigDir, ".gitignore")

			if e.DryRun {
				results = append(results, output.CheckResult{
					Group:      "Security Audits",
					Name:       fmt.Sprintf("Unignored Env File: %s", relPath),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("[Dry-Run] Would add '%s' to %s", relPath, gitignorePath),
					Mitigation: fmt.Sprintf("Add '%s' to your '.gitignore' file to prevent it from being accidentally committed.", relPath),
				})
				continue
			}

			shouldFix := e.AutoFix || e.Interactive
			confirmed := true
			if e.Interactive {
				confirmed = promptConfirm(fmt.Sprintf("Environment file '%s' is not ignored by Git. Add it to .gitignore?", relPath))
			}

			if shouldFix && confirmed {
				if err := appendToGitignore(gitignorePath, relPath); err == nil {
					reIgnored := false
					if gitAvailable {
						cmd := exec.CommandContext(ctx, "git", "check-ignore", "-q", path)
						cmd.Dir = e.ConfigDir
						if err := cmd.Run(); err == nil {
							reIgnored = true
						}
					} else {
						reIgnored, _ = isIgnoredCustom(ctx, path, e.ConfigDir)
					}
					if reIgnored {
						results = append(results, output.CheckResult{
							Group:  "Security Audits",
							Name:   fmt.Sprintf("Ignored Env File auto-fixed: %s", relPath),
							Status: output.CheckPassed,
						})
						continue
					}
				}
			}

			results = append(results, output.CheckResult{
				Group:      "Security Audits",
				Name:       fmt.Sprintf("Unignored Env File: %s", relPath),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("Environment file '%s' is not ignored by Git (missing from .gitignore)", relPath),
				Mitigation: fmt.Sprintf("Add '%s' to your '.gitignore' file to prevent it from being accidentally committed.", relPath),
			})
		} else {
			results = append(results, output.CheckResult{
				Group:  "Security Audits",
				Name:   fmt.Sprintf("Ignored Env File: %s", relPath),
				Status: output.CheckPassed,
			})
		}
	}

	return results
}

func appendToGitignore(gitignorePath, entry string) error {
	var fileExists bool
	if info, err := os.Stat(gitignorePath); err == nil && !info.IsDir() {
		fileExists = true
	}

	f, err := os.OpenFile(gitignorePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if fileExists {
		info, err := os.Stat(gitignorePath)
		if err == nil && info.Size() > 0 {
			rf, err := os.Open(gitignorePath)
			if err == nil {
				if _, errSeek := rf.Seek(-1, 2); errSeek == nil {
					b := make([]byte, 1)
					if _, errRead := rf.Read(b); errRead == nil && b[0] != '\n' {
						_, _ = f.WriteString("\n")
					}
				}
				_ = rf.Close()
			}
		}
	}

	_, err = f.WriteString(entry + "\n")
	return err
}

func findEnvFiles(ctx context.Context, dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".next" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		nameLower := strings.ToLower(name)
		if strings.Contains(nameLower, "example") || strings.Contains(nameLower, "sample") || strings.Contains(nameLower, "template") {
			return nil
		}
		if name == ".env" || strings.HasPrefix(name, ".env.") || strings.HasPrefix(name, ".env-") || strings.HasPrefix(name, ".env_") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func isGitRepository(ctx context.Context, dir string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func isIgnoredCustom(ctx context.Context, filePath string, configDir string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return false, err
	}
	absConfig, err := filepath.Abs(configDir)
	if err != nil {
		return false, err
	}

	// Traverse directories upwards to configDir.
	currentDir := filepath.Dir(absFile)
	var gitignores []string

	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		ignoreFile := filepath.Join(currentDir, ".gitignore")
		if stat, err := os.Stat(ignoreFile); err == nil && !stat.IsDir() {
			gitignores = append([]string{ignoreFile}, gitignores...) // Prepend parents first.
		}
		if currentDir == absConfig || currentDir == filepath.Dir(currentDir) {
			break
		}
		currentDir = filepath.Dir(currentDir)
	}

	ignored := false
	for _, ignorePath := range gitignores {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		file, err := os.Open(ignorePath)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(file)
		ignoreDir := filepath.Dir(ignorePath)

		relPath, err := filepath.Rel(ignoreDir, absFile)
		if err != nil {
			_ = file.Close()
			continue
		}
		relPath = filepath.ToSlash(relPath)

		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				_ = file.Close()
				return false, err
			}
			line := scanner.Text()
			matches, err := matchGitignorePattern(line, relPath)
			if err == nil && matches {
				if strings.HasPrefix(strings.TrimSpace(line), "!") {
					ignored = false
				} else {
					ignored = true
				}
			}
		}
		_ = file.Close()
	}

	return ignored, nil
}

func matchGitignorePattern(pattern string, relPath string) (bool, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || strings.HasPrefix(pattern, "#") {
		return false, nil
	}

	pattern = strings.TrimPrefix(pattern, "!")

	// Check if pattern is anchored.
	anchored := false
	if strings.HasPrefix(pattern, "/") {
		anchored = true
		pattern = pattern[1:]
	} else if strings.Contains(strings.TrimSuffix(pattern, "/"), "/") {
		anchored = true
	}

	var matched bool
	var err error

	if !anchored {
		base := filepath.Base(relPath)
		matched, err = filepath.Match(pattern, base)
		if err != nil {
			return false, err
		}
		if !matched {
			parts := strings.Split(relPath, "/")
			for _, part := range parts {
				matched, _ = filepath.Match(pattern, part)
				if matched {
					break
				}
			}
		}
	} else {
		matched, err = matchGlob(pattern, relPath)
		if err != nil {
			return false, err
		}
	}

	return matched, nil
}

func matchGlob(pattern string, target string) (bool, error) {
	var regexBuf strings.Builder
	regexBuf.WriteString("^")

	i := 0
	n := len(pattern)
	for i < n {
		c := pattern[i]
		switch c {
		case '.':
			regexBuf.WriteString(`\.`)
			i++
		case '*':
			if i+1 < n && pattern[i+1] == '*' {
				if i+2 < n && pattern[i+2] == '/' {
					regexBuf.WriteString(`(?:.*/)?`)
					i += 3
				} else {
					regexBuf.WriteString(`.*`)
					i += 2
				}
			} else {
				regexBuf.WriteString(`[^/]*`)
				i++
			}
		case '?':
			regexBuf.WriteString(`[^/]`)
			i++
		case '\\':
			if i+1 < n {
				regexBuf.WriteString(regexp.QuoteMeta(string(pattern[i+1])))
				i += 2
			} else {
				regexBuf.WriteByte('\\')
				i++
			}
		default:
			regexBuf.WriteString(regexp.QuoteMeta(string(c)))
			i++
		}
	}
	regexBuf.WriteString("$")
	re, err := regexp.Compile(regexBuf.String())
	if err != nil {
		return false, err
	}
	return re.MatchString(target), nil
}
