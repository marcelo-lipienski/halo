package diagnostics

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/marcelo-lipienski/halo/output"
)

var promptMutex sync.Mutex

var promptConfirm = func(question string) bool {
	promptMutex.Lock()
	defer promptMutex.Unlock()

	fmt.Fprintf(os.Stderr, "%s [y/N]: ", question)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

func isLikelyFilePath(path string) bool {
	base := filepath.Base(path)
	ext := filepath.Ext(path)
	return ext != "" || base == ".env" || base == ".gitignore" || base == "Dockerfile"
}

func isReadable(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	if info.IsDir() {
		// Read directory contents.
		f, err := os.Open(path)
		if err != nil {
			return false, err
		}
		defer func() { _ = f.Close() }()
		_, err = f.Readdirnames(1)
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}
		return true, nil
	}

	// Open file for reading.
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	_ = f.Close()
	return true, nil
}

func isWritable(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	if info.IsDir() {
		// Create temporary file.
		tempFile, err := os.CreateTemp(path, ".halo_write_test_*")
		if err != nil {
			return false, err
		}
		defer func() { _ = os.Remove(tempFile.Name()) }()
		_ = tempFile.Close()
		return true, nil
	}

	// Open file for writing.
	f, err := os.OpenFile(path, os.O_WRONLY, 0666)
	if err != nil {
		return false, err
	}
	_ = f.Close()
	return true, nil
}

// checkReadPermission verifies read access and optionally auto-fixes it. See ADR-0005.
func (e *Engine) checkReadPermission(results []output.CheckResult, hostPath, volSource, svcName string, info os.FileInfo) ([]output.CheckResult, bool) {
	readable, rErr := isReadable(hostPath)
	if readable && rErr == nil {
		return results, true
	}

	pathType := "Directory"
	if !info.IsDir() {
		pathType = "File"
	}

	if e.DryRun {
		originalMode := info.Mode()
		newMode := os.FileMode(0755)
		if !info.IsDir() {
			newMode = 0644
		}
		results = append(results, output.CheckResult{
			Group:      "Volume & File Permissions",
			Name:       fmt.Sprintf("Volume read lockout: %s", volSource),
			Status:     output.CheckFailed,
			Error:      fmt.Sprintf("[Dry-Run] Would apply chmod %s (original: %s) to %s '%s'", newMode, originalMode, pathType, hostPath),
			Mitigation: fmt.Sprintf("Run: chmod -R u+r %s or sudo chown -R $USER %s", hostPath, hostPath),
		})
		return results, false
	}

	shouldFix := e.AutoFix || e.Interactive
	confirmed := true
	if e.Interactive {
		newMode := os.FileMode(0755)
		if !info.IsDir() {
			newMode = 0644
		}
		confirmed = promptConfirm(fmt.Sprintf("%s '%s' is not readable. Apply permissions (chmod %s, original: %s)?", pathType, hostPath, newMode, info.Mode()))
	}

	if shouldFix && confirmed {
		originalMode := info.Mode()
		newMode := os.FileMode(0755)
		if !info.IsDir() {
			newMode = 0644
		}
		if chmodErr := fixPermissions(hostPath, newMode); chmodErr == nil {
			// Re-verify readability. See ADR-0005.
			if r, reVerifyErr := isReadable(hostPath); r && reVerifyErr == nil {
				results = append(results, output.CheckResult{
					Group:  "Volume & File Permissions",
					Name:   fmt.Sprintf("Volume read lockout auto-fixed: %s", volSource),
					Status: output.CheckPassed,
					Error:  fmt.Sprintf("Original permissions: %s, applied: %s", originalMode, newMode),
				})
				return results, true
			} else {
				// Update error if readability still fails. See ADR-0005.
				rErr = reVerifyErr
			}
		}
	}

	errStr := fmt.Sprintf("%s '%s' for service %s is not readable by current host user.", pathType, hostPath, svcName)
	if rErr != nil {
		errStr = fmt.Sprintf("%s '%s' for service %s is not readable by current host user. System error: %v", pathType, hostPath, svcName, rErr)
	}
	results = append(results, output.CheckResult{
		Group:      "Volume & File Permissions",
		Name:       fmt.Sprintf("Volume read lockout: %s", volSource),
		Status:     output.CheckFailed,
		Error:      errStr,
		Mitigation: getPermissionMitigation(hostPath, false, info.IsDir()),
	})
	return results, false
}

// checkWritePermission verifies write access and optionally auto-fixes it. See ADR-0005 and ADR-0015.
func (e *Engine) checkWritePermission(results []output.CheckResult, hostPath, volSource, svcName string, info os.FileInfo) ([]output.CheckResult, bool) {
	writable, wErr := isWritable(hostPath)
	if writable && wErr == nil {
		return results, true
	}

	pathType := "Directory"
	if !info.IsDir() {
		pathType = "File"
	}

	if e.DryRun {
		results = append(results, output.CheckResult{
			Group:      "Volume & File Permissions",
			Name:       fmt.Sprintf("Volume permission lockout: %s", volSource),
			Status:     output.CheckFailed,
			Error:      fmt.Sprintf("[Dry-Run] Would apply permissions to %s '%s'", pathType, hostPath),
			Mitigation: getPermissionMitigation(hostPath, true, info.IsDir()),
		})
		return results, false
	}

	shouldFix := e.AutoFix || e.Interactive
	confirmed := true
	if e.Interactive {
		mode := os.FileMode(0755)
		if !info.IsDir() {
			mode = 0644
		}
		confirmed = promptConfirm(fmt.Sprintf("%s '%s' is not writable. Apply permissions (chmod %s, original: %s)?", pathType, hostPath, mode, info.Mode()))
	}

	if shouldFix && confirmed {
		mode := os.FileMode(0755)
		if !info.IsDir() {
			mode = 0644
		}
		if chmodErr := fixPermissions(hostPath, mode); chmodErr == nil {
			if w, _ := isWritable(hostPath); w {
				results = append(results, output.CheckResult{
					Group:  "Volume & File Permissions",
					Name:   fmt.Sprintf("Volume write lockout auto-fixed: %s", volSource),
					Status: output.CheckPassed,
				})
				return results, true
			}
		}
	}

	errStr := fmt.Sprintf("%s '%s' for service %s is not writable by current host user.", pathType, hostPath, svcName)
	if wErr != nil {
		errStr = fmt.Sprintf("%s '%s' for service %s is not writable by current host user. System error: %v", pathType, hostPath, svcName, wErr)
	}
	results = append(results, output.CheckResult{
		Group:      "Volume & File Permissions",
		Name:       fmt.Sprintf("Volume permission lockout: %s", volSource),
		Status:     output.CheckFailed,
		Error:      errStr,
		Mitigation: getPermissionMitigation(hostPath, true, info.IsDir()),
	})
	return results, false
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

	// 1. Volumes check
	var svcNames []string
	if e.Compose != nil {
		for name := range e.Compose.Services {
			svcNames = append(svcNames, name)
		}
		sort.Strings(svcNames)
	}

	for _, svcName := range svcNames {
		svc := e.Compose.Services[svcName]
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

			// Cross-platform path convention check.
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

			// Clean path.
			hostPath = filepath.Clean(hostPath)

			info, err := os.Stat(hostPath)
			if os.IsNotExist(err) {
				if e.DryRun {
					volumeCheckPassed = false
					isLikelyFile := isLikelyFilePath(hostPath)
					pathTypeStr := "directory"
					permissions := "0775"
					mitigation := fmt.Sprintf("Run: mkdir -p %s && chmod -R 775 %s", hostPath, hostPath)
					if isLikelyFile {
						pathTypeStr = "file"
						permissions = "0664"
						mitigation = fmt.Sprintf("Run: touch %s && chmod 664 %s", hostPath, hostPath)
					}
					results = append(results, output.CheckResult{
						Group:      "Volume & File Permissions",
						Name:       fmt.Sprintf("Volume source missing: %s", vol.Source),
						Status:     output.CheckFailed,
						Error:      fmt.Sprintf("[Dry-Run] Would create missing %s '%s' (permissions: %s)", pathTypeStr, hostPath, permissions),
						Mitigation: mitigation,
					})
					continue
				}
				shouldFix := e.AutoFix || e.Interactive
				confirmed := true
				if e.Interactive {
					pathTypeStr := "directory"
					if isLikelyFilePath(hostPath) {
						pathTypeStr = "file"
					}
					confirmed = promptConfirm(fmt.Sprintf("Volume source %s '%s' is missing. Create it?", pathTypeStr, hostPath))
				}

				if shouldFix && confirmed {
					isLikelyFile := isLikelyFilePath(hostPath)
					var fixErr error
					if isLikelyFile {
						dir := filepath.Dir(hostPath)
						_ = os.MkdirAll(dir, 0755)
						fixErr = os.WriteFile(hostPath, []byte{}, 0664)
					} else {
						fixErr = os.MkdirAll(hostPath, 0775)
					}
					if fixErr == nil {
						if info, err = os.Stat(hostPath); err == nil {
							results = append(results, output.CheckResult{
								Group:  "Volume & File Permissions",
								Name:   fmt.Sprintf("Volume source auto-fixed: %s", vol.Source),
								Status: output.CheckPassed,
							})
							var readable bool
							results, readable = e.checkReadPermission(results, hostPath, vol.Source, svcName, info)
							if readable && !vol.ReadOnly {
								var writable bool
								results, writable = e.checkWritePermission(results, hostPath, vol.Source, svcName, info)
								if !writable {
									volumeCheckPassed = false
								}
							} else if !readable {
								volumeCheckPassed = false
							}
							continue
						}
					}
				}

				volumeCheckPassed = false

				isLikelyFile := isLikelyFilePath(hostPath)

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

			var readable bool
			results, readable = e.checkReadPermission(results, hostPath, vol.Source, svcName, info)
			if !readable {
				volumeCheckPassed = false
			}
			if readable && !vol.ReadOnly {
				var writable bool
				results, writable = e.checkWritePermission(results, hostPath, vol.Source, svcName, info)
				if !writable {
					volumeCheckPassed = false
				}
			}
		}
	}

	// 2. Secrets check
	var secNames []string
	if e.Compose != nil {
		for name := range e.Compose.Secrets {
			secNames = append(secNames, name)
		}
		sort.Strings(secNames)
	}

	for _, secName := range secNames {
		sec := e.Compose.Secrets[secName]
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
			if e.DryRun {
				volumeCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Secret file missing: %s", secName),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("[Dry-Run] Would create missing secret file '%s' (permissions: 0600)", secretPath),
					Mitigation: fmt.Sprintf("Run: touch %s", secretPath),
				})
				continue
			}
			shouldFix := e.AutoFix || e.Interactive
			confirmed := true
			if e.Interactive {
				confirmed = promptConfirm(fmt.Sprintf("Secret file for secret '%s' is missing. Create it at '%s'?", secName, secretPath))
			}

			if shouldFix && confirmed {
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

		// Verify read permission.
		f, err := os.Open(secretPath)
		if err != nil {
			if e.DryRun {
				volumeCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Secret read lockout: %s", secName),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("[Dry-Run] Would apply permissions to secret file '%s'", secretPath),
					Mitigation: getPermissionMitigation(secretPath, false, false),
				})
				continue
			}
			shouldFix := e.AutoFix || e.Interactive
			confirmed := true
			if e.Interactive {
				confirmed = promptConfirm(fmt.Sprintf("Secret file for secret '%s' is not readable. Apply permissions (chmod 0600)?", secName))
			}

			if shouldFix && confirmed {
				if chmodErr := fixPermissions(secretPath, 0600); chmodErr == nil {
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
				Mitigation: getPermissionMitigation(secretPath, false, false),
			})
		} else {
			_ = f.Close()
		}
	}

	// 3. Configs check
	var cfgNames []string
	if e.Compose != nil {
		for name := range e.Compose.Configs {
			cfgNames = append(cfgNames, name)
		}
		sort.Strings(cfgNames)
	}

	for _, cfgName := range cfgNames {
		cfg := e.Compose.Configs[cfgName]
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
			if e.DryRun {
				volumeCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Config file missing: %s", cfgName),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("[Dry-Run] Would create missing config file '%s' (permissions: 0644)", cfgPath),
					Mitigation: fmt.Sprintf("Run: touch %s", cfgPath),
				})
				continue
			}
			shouldFix := e.AutoFix || e.Interactive
			confirmed := true
			if e.Interactive {
				confirmed = promptConfirm(fmt.Sprintf("Config file for config '%s' is missing. Create it at '%s'?", cfgName, cfgPath))
			}

			if shouldFix && confirmed {
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

		// Verify read permission.
		f, err := os.Open(cfgPath)
		if err != nil {
			if e.DryRun {
				volumeCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Config read lockout: %s", cfgName),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("[Dry-Run] Would apply permissions to config file '%s'", cfgPath),
					Mitigation: getPermissionMitigation(cfgPath, false, false),
				})
				continue
			}
			shouldFix := e.AutoFix || e.Interactive
			confirmed := true
			if e.Interactive {
				confirmed = promptConfirm(fmt.Sprintf("Config file for config '%s' is not readable. Apply permissions (chmod 0644)?", cfgName))
			}

			if shouldFix && confirmed {
				if chmodErr := fixPermissions(cfgPath, 0644); chmodErr == nil {
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
				Mitigation: getPermissionMitigation(cfgPath, false, false),
			})
		} else {
			_ = f.Close()
		}
	}

	// 4. EnvFiles check
	for _, svcName := range svcNames {
		svc := e.Compose.Services[svcName]
		for _, ef := range svc.EnvFiles {
			select {
			case <-ctx.Done():
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       "Check Timeout",
					Status:     output.CheckFailed,
					Error:      "Env file check timed out",
					Mitigation: "Verify host performance and disk storage health.",
				})
				return results
			default:
			}

			resolvedFile := e.resolveEnvVars(ef.File)
			if resolvedFile == "" {
				continue
			}

			envFilePath := resolvedFile
			if !filepath.IsAbs(envFilePath) {
				baseDir := ef.BaseDir
				if baseDir == "" {
					baseDir = e.ConfigDir
				}
				envFilePath = filepath.Join(baseDir, envFilePath)
			}
			envFilePath = filepath.Clean(envFilePath)

			_, err := os.Stat(envFilePath)
			if os.IsNotExist(err) {
				if !ef.Required {
					// Warn on missing optional env file.
					results = append(results, output.CheckResult{
						Group:      "Volume & File Permissions",
						Name:       fmt.Sprintf("Optional env file missing: %s", ef.File),
						Status:     output.CheckWarning,
						Error:      fmt.Sprintf("Optional env_file '%s' for service %s does not exist.", envFilePath, svcName),
						Mitigation: fmt.Sprintf("Create the file if needed: touch %s", envFilePath),
					})
					continue
				}

				if e.DryRun {
					volumeCheckPassed = false
					results = append(results, output.CheckResult{
						Group:      "Volume & File Permissions",
						Name:       fmt.Sprintf("Env file missing: %s", ef.File),
						Status:     output.CheckFailed,
						Error:      fmt.Sprintf("[Dry-Run] Would create missing env file '%s' (permissions: 0644)", envFilePath),
						Mitigation: fmt.Sprintf("Run: touch %s", envFilePath),
					})
					continue
				}

				shouldFix := e.AutoFix || e.Interactive
				confirmed := true
				if e.Interactive {
					confirmed = promptConfirm(fmt.Sprintf("Env file '%s' for service %s is missing. Create it at '%s'?", ef.File, svcName, envFilePath))
				}

				if shouldFix && confirmed {
					dir := filepath.Dir(envFilePath)
					_ = os.MkdirAll(dir, 0755)
					if writeErr := os.WriteFile(envFilePath, []byte{}, 0644); writeErr == nil {
						results = append(results, output.CheckResult{
							Group:  "Volume & File Permissions",
							Name:   fmt.Sprintf("Env file auto-created: %s", ef.File),
							Status: output.CheckPassed,
						})
						continue
					}
				}

				volumeCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Env file missing: %s", ef.File),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("File '%s' for env_file in service %s does not exist.", envFilePath, svcName),
					Mitigation: fmt.Sprintf("Create the env file: touch %s", envFilePath),
				})
				continue
			} else if err != nil {
				volumeCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Env file access error: %s", ef.File),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Failed to inspect env_file path '%s': %v", envFilePath, err),
					Mitigation: fmt.Sprintf("Verify permissions for path: %s", envFilePath),
				})
				continue
			}

			// Verify read permission.
			f, err := os.Open(envFilePath)
			if err != nil {
				if e.DryRun {
					volumeCheckPassed = false
					results = append(results, output.CheckResult{
						Group:      "Volume & File Permissions",
						Name:       fmt.Sprintf("Env file read lockout: %s", ef.File),
						Status:     output.CheckFailed,
						Error:      fmt.Sprintf("[Dry-Run] Would apply permissions to env file '%s'", envFilePath),
						Mitigation: getPermissionMitigation(envFilePath, false, false),
					})
					continue
				}
				shouldFix := e.AutoFix || e.Interactive
				confirmed := true
				if e.Interactive {
					confirmed = promptConfirm(fmt.Sprintf("Env file '%s' for service %s is not readable. Apply permissions (chmod 0644)?", ef.File, svcName))
				}

				if shouldFix && confirmed {
					if chmodErr := fixPermissions(envFilePath, 0644); chmodErr == nil {
						results = append(results, output.CheckResult{
							Group:  "Volume & File Permissions",
							Name:   fmt.Sprintf("Env file permissions auto-fixed: %s", ef.File),
							Status: output.CheckPassed,
						})
						continue
					}
				}

				volumeCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Env file read lockout: %s", ef.File),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Env file '%s' is not readable by current host user. System error: %v", envFilePath, err),
					Mitigation: getPermissionMitigation(envFilePath, false, false),
				})
			} else {
				_ = f.Close()
			}
		}
	}

	// 5. Service secrets and configs mapping check
	for _, svcName := range svcNames {
		svc := e.Compose.Services[svcName]

		for _, s := range svc.Secrets {
			if _, exists := e.Compose.Secrets[s.Source]; !exists {
				volumeCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Service %s secret missing: %s", svcName, s.Source),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Service %s references secret '%s' which is not defined in the top-level secrets block", svcName, s.Source),
					Mitigation: fmt.Sprintf("Define secret '%s' in the root-level secrets block of your docker-compose.yml", s.Source),
				})
			}
		}

		for _, c := range svc.Configs {
			if _, exists := e.Compose.Configs[c.Source]; !exists {
				volumeCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Volume & File Permissions",
					Name:       fmt.Sprintf("Service %s config missing: %s", svcName, c.Source),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Service %s references config '%s' which is not defined in the top-level configs block", svcName, c.Source),
					Mitigation: fmt.Sprintf("Define config '%s' in the root-level configs block of your docker-compose.yml", c.Source),
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

var fixPermissionsFunc = func(path string, perm os.FileMode) error {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("icacls", path, "/grant", "Users:M")
		return cmd.Run()
	}
	return os.Chmod(path, perm)
}

func fixPermissions(path string, perm os.FileMode) error {
	return fixPermissionsFunc(path, perm)
}

func getPermissionMitigation(path string, isWrite bool, isDir bool) string {
	if runtime.GOOS == "windows" {
		perm := "R"
		if isWrite {
			perm = "M"
		}
		return fmt.Sprintf("Run: icacls %q /grant Users:%s", path, perm)
	}
	cmd := "u+r"
	if isWrite {
		cmd = "u+rwx"
	} else if isDir {
		cmd = "u+rx"
	}
	return fmt.Sprintf("Run: chmod %s %s or sudo chown $USER %s", cmd, path, path)
}
