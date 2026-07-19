package diagnostics

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/marcelo-lipienski/halo/output"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// resolveEnvVars expands environment variables like ${PORT:-80} or $PORT using e.Env and system Env
func (e *Engine) resolveEnvVars(s string) string {
	return envVarRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := envVarRegex.FindStringSubmatch(match)
		if len(sub) == 0 {
			return match
		}
		varName := ""
		fallback := ""
		if len(sub) > 1 && sub[1] != "" {
			varName = sub[1]
			if len(sub) > 2 {
				fallback = sub[2]
			}
		} else if len(sub) > 3 && sub[3] != "" {
			varName = sub[3]
		}

		// Check if it's the :- format (meaning fallback if unset OR empty)
		isUnsetOrEmptyFallback := strings.Contains(match, ":-")

		val, ok := os.LookupEnv(varName)
		if !ok {
			val, ok = e.Env[varName]
		}

		if ok {
			if val == "" && isUnsetOrEmptyFallback {
				return fallback
			}
			return val
		}
		return fallback
	})
}

func parseHostPortProto(p string) (string, string) {
	proto := "tcp"
	if strings.HasSuffix(p, "/udp") {
		proto = "udp"
		p = strings.TrimSuffix(p, "/udp")
	} else if strings.HasSuffix(p, "/tcp") {
		p = strings.TrimSuffix(p, "/tcp")
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

func checkPortCollision(hostPortRange string, proto string) bool {
	parts := strings.Split(hostPortRange, "-")
	if len(parts) == 1 {
		return checkSinglePortCollision(hostPortRange, proto)
	}
	if len(parts) == 2 {
		start, err1 := strconv.Atoi(parts[0])
		end, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return checkSinglePortCollision(hostPortRange, proto)
		}
		if start > end {
			start, end = end, start
		}
		for p := start; p <= end; p++ {
			if checkSinglePortCollision(strconv.Itoa(p), proto) {
				return true
			}
		}
		return false
	}
	return checkSinglePortCollision(hostPortRange, proto)
}

func checkSinglePortCollision(hostPort string, proto string) bool {
	if proto == "udp" {
		l, err := net.ListenPacket("udp", "127.0.0.1:"+hostPort)
		if err != nil {
			return true
		}
		l.Close()
		return false
	}
	l, err := net.Listen("tcp", "127.0.0.1:"+hostPort)
	if err != nil {
		return true
	}
	l.Close()
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
	for svcName, svc := range e.Compose.Services {
		select {
		case <-ctx.Done():
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

			for _, p := range ports {
				pStr := strconv.Itoa(p)
				if hasContainers && isPortBoundBySelf(p, proto, containers.Items, projectName, svcName) {
					continue
				}

				if checkSinglePortCollision(pStr, proto) {
					servicesWithCollisions[svcName] = true
					portCollisionPassed = false
					results = append(results, output.CheckResult{
						Group:      "Network & Port Availability",
						Name:       fmt.Sprintf("Port Collision %s:%s", svcName, pStr),
						Status:     output.CheckFailed,
						Error:      fmt.Sprintf("Port %s (%s) mapped for service %s is already occupied on the host", pStr, proto, svcName),
						Mitigation: fmt.Sprintf("Stop the process occupying port %s or change the host port mapping in docker-compose.yml", pStr),
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

	reachabilityPassed := true
	for svcName := range e.Compose.Services {
		select {
		case <-ctx.Done():
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
			reachabilityPassed = false
			results = append(results, output.CheckResult{
				Group:      "Network & Port Availability",
				Name:       fmt.Sprintf("Service %s unreachable", svcName),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("No container found for service %s in project %s", svcName, projectName),
				Mitigation: fmt.Sprintf("Run: docker-compose up -d %s", svcName),
			})
			continue
		}

		inspect, err := e.DockerCli.ContainerInspect(ctx, matchedContainer.ID, client.ContainerInspectOptions{})
		if err != nil {
			if matchedContainer.State != "running" {
				reachabilityPassed = false
				results = append(results, output.CheckResult{
					Group:      "Network & Port Availability",
					Name:       fmt.Sprintf("Service %s is %s", svcName, matchedContainer.State),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Container for service %s is in state '%s' instead of 'running' (inspect failed)", svcName, matchedContainer.State),
					Mitigation: fmt.Sprintf("Run: docker-compose start %s or check logs: docker-compose logs %s", svcName, svcName),
				})
			}
			continue
		}

		if inspect.Container.State == nil || !inspect.Container.State.Running {
			reachabilityPassed = false
			startError := ""
			if inspect.Container.State != nil {
				startError = inspect.Container.State.Error
			}

			if isPortBindError(startError) {
				if servicesWithCollisions[svcName] {
					results = append(results, output.CheckResult{
						Group:      "Network & Port Availability",
						Name:       fmt.Sprintf("Service %s failed to start", svcName),
						Status:     output.CheckFailed,
						Error:      fmt.Sprintf("Container failed to start due to host port collision. Docker error: %s", startError),
						Mitigation: fmt.Sprintf("Stop the process occupying the port or change the host port mapping, then restart the service: docker-compose up -d %s", svcName),
					})
				} else {
					results = append(results, output.CheckResult{
						Group:      "Network & Port Availability",
						Name:       fmt.Sprintf("Service %s failed to start", svcName),
						Status:     output.CheckFailed,
						Error:      fmt.Sprintf("Container failed to start due to a previous port collision, but the port is now available. Docker error: %s", startError),
						Mitigation: fmt.Sprintf("Simply restart the service: docker-compose up -d %s", svcName),
					})
				}
			} else {
				stateStr := string(matchedContainer.State)
				if inspect.Container.State != nil {
					stateStr = string(inspect.Container.State.Status)
				}
				results = append(results, output.CheckResult{
					Group:      "Network & Port Availability",
					Name:       fmt.Sprintf("Service %s is %s", svcName, stateStr),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Container for service %s is in state '%s' instead of 'running'", svcName, stateStr),
					Mitigation: fmt.Sprintf("Run: docker-compose start %s or check logs: docker-compose logs %s", svcName, svcName),
				})
			}
			continue
		}

		if inspect.Container.State != nil && inspect.Container.State.Health != nil {
			healthStatus := string(inspect.Container.State.Health.Status)
			if healthStatus != "healthy" {
				reachabilityPassed = false
				results = append(results, output.CheckResult{
					Group:      "Network & Port Availability",
					Name:       fmt.Sprintf("Service %s health is %s", svcName, healthStatus),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Container for service %s passed running check but health state is '%s'", svcName, healthStatus),
					Mitigation: fmt.Sprintf("Check service health: docker inspect --format='{{json .State.Health}}' %s", matchedContainer.ID),
				})
			}
		}
	}

	if reachabilityPassed {
		results = append(results, output.CheckResult{
			Group:  "Network & Port Availability",
			Name:   "Service Reachability Check",
			Status: output.CheckPassed,
		})
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
