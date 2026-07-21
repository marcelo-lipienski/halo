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

// CheckGitignoreSecurity audits local environment files to ensure they are git-ignored and not tracked.
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

	envFiles, err := findEnvFiles(e.ConfigDir)
	if err != nil {
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

	gitAvailable := isGitRepository(e.ConfigDir)

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

		// 1. Check if tracked (committed)
		tracked := false
		if gitAvailable {
			cmd := exec.CommandContext(ctx, "git", "ls-files", "--error-unmatch", path)
			cmd.Dir = e.ConfigDir
			if err := cmd.Run(); err == nil {
				tracked = true
			}
		}

		if tracked {
			results = append(results, output.CheckResult{
				Group:      "Security Audits",
				Name:       fmt.Sprintf("Tracked Env File: %s", relPath),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("Environment file '%s' is committed to the Git repository (tracked by Git)", relPath),
				Mitigation: fmt.Sprintf("Remove it from Git tracking without deleting it from disk: run 'git rm --cached %s' and commit the deletion.", relPath),
			})
			continue
		}

		// 2. Check if ignored
		ignored := false
		if gitAvailable {
			cmd := exec.CommandContext(ctx, "git", "check-ignore", "-q", path)
			cmd.Dir = e.ConfigDir
			if err := cmd.Run(); err == nil {
				ignored = true
			}
		} else {
			// Fallback custom ignore parser
			ignored, _ = isIgnoredCustom(path, e.ConfigDir)
		}

		if !ignored {
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

func findEnvFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
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
		if name == ".env.example" {
			return nil
		}
		if name == ".env" || strings.HasPrefix(name, ".env.") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func isGitRepository(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func isIgnoredCustom(filePath string, configDir string) (bool, error) {
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return false, err
	}
	absConfig, err := filepath.Abs(configDir)
	if err != nil {
		return false, err
	}

	// Traverse directories from filePath directory up to configDir
	currentDir := filepath.Dir(absFile)
	var gitignores []string

	for {
		ignoreFile := filepath.Join(currentDir, ".gitignore")
		if stat, err := os.Stat(ignoreFile); err == nil && !stat.IsDir() {
			gitignores = append([]string{ignoreFile}, gitignores...) // Prepend so parents are processed first
		}
		if currentDir == absConfig || currentDir == filepath.Dir(currentDir) {
			break
		}
		currentDir = filepath.Dir(currentDir)
	}

	ignored := false
	for _, ignorePath := range gitignores {
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
			line := scanner.Text()
			matches, err := matchGitignorePattern(line, relPath)
			if err == nil && matches {
				// Negation overrides ignore
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

	// Anchored patterns contain a slash (not at the end) or start with a slash
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
