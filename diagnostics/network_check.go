package diagnostics

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/marcelo-lipienski/halo/output"
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

		if val, ok := e.Env[varName]; ok {
			return val
		}
		if val, ok := os.LookupEnv(varName); ok {
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

func checkPortCollision(hostPort string, proto string) bool {
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

func (e *Engine) checkNetworkAndPort(ctx context.Context) []output.CheckResult {
	var results []output.CheckResult

	// 1. Port Collision Check
	portCollisionPassed := true
	for svcName, svc := range e.Compose.Services {
		for _, rawPort := range svc.Ports {
			resolvedPort := e.resolveEnvVars(rawPort)
			hostPort, proto := parseHostPortProto(resolvedPort)
			if hostPort == "" {
				continue
			}

			if checkPortCollision(hostPort, proto) {
				portCollisionPassed = false
				results = append(results, output.CheckResult{
					Group:      "Network & Port Availability",
					Name:       fmt.Sprintf("Port Collision %s:%s", svcName, hostPort),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Port %s (%s) mapped for service %s is already occupied on the host", hostPort, proto, svcName),
					Mitigation: fmt.Sprintf("Stop the process occupying port %s or change the host port mapping in docker-compose.yml", hostPort),
				})
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

	absDir, _ := filepath.Abs(e.ConfigDir)
	projectName := filepath.Base(absDir)
	if envProj := os.Getenv("COMPOSE_PROJECT_NAME"); envProj != "" {
		projectName = envProj
	}
	projectName = strings.ToLower(projectName)

	containers, err := e.DockerCli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		results = append(results, output.CheckResult{
			Group:      "Network & Port Availability",
			Name:       "Service Reachability",
			Status:     output.CheckFailed,
			Error:      fmt.Sprintf("Failed to list docker containers: %v", err),
			Mitigation: "Check docker service status and permissions.",
		})
		return results
	}

	reachabilityPassed := true
	for svcName := range e.Compose.Services {
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

		if matchedContainer.State != "running" {
			reachabilityPassed = false
			results = append(results, output.CheckResult{
				Group:      "Network & Port Availability",
				Name:       fmt.Sprintf("Service %s is %s", svcName, matchedContainer.State),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("Container for service %s is in state '%s' instead of 'running'", svcName, matchedContainer.State),
				Mitigation: fmt.Sprintf("Run: docker-compose start %s or check logs: docker-compose logs %s", svcName, svcName),
			})
			continue
		}

		// Inspect to check health status if configured
		inspect, err := e.DockerCli.ContainerInspect(ctx, matchedContainer.ID, client.ContainerInspectOptions{})
		if err == nil && inspect.Container.State != nil && inspect.Container.State.Health != nil {
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
