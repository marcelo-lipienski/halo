package diagnostics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/marcelo-lipienski/halo/output"
)

func isReadable(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	if info.IsDir() {
		// Attempt to read the directory names (at least 1)
		f, err := os.Open(path)
		if err != nil {
			return false, err
		}
		defer f.Close()
		_, err = f.Readdirnames(1)
		if err != nil && err.Error() != "EOF" {
			return false, err
		}
		return true, nil
	}

	// For files, attempt to open for reading
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	f.Close()
	return true, nil
}

func isWritable(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	if info.IsDir() {
		// Attempt to create a temp file inside the directory
		tempFile, err := os.CreateTemp(path, ".halo_write_test_*")
		if err != nil {
			return false, err
		}
		tempFile.Close()
		os.Remove(tempFile.Name())
		return true, nil
	}

	// For files, attempt to open with write permission
	f, err := os.OpenFile(path, os.O_WRONLY, 0666)
	if err != nil {
		return false, err
	}
	f.Close()
	return true, nil
}

func (e *Engine) checkVolumeAndPermissions(ctx context.Context) []output.CheckResult {
	var results []output.CheckResult

	if err := ctx.Err(); err != nil {
		results = append(results, output.CheckResult{
			Group:      "Volume & File Permissions",
			Name:       "Check Timeout",
			Status:     output.CheckFailed,
			Error:      fmt.Sprintf("Volume and file permissions check was cancelled: %v", err),
			Mitigation: "Verify host performance and disk storage health.",
		})
		return results
	}

	volumeCheckPassed := true

	// 1. Volumes Check
	for svcName, svc := range e.Compose.Services {
		for _, vol := range svc.Volumes {
			select {
			case <-ctx.Done():
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       "Check Timeout",
					Status:     output.CheckFailed,
					Error:      "Volume and file permissions check timed out",
					Mitigation: "Verify host performance and disk storage health.",
				})
				return results
			default:
			}

			if vol.Type != "bind" {
				continue
			}

			// Cross-platform OS path conventions warning
			isWindowsPath := false
			if len(vol.Source) >= 2 {
				drive := vol.Source[0]
				isLetter := (drive >= 'a' && drive <= 'z') || (drive >= 'A' && drive <= 'Z')
				if (isLetter && vol.Source[1] == ':') || strings.Contains(vol.Source, "\\") {
					isWindowsPath = true
				}
			}

			if runtime.GOOS != "windows" && isWindowsPath {
				volumeCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Incompatible OS Path: %s", vol.Source),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Bind-mount host path '%s' for service %s uses Windows path conventions on a non-Windows OS.", vol.Source, svcName),
					Mitigation: "Convert the path mapping in docker-compose.yml to use relative Unix paths (e.g., ./data instead of C:\\data).",
				})
				continue
			}

			resolvedSource := e.resolveEnvVars(vol.Source)
			if resolvedSource == "" {
				continue
			}

			hostPath := resolvedSource
			if strings.HasPrefix(hostPath, "~") {
				home, err := os.UserHomeDir()
				if err == nil {
					if hostPath == "~" {
						hostPath = home
					} else if strings.HasPrefix(hostPath, "~/") {
						hostPath = filepath.Join(home, hostPath[2:])
					} else if strings.HasPrefix(hostPath, "~\\") {
						hostPath = filepath.Join(home, hostPath[2:])
					}
				}
			}
			if !filepath.IsAbs(hostPath) {
				baseDir := vol.BaseDir
				if baseDir == "" {
					baseDir = e.ConfigDir
				}
				hostPath = filepath.Join(baseDir, hostPath)
			}

			// Clean path
			hostPath = filepath.Clean(hostPath)

			info, err := os.Stat(hostPath)
			if os.IsNotExist(err) {
				if e.AutoFix {
					// Check if the source is likely a file
					base := filepath.Base(hostPath)
					ext := filepath.Ext(hostPath)
					isLikelyFile := ext != "" || base == ".env" || base == ".gitignore" || base == "Dockerfile"
					var fixErr error
					if isLikelyFile {
						dir := filepath.Dir(hostPath)
						_ = os.MkdirAll(dir, 0755)
						fixErr = os.WriteFile(hostPath, []byte{}, 0664)
					} else {
						fixErr = os.MkdirAll(hostPath, 0775)
					}
					if fixErr == nil {
						// Auto-fixed, check again
						info, err = os.Stat(hostPath)
						if err == nil {
							results = append(results, output.CheckResult{
								Group:  "Volume & File Permissions",
								Name:   fmt.Sprintf("Volume source auto-fixed: %s", vol.Source),
								Status: output.CheckPassed,
							})
							goto checkRead
						}
					}
				}

				volumeCheckPassed = false

				// Check if the source is likely a file
				base := filepath.Base(hostPath)
				ext := filepath.Ext(hostPath)
				isLikelyFile := ext != "" || base == ".env" || base == ".gitignore" || base == "Dockerfile"

				mitigation := fmt.Sprintf("Run: mkdir -p %s && chmod -R 775 %s", hostPath, hostPath)
				if isLikelyFile {
					mitigation = fmt.Sprintf("Run: touch %s && chmod 664 %s", hostPath, hostPath)
				}

				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Volume source missing: %s", vol.Source),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Bind-mount host path '%s' for service %s does not exist. Docker auto-creation can lead to write permission lockouts (root ownership).", hostPath, svcName),
					Mitigation: mitigation,
				})
				continue
			} else if err != nil {
				volumeCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Volume access error: %s", vol.Source),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Failed to inspect volume path '%s': %v", hostPath, err),
					Mitigation: fmt.Sprintf("Verify host permissions for path: %s", hostPath),
				})
				continue
			}

		checkRead:
			readable, rErr := isReadable(hostPath)
			if !readable || rErr != nil {
				if e.AutoFix {
					var chmodErr error
					if info.IsDir() {
						chmodErr = os.Chmod(hostPath, 0755)
					} else {
						chmodErr = os.Chmod(hostPath, 0644)
					}
					if chmodErr == nil {
						if r, _ := isReadable(hostPath); r {
							results = append(results, output.CheckResult{
								Group:  "Volume & File Permissions",
								Name:   fmt.Sprintf("Volume read lockout auto-fixed: %s", vol.Source),
								Status: output.CheckPassed,
							})
							goto checkWrite
						}
					}
				}

				volumeCheckPassed = false
				pathType := "Directory"
				if !info.IsDir() {
					pathType = "File"
				}
				errStr := fmt.Sprintf("%s '%s' for service %s is not readable by current host user.", pathType, hostPath, svcName)
				if rErr != nil {
					errStr = fmt.Sprintf("%s '%s' for service %s is not readable by current host user. System error: %v", pathType, hostPath, svcName, rErr)
				}
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Volume read lockout: %s", vol.Source),
					Status:     output.CheckFailed,
					Error:      errStr,
					Mitigation: fmt.Sprintf("Run: chmod -R u+r %s or sudo chown -R $USER %s", hostPath, hostPath),
				})
			}

		checkWrite:
			// Only validate write permissions if the volume is NOT read-only
			if !vol.ReadOnly {
				writable, wErr := isWritable(hostPath)
				if !writable || wErr != nil {
					if e.AutoFix {
						var chmodErr error
						if info.IsDir() {
							chmodErr = os.Chmod(hostPath, 0755)
						} else {
							chmodErr = os.Chmod(hostPath, 0644)
						}
						if chmodErr == nil {
							if w, _ := isWritable(hostPath); w {
								results = append(results, output.CheckResult{
									Group:  "Volume & File Permissions",
									Name:   fmt.Sprintf("Volume write lockout auto-fixed: %s", vol.Source),
									Status: output.CheckPassed,
								})
								continue
							}
						}
					}

					volumeCheckPassed = false
					pathType := "Directory"
					if !info.IsDir() {
						pathType = "File"
					}
					errStr := fmt.Sprintf("%s '%s' for service %s is not writable by current host user.", pathType, hostPath, svcName)
					if wErr != nil {
						errStr = fmt.Sprintf("%s '%s' for service %s is not writable by current host user. System error: %v", pathType, hostPath, svcName, wErr)
					}
					results = append(results, output.CheckResult{
						Group:      "Volume & File Permissions",
						Name:       fmt.Sprintf("Volume permission lockout: %s", vol.Source),
						Status:     output.CheckFailed,
						Error:      errStr,
						Mitigation: fmt.Sprintf("Run: chmod -R u+rw %s or sudo chown -R $USER %s", hostPath, hostPath),
					})
				}
			}
		}
	}

	// 2. Secrets Check
	for secName, sec := range e.Compose.Secrets {
		select {
		case <-ctx.Done():
			results = append(results, output.CheckResult{
				Group:      "Volume & File Permissions",
				Name:       "Check Timeout",
				Status:     output.CheckFailed,
				Error:      "Secrets check timed out",
				Mitigation: "Verify host performance and disk storage health.",
			})
			return results
		default:
		}

		if sec.File == "" {
			continue
		}
		if isExternal(sec.External) {
			continue
		}

		resolvedFile := e.resolveEnvVars(sec.File)
		if resolvedFile == "" {
			continue
		}

		secretPath := resolvedFile
		if !filepath.IsAbs(secretPath) {
			baseDir := sec.BaseDir
			if baseDir == "" {
				baseDir = e.ConfigDir
			}
			secretPath = filepath.Join(baseDir, secretPath)
		}
		secretPath = filepath.Clean(secretPath)

		_, err := os.Stat(secretPath)
		if os.IsNotExist(err) {
			if e.AutoFix {
				dir := filepath.Dir(secretPath)
				_ = os.MkdirAll(dir, 0755)
				if writeErr := os.WriteFile(secretPath, []byte{}, 0600); writeErr == nil {
					results = append(results, output.CheckResult{
						Group:  "Volume & File Permissions",
						Name:   fmt.Sprintf("Secret file auto-created: %s", secName),
						Status: output.CheckPassed,
					})
					continue
				}
			}

			volumeCheckPassed = false
			results = append(results, output.CheckResult{
				Group:      "Volume & File Permissions",
				Name:       fmt.Sprintf("Secret file missing: %s", secName),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("File '%s' for secret '%s' does not exist.", secretPath, secName),
				Mitigation: fmt.Sprintf("Create the secret file: touch %s", secretPath),
			})
			continue
		} else if err != nil {
			volumeCheckPassed = false
			results = append(results, output.CheckResult{
				Group:      "Volume & File Permissions",
				Name:       fmt.Sprintf("Secret file access error: %s", secName),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("Failed to inspect secret path '%s': %v", secretPath, err),
				Mitigation: fmt.Sprintf("Verify permissions for path: %s", secretPath),
			})
			continue
		}

		// Verify read permission
		f, err := os.Open(secretPath)
		if err != nil {
			if e.AutoFix {
				if chmodErr := os.Chmod(secretPath, 0600); chmodErr == nil {
					results = append(results, output.CheckResult{
						Group:  "Volume & File Permissions",
						Name:   fmt.Sprintf("Secret permissions auto-fixed: %s", secName),
						Status: output.CheckPassed,
					})
					continue
				}
			}

			volumeCheckPassed = false
			results = append(results, output.CheckResult{
				Group:      "Volume & File Permissions",
				Name:       fmt.Sprintf("Secret read lockout: %s", secName),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("Secret file '%s' is not readable by current host user. System error: %v", secretPath, err),
				Mitigation: fmt.Sprintf("Run: chmod u+r %s or sudo chown $USER %s", secretPath, secretPath),
			})
		} else {
			f.Close()
		}
	}

	// 3. Configs Check
	for cfgName, cfg := range e.Compose.Configs {
		select {
		case <-ctx.Done():
			results = append(results, output.CheckResult{
				Group:      "Volume & File Permissions",
				Name:       "Check Timeout",
				Status:     output.CheckFailed,
				Error:      "Configs check timed out",
				Mitigation: "Verify host performance and disk storage health.",
			})
			return results
		default:
		}

		if cfg.File == "" {
			continue
		}
		if isExternal(cfg.External) {
			continue
		}

		resolvedFile := e.resolveEnvVars(cfg.File)
		if resolvedFile == "" {
			continue
		}

		cfgPath := resolvedFile
		if !filepath.IsAbs(cfgPath) {
			baseDir := cfg.BaseDir
			if baseDir == "" {
				baseDir = e.ConfigDir
			}
			cfgPath = filepath.Join(baseDir, cfgPath)
		}
		cfgPath = filepath.Clean(cfgPath)

		_, err := os.Stat(cfgPath)
		if os.IsNotExist(err) {
			if e.AutoFix {
				dir := filepath.Dir(cfgPath)
				_ = os.MkdirAll(dir, 0755)
				if writeErr := os.WriteFile(cfgPath, []byte{}, 0644); writeErr == nil {
					results = append(results, output.CheckResult{
						Group:  "Volume & File Permissions",
						Name:   fmt.Sprintf("Config file auto-created: %s", cfgName),
						Status: output.CheckPassed,
					})
					continue
				}
			}

			volumeCheckPassed = false
			results = append(results, output.CheckResult{
				Group:      "Volume & File Permissions",
				Name:       fmt.Sprintf("Config file missing: %s", cfgName),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("File '%s' for config '%s' does not exist.", cfgPath, cfgName),
				Mitigation: fmt.Sprintf("Create the config file: touch %s", cfgPath),
			})
			continue
		} else if err != nil {
			volumeCheckPassed = false
			results = append(results, output.CheckResult{
				Group:      "Volume & File Permissions",
				Name:       fmt.Sprintf("Config file access error: %s", cfgName),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("Failed to inspect config path '%s': %v", cfgPath, err),
				Mitigation: fmt.Sprintf("Verify permissions for path: %s", cfgPath),
			})
			continue
		}

		// Verify read permission
		f, err := os.Open(cfgPath)
		if err != nil {
			if e.AutoFix {
				if chmodErr := os.Chmod(cfgPath, 0644); chmodErr == nil {
					results = append(results, output.CheckResult{
						Group:  "Volume & File Permissions",
						Name:   fmt.Sprintf("Config permissions auto-fixed: %s", cfgName),
						Status: output.CheckPassed,
					})
					continue
				}
			}

			volumeCheckPassed = false
			results = append(results, output.CheckResult{
				Group:      "Volume & File Permissions",
				Name:       fmt.Sprintf("Config read lockout: %s", cfgName),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("Config file '%s' is not readable by current host user. System error: %v", cfgPath, err),
				Mitigation: fmt.Sprintf("Run: chmod u+r %s or sudo chown $USER %s", cfgPath, cfgPath),
			})
		} else {
			f.Close()
		}
	}

	if volumeCheckPassed {
		results = append(results, output.CheckResult{
			Group:  "Volume & File Permissions",
			Name:   "Volume & File Permissions Check",
			Status: output.CheckPassed,
		})
	}

	return results
}

func isExternal(ext interface{}) bool {
	if ext == nil {
		return false
	}
	switch v := ext.(type) {
	case bool:
		return v
	case string:
		return strings.ToLower(v) == "true"
	case map[string]interface{}:
		if val, ok := v["external"]; ok {
			if b, ok := val.(bool); ok {
				return b
			}
		}
		return true
	}
	return false
}
