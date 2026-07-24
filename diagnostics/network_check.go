package diagnostics

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/marcelo-lipienski/halo/output"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// resolveEnvVars expands s using .env and host env. See ADR-0003.
func (e *Engine) resolveEnvVars(s string) string {
	return resolveShellExpr(s, e.Env)
}

func ParseHostPortProto(p string) (string, string) {
	proto := "tcp"
	if strings.HasSuffix(p, "/udp") {
		proto = "udp"
		p = strings.TrimSuffix(p, "/udp")
	} else if strings.HasSuffix(p, "/tcp") {
		p = strings.TrimSuffix(p, "/tcp")
	}

	if strings.HasPrefix(p, "[") {
		closeBracketIdx := strings.LastIndex(p, "]")
		if closeBracketIdx != -1 && len(p) > closeBracketIdx+1 && p[closeBracketIdx+1] == ':' {
			remainder := p[closeBracketIdx+2:]
			parts := strings.Split(remainder, ":")
			if len(parts) == 2 || len(parts) == 1 {
				if parts[0] != "" {
					return parts[0], proto
				}
			}
			return "", proto
		}
	}

	parts := strings.Split(p, ":")
	switch len(parts) {
	case 3:
		return parts[1], proto
	case 2, 1:
		if parts[0] != "" {
			return parts[0], proto
		}
		return "", proto
	default:
		return "", proto
	}
}

// portRangeWarnThreshold is the max ports in a range before warning. See ADR-0006.
const portRangeWarnThreshold = 64

func CheckSinglePortCollision(hostPort string, proto string) bool {
	addrs := []string{"127.0.0.1", "0.0.0.0", "[::1]", "[::]"}
	if proto == "udp" {
		for _, addr := range addrs {
			l, err := net.ListenPacket("udp", addr+":"+hostPort)
			if err != nil {
				return true
			}
			_ = l.Close()
		}
		return false
	}
	for _, addr := range addrs {
		l, err := net.Listen("tcp", addr+":"+hostPort)
		if err != nil {
			return true
		}
		_ = l.Close()
	}
	return false
}

func isPortBoundBySelf(port int, proto string, containers []container.Summary, projectName, svcName string) bool {
	for _, c := range containers {
		cProj := strings.ToLower(c.Labels["com.docker.compose.project"])
		cSvc := c.Labels["com.docker.compose.service"]
		if cProj == projectName && cSvc == svcName && c.State == "running" {
			for _, p := range c.Ports {
				if int(p.PublicPort) == port && p.Type == proto {
					return true
				}
			}
		}
	}
	return false
}

func (e *Engine) checkNetworkAndPort(ctx context.Context) []output.CheckResult {
	var results []output.CheckResult

	if err := ctx.Err(); err != nil {
		results = append(results, output.CheckResult{
			Group:      "Network & Port Availability",
			Name:       "Check Timeout",
			Status:     output.CheckFailed,
			Error:      fmt.Sprintf("Network check was cancelled: %v", err),
			Mitigation: "Verify local Docker daemon performance and resources.",
		})
		return results
	}

	projectName := ""
	if envProj := os.Getenv("COMPOSE_PROJECT_NAME"); envProj != "" {
		projectName = envProj
	} else if envProj, ok := e.Env["COMPOSE_PROJECT_NAME"]; ok && envProj != "" {
		projectName = envProj
	} else if e.Compose != nil && e.Compose.Name != "" {
		projectName = e.Compose.Name
	} else if e.ComposePath != "" {
		if absComposePath, err := filepath.Abs(e.ComposePath); err == nil {
			projectName = filepath.Base(filepath.Dir(absComposePath))
		}
	}
	if projectName == "" {
		absDir, _ := filepath.Abs(e.ConfigDir)
		projectName = filepath.Base(absDir)
	}
	projectName = strings.ToLower(projectName)

	// Fetch active containers to check if port is self-bound.
	var containers client.ContainerListResult
	var listErr error
	hasContainers := false
	if e.DockerCli != nil {
		containers, listErr = e.DockerCli.ContainerList(ctx, client.ContainerListOptions{All: true})
		if listErr == nil {
			hasContainers = true
		}
	}

	// 1. Port collision check
	servicesWithCollisions := make(map[string]bool)
	portCollisionPassed := true

	// Sort services for deterministic checks.
	var svcNames []string
	if e.Compose != nil {
		for name := range e.Compose.Services {
			svcNames = append(svcNames, name)
		}
		sort.Strings(svcNames)
	}

	for _, svcName := range svcNames {
		svc := e.Compose.Services[svcName]
		select {
		case <-ctx.Done():
			results = append(results, output.CheckResult{
				Group:      "Network & Port Availability",
				Name:       "Check Timeout",
				Status:     output.CheckFailed,
				Error:      "Network and port collision check timed out",
				Mitigation: "Verify local Docker daemon performance and resources.",
			})
			return results
		default:
		}

		for _, rawPort := range svc.Ports {
			resolvedPort := e.resolveEnvVars(rawPort)
			hostPortRange, proto := ParseHostPortProto(resolvedPort)
			if hostPortRange == "" {
				continue
			}

			// Parse port range.
			var ports []int
			parts := strings.Split(hostPortRange, "-")
			if len(parts) == 1 {
				p, err := strconv.Atoi(hostPortRange)
				if err == nil {
					ports = append(ports, p)
				}
			} else if len(parts) == 2 {
				start, err1 := strconv.Atoi(parts[0])
				end, err2 := strconv.Atoi(parts[1])
				if err1 == nil && err2 == nil {
					if start > end {
						start, end = end, start
					}
					for p := start; p <= end; p++ {
						ports = append(ports, p)
					}
				}
			}

			// Warn on large ranges to prevent timeout. See ADR-0004.
			if len(ports) > portRangeWarnThreshold {
				results = append(results, output.CheckResult{
					Group:      "Network & Port Availability",
					Name:       fmt.Sprintf("Large port range %s (%s) for service %s", hostPortRange, proto, svcName),
					Status:     output.CheckWarning,
					Error:      fmt.Sprintf("Port range %s maps %d ports for service %s. Scanning all ports may be slow and risk exceeding the check timeout.", hostPortRange, len(ports), svcName),
					Mitigation: fmt.Sprintf("Consider narrowing the port range in docker-compose.yml for service %s to fewer than %d ports.", svcName, portRangeWarnThreshold),
				})
			}

			for _, p := range ports {
				if err := ctx.Err(); err != nil {
					results = append(results, output.CheckResult{
						Group:      "Network & Port Availability",
						Name:       "Check Timeout",
						Status:     output.CheckFailed,
						Error:      "Network and port collision check was cancelled",
						Mitigation: "Verify local Docker daemon performance and resources.",
					})
					return results
				}

				pStr := strconv.Itoa(p)
				if hasContainers && isPortBoundBySelf(p, proto, containers.Items, projectName, svcName) {
					continue
				}

				if CheckSinglePortCollision(pStr, proto) {
					servicesWithCollisions[svcName] = true
					portCollisionPassed = false
					errStr := fmt.Sprintf("Port %s (%s) mapped for service %s is already occupied on the host", pStr, proto, svcName)
					mitigation := fmt.Sprintf("Stop the process occupying port %s or change the host port mapping in docker-compose.yml", pStr)
					if procName, pid, err := getOccupyingProcessFunc(pStr, proto); err == nil {
						errStr = fmt.Sprintf("Port %s (%s) mapped for service %s is occupied by '%s' (PID %d) on the host", pStr, proto, svcName, procName, pid)
						mitigation = fmt.Sprintf("Stop the process '%s' (PID %d) occupying port %s or change the host port mapping in docker-compose.yml", procName, pid, pStr)
					}
					results = append(results, output.CheckResult{
						Group:      "Network & Port Availability",
						Name:       fmt.Sprintf("Port Collision %s:%s", svcName, pStr),
						Status:     output.CheckFailed,
						Error:      errStr,
						Mitigation: mitigation,
					})
				}
			}
		}
	}

	if portCollisionPassed {
		results = append(results, output.CheckResult{
			Group:  "Network & Port Availability",
			Name:   "Port Collision Check",
			Status: output.CheckPassed,
		})
	}

	// 2. Service reachability check
	// Warn and skip reachability checks when Docker daemon is unreachable. See ADR-0006.
	if e.DockerCli == nil {
		results = append(results, output.CheckResult{
			Group:      "Network & Port Availability",
			Name:       "Docker Daemon Status",
			Status:     output.CheckWarning,
			Error:      "Docker daemon is unreachable or not running",
			Mitigation: "Start the Docker daemon/service (Docker Desktop, systemctl start docker, etc.) to enable container reachability checks.",
		})
		return results
	}

	if listErr != nil {
		results = append(results, output.CheckResult{
			Group:      "Network & Port Availability",
			Name:       "Docker Daemon Status",
			Status:     output.CheckWarning,
			Error:      fmt.Sprintf("Failed to list docker containers (Docker daemon may be unreachable): %v", listErr),
			Mitigation: "Ensure Docker daemon/service is running and socket is accessible.",
		})
		return results
	}

	for _, svcName := range svcNames {
		select {
		case <-ctx.Done():
			results = append(results, output.CheckResult{
				Group:      "Network & Port Availability",
				Name:       "Check Timeout",
				Status:     output.CheckFailed,
				Error:      "Service reachability check timed out",
				Mitigation: "Verify local Docker daemon performance and resources.",
			})
			return results
		default:
		}

		var matchedContainer *container.Summary
		for _, c := range containers.Items {
			cProj := strings.ToLower(c.Labels["com.docker.compose.project"])
			cSvc := c.Labels["com.docker.compose.service"]
			if cProj == projectName && cSvc == svcName {
				matchedContainer = &c
				break
			}
		}

		if matchedContainer == nil {
			if e.DryRun {
				results = append(results, output.CheckResult{
					Group:      "Network & Port Availability",
					Name:       fmt.Sprintf("Service %s reachability", svcName),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("[Dry-Run] Would create and start container for service %s (docker compose up -d %s)", svcName, svcName),
					Mitigation: fmt.Sprintf("Run: docker compose up -d %s", svcName),
				})
				continue
			}

			shouldFix := e.AutoFix || e.Interactive
			confirmed := true
			if e.Interactive {
				confirmed = promptConfirm(fmt.Sprintf("No container found for service %s. Create and start it (docker compose up -d %s)?", svcName, svcName))
			}

			if shouldFix && confirmed {
				composeFile := e.ComposePath
				if composeFile == "" {
					composeFile = filepath.Join(e.ConfigDir, "docker-compose.yml")
				}
				upArgs := []string{"compose", "-f", composeFile}
				if e.EnvPath != "" {
					if _, statErr := os.Stat(e.EnvPath); statErr == nil {
						upArgs = append(upArgs, "--env-file", e.EnvPath)
					}
				}
				upArgs = append(upArgs, "up", "-d", svcName)
				upCmd := exec.CommandContext(ctx, "docker", upArgs...)
				upCmd.Dir = e.ConfigDir
				if err := upCmd.Run(); err == nil {
					if reContainers, listErr := e.DockerCli.ContainerList(ctx, client.ContainerListOptions{All: true}); listErr == nil {
						for _, c := range reContainers.Items {
							cProj := strings.ToLower(c.Labels["com.docker.compose.project"])
							cSvc := c.Labels["com.docker.compose.service"]
							if cProj == projectName && cSvc == svcName {
								if c.State == "running" {
									results = append(results, output.CheckResult{
										Group:  "Network & Port Availability",
										Name:   fmt.Sprintf("Service %s reachability auto-fixed", svcName),
										Status: output.CheckPassed,
									})
									matchedContainer = &c
									break
								}
							}
						}
						if matchedContainer != nil {
							continue
						}
					}
				}
			}

			results = append(results, output.CheckResult{
				Group:      "Network & Port Availability",
				Name:       fmt.Sprintf("Service %s reachability", svcName),
				Status:     output.CheckWarning,
				Error:      fmt.Sprintf("No container found for service %s in project %s", svcName, projectName),
				Mitigation: fmt.Sprintf("Run: docker compose up -d %s", svcName),
			})
			continue
		}

		inspect, err := e.DockerCli.ContainerInspect(ctx, matchedContainer.ID, client.ContainerInspectOptions{})
		if err != nil {
			results = append(results, output.CheckResult{
				Group:      "Network & Port Availability",
				Name:       fmt.Sprintf("Service %s reachability", svcName),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("Failed to inspect container for service %s: %v", svcName, err),
				Mitigation: fmt.Sprintf("Run: docker compose start %s or check logs: docker compose logs %s", svcName, svcName),
			})
			continue
		}

		if inspect.Container.State == nil || !inspect.Container.State.Running {
			startError := ""
			if inspect.Container.State != nil {
				startError = inspect.Container.State.Error
			}

			var res output.CheckResult
			res.Group = "Network & Port Availability"
			res.Name = fmt.Sprintf("Service %s reachability", svcName)
			res.Status = output.CheckFailed

			if isPortBindError(startError) {
				if servicesWithCollisions[svcName] {
					res.Error = fmt.Sprintf("Container failed to start due to host port collision. Docker error: %s", startError)
					res.Mitigation = fmt.Sprintf("Stop the process occupying the port or change the host port mapping, then restart the service: docker compose up -d %s", svcName)
				} else {
					res.Error = fmt.Sprintf("Container failed to start due to a previous port collision, but the port is now available. Docker error: %s", startError)
					res.Mitigation = fmt.Sprintf("Simply restart the service: docker compose up -d %s", svcName)
				}
			} else {
				stateStr := string(matchedContainer.State)
				if inspect.Container.State != nil {
					stateStr = string(inspect.Container.State.Status)
				}
				res.Error = fmt.Sprintf("Container for service %s is in state '%s' instead of 'running'", svcName, stateStr)
				res.Mitigation = fmt.Sprintf("Run: docker compose start %s or check logs: docker compose logs %s", svcName, svcName)
			}

			if e.DryRun {
				res.Error = fmt.Sprintf("[Dry-Run] Would start exited container for service %s (currently '%s')", svcName, matchedContainer.State)
				results = append(results, res)
				continue
			}

			shouldFix := e.AutoFix || e.Interactive
			confirmed := true
			if e.Interactive {
				confirmed = promptConfirm(fmt.Sprintf("Container for service %s is %s. Start it?", svcName, matchedContainer.State))
			}

			if shouldFix && confirmed {
				if _, startErr := e.DockerCli.ContainerStart(ctx, matchedContainer.ID, client.ContainerStartOptions{}); startErr == nil {
					if updatedInspect, insErr := e.DockerCli.ContainerInspect(ctx, matchedContainer.ID, client.ContainerInspectOptions{}); insErr == nil {
						if updatedInspect.Container.State != nil && updatedInspect.Container.State.Running {
							results = append(results, output.CheckResult{
								Group:  "Network & Port Availability",
								Name:   fmt.Sprintf("Service %s reachability auto-fixed", svcName),
								Status: output.CheckPassed,
							})
							continue
						}
					}
				}
			}

			results = append(results, res)
			continue
		}

		hasUnhealthyOrStarting := false
		if inspect.Container.State != nil && inspect.Container.State.Health != nil {
			healthStatus := string(inspect.Container.State.Health.Status)
			switch healthStatus {
			case "healthy":
				// All good — falls through to passed check.
			case "starting":
				hasUnhealthyOrStarting = true
				results = append(results, output.CheckResult{
					Group:      "Network & Port Availability",
					Name:       fmt.Sprintf("Service %s reachability", svcName),
					Status:     output.CheckWarning,
					Error:      fmt.Sprintf("Container for service %s is running but health check is still initialising", svcName),
					Mitigation: fmt.Sprintf("Wait a few seconds, then re-run halo check. To inspect: docker inspect --format='{{json .State.Health}}' %s", matchedContainer.ID),
				})
			default:
				hasUnhealthyOrStarting = true
				results = append(results, output.CheckResult{
					Group:      "Network & Port Availability",
					Name:       fmt.Sprintf("Service %s reachability", svcName),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Container for service %s passed running check but health state is '%s'", svcName, healthStatus),
					Mitigation: fmt.Sprintf("Check service health: docker inspect --format='{{json .State.Health}}' %s", matchedContainer.ID),
				})
			}
		}

		if !hasUnhealthyOrStarting {
			results = append(results, output.CheckResult{
				Group:  "Network & Port Availability",
				Name:   fmt.Sprintf("Service %s reachability", svcName),
				Status: output.CheckPassed,
			})
		}
	}

	return results
}

func isPortBindError(errStr string) bool {
	errStr = strings.ToLower(errStr)
	return strings.Contains(errStr, "port") ||
		strings.Contains(errStr, "bind") ||
		strings.Contains(errStr, "address") ||
		strings.Contains(errStr, "allocat") ||
		strings.Contains(errStr, "already in use") ||
		strings.Contains(errStr, "connectivity")
}

var ssUsersRegex = regexp.MustCompile(`users:\(\("([^"]+)",pid=(\d+)`)

var getOccupyingProcessFunc = func(port string, proto string) (string, int, error) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("netstat", "-ano", "-p", proto)
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return "", 0, err
		}

		lines := strings.Split(out.String(), "\n")
		var pid int
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				localAddr := fields[1]
				if strings.HasSuffix(localAddr, ":"+port) || strings.HasSuffix(localAddr, "]:"+port) {
					pidStr := fields[len(fields)-1]
					if p, err := strconv.Atoi(pidStr); err == nil {
						pid = p
						break
					}
				}
			}
		}
		if pid == 0 {
			return "", 0, fmt.Errorf("no process found on port %s", port)
		}

		cmd2 := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
		var out2 bytes.Buffer
		cmd2.Stdout = &out2
		if err := cmd2.Run(); err == nil {
			parts := strings.Split(out2.String(), ",")
			if len(parts) > 0 {
				procName := strings.Trim(parts[0], `"`)
				return procName, pid, nil
			}
		}
		return "Unknown", pid, nil
	}

	if runtime.GOOS == "darwin" {
		cmd := exec.Command("lsof", "-i", fmt.Sprintf("%s:%s", proto, port), "-P", "-n", "-sTCP:LISTEN")
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
			lines := strings.Split(out.String(), "\n")
			if len(lines) > 1 {
				fields := strings.Fields(lines[1])
				if len(fields) >= 2 {
					name := fields[0]
					pid, err := strconv.Atoi(fields[1])
					if err == nil {
						return name, pid, nil
					}
				}
			}
		}
	}

	if runtime.GOOS == "linux" {
		ssFlag := "-lptn"
		if proto == "udp" {
			ssFlag = "-lpun"
		}
		cmd := exec.Command("ss", ssFlag, "sport = :"+port)
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
			lines := strings.Split(out.String(), "\n")
			for _, line := range lines {
				if strings.Contains(line, "users:") {
					matches := ssUsersRegex.FindStringSubmatch(line)
					if len(matches) == 3 {
						if p, err := strconv.Atoi(matches[2]); err == nil {
							return matches[1], p, nil
						}
					}
				}
			}
		}
	}

	return "", 0, fmt.Errorf("no occupying process found")
}

// GetOccupyingProcess wraps getOccupyingProcessFunc.
func GetOccupyingProcess(port string, proto string) (string, int, error) {
	return getOccupyingProcessFunc(port, proto)
}
