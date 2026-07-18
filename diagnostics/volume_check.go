package diagnostics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/marcelo-lipienski/halo/output"
)

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
			if vol.Type != "bind" {
				continue
			}

			resolvedSource := e.resolveEnvVars(vol.Source)
			if resolvedSource == "" {
				continue
			}

			hostPath := resolvedSource
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
					Mitigation: fmt.Sprintf("Run: mkdir -p %s && chmod -R 775 %s", vol.Source, vol.Source),
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

			writable, wErr := isWritable(hostPath)
			if !writable || wErr != nil {
				volumeCheckPassed = false
				pathType := "Directory"
				if !info.IsDir() {
					pathType = "File"
				}
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Volume permission lockout: %s", vol.Source),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("%s '%s' for service %s is not writable by current host user.", pathType, hostPath, svcName),
					Mitigation: fmt.Sprintf("Run: chmod -R u+rw %s or sudo chown -R $USER %s", vol.Source, vol.Source),
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
