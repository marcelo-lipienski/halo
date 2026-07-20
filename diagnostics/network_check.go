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

// resolveEnvVars expands shell parameter expressions in s using the .env map
// and host OS environment. It delegates to resolveShellExpr for full shell-spec
// compliance including ${VAR:-default} and ${VAR:?error} forms.
func (e *Engine) resolveEnvVars(s string) string {
	return resolveShellExpr(s, e.Env)
}

func parseHostPortProto(p string) (string, string) {
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
			if len(parts) == 2 {
				return parts[0], proto
			}
			return "", proto
		}
	}

	parts := strings.Split(p, ":")
	switch len(parts) {
	case 3:
		return parts[1], proto
	case 2:
		return parts[0], proto
	default:
		return "", proto
	}
}

// portRangeWarnThreshold is the maximum number of ports in a mapped range before
// halo emits a CheckWarning. Large ranges significantly increase check time and
// may exhaust the 2-second check group timeout.
const portRangeWarnThreshold = 64

func checkSinglePortCollision(hostPort string, proto string) bool {
	if proto == "udp" {
		l, err := net.ListenPacket("udp", "127.0.0.1:"+hostPort)
		if err != nil {
			return true
		}
		_ = l.Close()

		l2, err2 := net.ListenPacket("udp", "0.0.0.0:"+hostPort)
		if err2 != nil {
			return true
		}
		_ = l2.Close()
		return false
	}
	l, err := net.Listen("tcp", "127.0.0.1:"+hostPort)
	if err != nil {
		return true
	}
	_ = l.Close()

	l2, err2 := net.Listen("tcp", "0.0.0.0:"+hostPort)
	if err2 != nil {
		return true
	}
	_ = l2.Close()
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
	} else {
		absDir, _ := filepath.Abs(e.ConfigDir)
		projectName = filepath.Base(absDir)
	}
	projectName = strings.ToLower(projectName)

	// Fetch containers first to check if a port is bound by our own active container
	var containers client.ContainerListResult
	var listErr error
	hasContainers := false
	if e.DockerCli != nil {
		containers, listErr = e.DockerCli.ContainerList(ctx, client.ContainerListOptions{All: true})
		if listErr == nil {
			hasContainers = true
		}
	}

	// 1. Port Collision Check
	servicesWithCollisions := make(map[string]bool)
	portCollisionPassed := true

	// Sort service names for deterministic checks
	var svcNames []string
	for name := range e.Compose.Services {
		svcNames = append(svcNames, name)
	}
	sort.Strings(svcNames)

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
			hostPortRange, proto := parseHostPortProto(resolvedPort)
			if hostPortRange == "" {
				continue
			}

			// Parse range into individual ports
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

			// Warn when a port range is unusually large — scanning every port is
			// slow and risks exhausting the 2-second check group timeout.
			if len(ports) > portRangeWarnThreshold {
				results = append(results, output.CheckResult{
					Group:  "Network & Port Availability",
					Name:   fmt.Sprintf("Large port range %s (%s) for service %s", hostPortRange, proto, svcName),
					Status: output.CheckWarning,
					Error:  fmt.Sprintf("Port range %s maps %d ports for service %s. Scanning all ports may be slow and risk exceeding the check timeout.", hostPortRange, len(ports), svcName),
					Mitigation: fmt.Sprintf("Consider narrowing the port range in docker-compose.yml for service %s to fewer than %d ports.", svcName, portRangeWarnThreshold),
				})
			}

			for _, p := range ports {
				pStr := strconv.Itoa(p)
				if hasContainers && isPortBoundBySelf(p, proto, containers.Items, projectName, svcName) {
					continue
				}

				if checkSinglePortCollision(pStr, proto) {
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

	// 2. Service Reachability Check
	if e.DockerCli == nil {
		results = append(results, output.CheckResult{
			Group:      "Network & Port Availability",
			Name:       "Service Reachability",
			Status:     output.CheckFailed,
			Error:      "Docker client is not initialized",
			Mitigation: "Ensure Docker daemon is running and socket is accessible.",
		})
		return results
	}

	if listErr != nil {
		results = append(results, output.CheckResult{
			Group:      "Network & Port Availability",
			Name:       "Service Reachability",
			Status:     output.CheckFailed,
			Error:      fmt.Sprintf("Failed to list docker containers: %v", listErr),
			Mitigation: "Check docker service status and permissions.",
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

var getOccupyingProcessFunc = func(port string, proto string) (string, int, error) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("netstat", "-ano")
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return "", 0, err
		}
		lines := strings.Split(out.String(), "\n")
		pid := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if !strings.HasPrefix(strings.ToLower(line), strings.ToLower(proto)) {
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

	lsofProto := proto
	cmd := exec.Command("lsof", "-i", fmt.Sprintf("%s:%s", lsofProto, port), "-F", "pc")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		lines := strings.Split(out.String(), "\n")
		var pid int
		var name string
		for _, line := range lines {
			if strings.HasPrefix(line, "p") {
				if p, err := strconv.Atoi(line[1:]); err == nil {
					pid = p
				}
			} else if strings.HasPrefix(line, "c") {
				name = line[1:]
			}
		}
		if pid > 0 && name != "" {
			return name, pid, nil
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
				if strings.Contains(line, "pid=") {
					re := regexp.MustCompile(`users:\(\("([^"]+)",pid=(\d+)`)
					matches := re.FindStringSubmatch(line)
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
