package diagnostics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
				hostPath = filepath.Join(e.ConfigDir, hostPath)
			}

			// Clean path
			hostPath = filepath.Clean(hostPath)

			info, err := os.Stat(hostPath)
			if os.IsNotExist(err) {
				volumeCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Volume source missing: %s", vol.Source),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Bind-mount host path '%s' for service %s does not exist. Docker auto-creation can lead to write permission lockouts (root ownership).", hostPath, svcName),
					Mitigation: fmt.Sprintf("Run: mkdir -p %s && chmod -R 775 %s", hostPath, hostPath),
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

	if volumeCheckPassed {
		results = append(results, output.CheckResult{
			Group:  "Volume & File Permissions",
			Name:   "Volume & File Permissions Check",
			Status: output.CheckPassed,
		})
	}

	return results
}
