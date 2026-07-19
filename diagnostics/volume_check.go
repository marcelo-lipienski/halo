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

	volumeCheckPassed := true
	for svcName, svc := range e.Compose.Services {
		for _, vol := range svc.Volumes {
			select {
			case <-ctx.Done():
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

			readable, rErr := isReadable(hostPath)
			if !readable || rErr != nil {
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

			// Only validate write permissions if the volume is NOT read-only
			if !vol.ReadOnly {
				writable, wErr := isWritable(hostPath)
				if !writable || wErr != nil {
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

	if volumeCheckPassed {
		results = append(results, output.CheckResult{
			Group:  "Volume & File Permissions",
			Name:   "Volume & File Permissions Check",
			Status: output.CheckPassed,
		})
	}

	return results
}
