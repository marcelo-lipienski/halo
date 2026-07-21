package doctor

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/output"
	"github.com/moby/moby/client"
)

// ParseBytes parses size string to byte count.
func ParseBytes(val string) (uint64, error) {
	val = strings.TrimSpace(strings.ToLower(val))
	if val == "" {
		return 0, nil
	}
	var numStr string
	var unit string
	for i, c := range val {
		if c >= '0' && c <= '9' {
			numStr += string(c)
		} else {
			unit = val[i:]
			break
		}
	}
	num, err := strconv.ParseUint(numStr, 10, 64)
	if err != nil {
		return 0, err
	}
	switch unit {
	case "b", "":
		return num, nil
	case "k", "kb":
		return num * 1024, nil
	case "m", "mb":
		return num * 1024 * 1024, nil
	case "g", "gb":
		return num * 1024 * 1024 * 1024, nil
	}
	return num, nil
}

// FormatBytes formats bytes to human-readable string.
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// RunDoctor runs system prerequisite and host resource checks. See ADR-0006.
func RunDoctor(ctx context.Context, configDir string, compose *config.ComposeConfig) *output.DiagnosticsReport {
	startTime := time.Now()
	var checks []output.CheckResult

	// 1. Docker CLI & Compose v2 check.
	composeCheck := output.CheckResult{
		Group: "System Prerequisites",
		Name:  "Docker CLI & Compose v2",
	}
	cmdCompose := exec.CommandContext(ctx, "docker", "compose", "version")
	if out, err := cmdCompose.Output(); err == nil {
		composeCheck.Status = output.CheckPassed
		versionStr := strings.TrimSpace(string(out))
		if idx := strings.Index(versionStr, "version"); idx != -1 {
			versionStr = versionStr[idx:]
		}
		composeCheck.Name = fmt.Sprintf("Docker CLI & Compose v2 (%s)", versionStr)
	} else {
		// Fallback to legacy Compose v1 check.
		cmdLegacy := exec.CommandContext(ctx, "docker-compose", "version")
		if outLegacy, errLegacy := cmdLegacy.Output(); errLegacy == nil {
			composeCheck.Status = output.CheckWarning
			versionStr := strings.TrimSpace(string(outLegacy))
			if idx := strings.Index(versionStr, "version"); idx != -1 {
				versionStr = versionStr[idx:]
			}
			composeCheck.Name = fmt.Sprintf("Docker CLI & Compose v1 (%s)", versionStr)
			composeCheck.Error = "Legacy Docker Compose v1 detected"
			composeCheck.Mitigation = "Install Docker Compose v2 (e.g. Docker Desktop or docker-compose-plugin)"
		} else {
			composeCheck.Status = output.CheckFailed
			composeCheck.Error = "Docker Compose v2 not found in system PATH"
			composeCheck.Mitigation = "Install Docker Desktop or the docker-compose-plugin package"
		}
	}
	checks = append(checks, composeCheck)

	// 2. Docker Engine version check.
	engineCheck := output.CheckResult{
		Group: "System Prerequisites",
		Name:  "Docker Engine Version",
	}
	dockerCli, dockerErr := client.New(client.FromEnv)
	if dockerErr == nil {
		pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
		ping, pingErr := dockerCli.Ping(pingCtx, client.PingOptions{})
		pingCancel()
		if pingErr == nil {
			engineCheck.Status = output.CheckPassed
			engineCheck.Name = fmt.Sprintf("Docker Engine Version (API %s, OS %s)", ping.APIVersion, ping.OSType)
		} else {
			engineCheck.Status = output.CheckWarning
			engineCheck.Error = fmt.Sprintf("Docker daemon is unreachable: %v", pingErr)
			engineCheck.Mitigation = "Start the Docker service or application (e.g. systemctl start docker)"
		}
		_ = dockerCli.Close()
	} else {
		engineCheck.Status = output.CheckWarning
		engineCheck.Error = fmt.Sprintf("Docker client creation failed: %v", dockerErr)
		engineCheck.Mitigation = "Ensure DOCKER_HOST env var is correct or Docker Desktop is running"
	}
	checks = append(checks, engineCheck)

	// 3. Required CLI tools check.
	toolsCheck := output.CheckResult{
		Group: "System Prerequisites",
		Name:  "Required CLI Tools",
	}
	requiredTools := []string{"git", "make", "docker"}
	var missingTools []string
	for _, tool := range requiredTools {
		if _, err := exec.LookPath(tool); err != nil {
			missingTools = append(missingTools, tool)
		}
	}
	if len(missingTools) == 0 {
		toolsCheck.Status = output.CheckPassed
		toolsCheck.Name = "Required CLI Tools (git, make, docker found)"
	} else {
		toolsCheck.Status = output.CheckWarning
		toolsCheck.Error = fmt.Sprintf("Missing required CLI tools in PATH: %s", strings.Join(missingTools, ", "))
		toolsCheck.Mitigation = fmt.Sprintf("Install missing tools: %s", strings.Join(missingTools, ", "))
	}
	checks = append(checks, toolsCheck)

	// 4. System memory check.
	memCheck := output.CheckResult{
		Group: "Host Resources",
		Name:  "System Memory",
	}
	totalMem, memErr := GetHostMemory()
	if memErr == nil {
		composeLimit := getComposeMemoryLimit(compose)
		if composeLimit > 0 {
			if totalMem < composeLimit {
				memCheck.Status = output.CheckWarning
				memCheck.Error = fmt.Sprintf("Total host memory (%s) is less than the sum of memory limits in Compose file (%s)", FormatBytes(totalMem), FormatBytes(composeLimit))
				memCheck.Mitigation = "Adjust Compose service limits or free up host RAM"
			} else {
				memCheck.Status = output.CheckPassed
				memCheck.Name = fmt.Sprintf("System Memory (%s host RAM, services request max %s)", FormatBytes(totalMem), FormatBytes(composeLimit))
			}
		} else {
			memCheck.Status = output.CheckPassed
			memCheck.Name = fmt.Sprintf("System Memory (%s host RAM)", FormatBytes(totalMem))
		}
	} else {
		memCheck.Status = output.CheckWarning
		memCheck.Error = fmt.Sprintf("Failed to query host memory: %v", memErr)
		memCheck.Mitigation = "Verify system stats tools permission"
	}
	checks = append(checks, memCheck)

	// 5. Free disk space check.
	diskCheck := output.CheckResult{
		Group: "Host Resources",
		Name:  "Free Disk Space",
	}
	absConfigDir, absErr := filepath.Abs(configDir)
	if absErr != nil {
		absConfigDir = configDir
	}
	freeDisk, diskErr := GetFreeDiskSpace(absConfigDir)
	if diskErr == nil {
		minRecommend := uint64(2 * 1024 * 1024 * 1024)
		if freeDisk < minRecommend {
			diskCheck.Status = output.CheckWarning
			diskCheck.Error = fmt.Sprintf("Free disk space (%s) is below recommended 2.0 GiB", FormatBytes(freeDisk))
			diskCheck.Mitigation = "Clean up unused Docker images, logs, or files"
		} else {
			diskCheck.Status = output.CheckPassed
			diskCheck.Name = fmt.Sprintf("Free Disk Space (%s remaining)", FormatBytes(freeDisk))
		}
	} else {
		diskCheck.Status = output.CheckWarning
		diskCheck.Error = fmt.Sprintf("Failed to query disk space: %v", diskErr)
		diskCheck.Mitigation = "Verify directory read permissions"
	}
	checks = append(checks, diskCheck)

	reportStatus := output.StatusHealthy
	for _, check := range checks {
		if check.Status == output.CheckFailed {
			reportStatus = output.StatusEnvironmentBroken
			break
		}
	}

	return &output.DiagnosticsReport{
		Status:     reportStatus,
		DurationMs: time.Since(startTime).Milliseconds(),
		Checks:     checks,
	}
}

func getComposeMemoryLimit(compose *config.ComposeConfig) uint64 {
	if compose == nil {
		return 0
	}
	var total uint64
	for _, service := range compose.Services {
		limitStr := service.Deploy.Resources.Limits.Memory
		if limitStr != "" {
			bytes, err := ParseBytes(limitStr)
			if err == nil {
				total += bytes
			}
		}
	}
	return total
}
