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
		if (c >= '0' && c <= '9') || c == '.' {
			numStr += string(c)
		} else {
			unit = val[i:]
			break
		}
	}
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, err
	}
	var mult float64 = 1
	switch unit {
	case "b", "":
		mult = 1
	case "k", "kb", "kib", "ki":
		mult = 1024
	case "m", "mb", "mib", "mi":
		mult = 1024 * 1024
	case "g", "gb", "gib", "gi":
		mult = 1024 * 1024 * 1024
	case "t", "tb", "tib", "ti":
		mult = 1024 * 1024 * 1024 * 1024
	}
	return uint64(num * mult), nil
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
	checks = append(checks, checkComposeVersion(ctx))

	// 2. Docker Engine version check.
	checks = append(checks, checkDockerEngine(ctx))

	// 3. Required CLI tools check.
	checks = append(checks, checkRequiredTools(ctx, []string{"git", "make", "docker"}))

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

func checkDockerEngine(ctx context.Context) output.CheckResult {
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
		engineCheck.Error = fmt.Sprintf("Docker daemon is unreachable: %v", dockerErr)
		engineCheck.Mitigation = "Ensure DOCKER_HOST env var is correct or Docker Desktop is running"
	}
	return engineCheck
}

func checkComposeVersion(ctx context.Context) output.CheckResult {
	composeCheck := output.CheckResult{
		Group: "System Prerequisites",
		Name:  "Docker CLI & Compose v2",
	}
	cmdCompose := exec.CommandContext(ctx, "docker", "compose", "version")
	if out, err := cmdCompose.Output(); err == nil {
		composeCheck.Status = output.CheckPassed
		versionStr := parseComposeVersionStr(string(out))
		composeCheck.Name = fmt.Sprintf("Docker CLI & Compose v2 (%s)", versionStr)
	} else {
		// Fallback to legacy Compose v1 check.
		cmdLegacy := exec.CommandContext(ctx, "docker-compose", "version")
		if outLegacy, errLegacy := cmdLegacy.Output(); errLegacy == nil {
			composeCheck.Status = output.CheckWarning
			versionStr := parseComposeVersionStr(string(outLegacy))
			composeCheck.Name = fmt.Sprintf("Docker CLI & Compose v1 (%s)", versionStr)
			composeCheck.Error = "Legacy Docker Compose v1 detected"
			composeCheck.Mitigation = "Install Docker Compose v2 (e.g. Docker Desktop or docker-compose-plugin)"
		} else {
			composeCheck.Status = output.CheckFailed
			composeCheck.Error = "Docker Compose v2 not found in system PATH"
			composeCheck.Mitigation = "Install Docker Desktop or the docker-compose-plugin package"
		}
	}
	return composeCheck
}

func parseComposeVersionStr(raw string) string {
	versionStr := strings.TrimSpace(raw)
	if idx := strings.Index(versionStr, "version"); idx != -1 {
		versionStr = versionStr[idx:]
	}
	return versionStr
}

func checkRequiredTools(ctx context.Context, requiredTools []string) output.CheckResult {
	toolsCheck := output.CheckResult{
		Group: "System Prerequisites",
		Name:  "Required CLI Tools",
	}
	var missingTools []string
	for _, tool := range requiredTools {
		if err := ctx.Err(); err != nil {
			toolsCheck.Status = output.CheckFailed
			toolsCheck.Error = fmt.Sprintf("Tool check cancelled: %v", err)
			return toolsCheck
		}
		if _, err := exec.LookPath(tool); err != nil {
			missingTools = append(missingTools, tool)
		}
	}
	if len(missingTools) == 0 {
		toolsCheck.Status = output.CheckPassed
		toolsCheck.Name = fmt.Sprintf("Required CLI Tools (%s found)", strings.Join(requiredTools, ", "))
	} else {
		toolsCheck.Status = output.CheckWarning
		toolsCheck.Error = fmt.Sprintf("Missing required CLI tools in PATH: %s", strings.Join(missingTools, ", "))
		toolsCheck.Mitigation = fmt.Sprintf("Install missing tools: %s", strings.Join(missingTools, ", "))
	}
	return toolsCheck
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
