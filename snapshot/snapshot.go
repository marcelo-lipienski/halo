package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/diagnostics"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type FileSnapshot struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	Hash    string    `json:"hash"`
}

type PortSnapshot struct {
	Service       string `json:"service"`
	HostPort      string `json:"host_port"`
	ContainerPort string `json:"container_port"`
	Protocol      string `json:"protocol"`
	IsOccupied    bool   `json:"is_occupied"`
	ProcessName   string `json:"process_name,omitempty"`
	PID           int    `json:"pid,omitempty"`
}

type ContainerSnapshot struct {
	ContainerID   string `json:"container_id"`
	ContainerName string `json:"container_name"`
	State         string `json:"state"`
	Status        string `json:"status"`
	Image         string `json:"image"`
	ImageID       string `json:"image_id"`
}

type EnvironmentSnapshot struct {
	CreatedAt time.Time                            `json:"created_at"`
	Project   string                               `json:"project"`
	Files     map[string]FileSnapshot              `json:"files"`     // path -> file info
	Variables map[string]map[string]string         `json:"variables"` // file path -> key -> value
	Ports     []PortSnapshot                       `json:"ports"`
	Services  map[string]ContainerSnapshot         `json:"services"`  // service name -> container info
}

// Helper to compute SHA256 hash of a file
func computeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Parse host port and protocol from string
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

// Check if a single port is occupied on host
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

// CreateSnapshot captures the state of the local environment
func CreateSnapshot(configDir string, envPath string, composeFiles []string) (*EnvironmentSnapshot, []string, error) {
	var warnings []string

	// Resolve envPath
	if envPath == "" {
		envPath = filepath.Join(configDir, ".env")
	}

	// Parse main env file to get variables
	envMap, envErr := config.ParseEnv(envPath)
	if envErr != nil {
		// It's not fatal if .env doesn't exist yet, we will snapshot whatever exists
		envMap = make(map[string]string)
	}

	// Resolve compose files
	var filesToLoad []string
	if len(composeFiles) > 0 {
		filesToLoad = composeFiles
	} else {
		composePathYml := filepath.Join(configDir, "docker-compose.yml")
		composePathYaml := filepath.Join(configDir, "docker-compose.yaml")
		composePath := composePathYaml
		if _, err := os.Stat(composePathYml); err == nil {
			composePath = composePathYml
		}
		filesToLoad = append(filesToLoad, composePath)

		overridePathYml := filepath.Join(configDir, "docker-compose.override.yml")
		overridePathYaml := filepath.Join(configDir, "docker-compose.override.yaml")
		if _, err := os.Stat(overridePathYml); err == nil {
			filesToLoad = append(filesToLoad, overridePathYml)
		} else if _, err := os.Stat(overridePathYaml); err == nil {
			filesToLoad = append(filesToLoad, overridePathYaml)
		}
	}

	// Project name determination
	projectName := ""
	if envProj := os.Getenv("COMPOSE_PROJECT_NAME"); envProj != "" {
		projectName = envProj
	} else if envProj, ok := envMap["COMPOSE_PROJECT_NAME"]; ok && envProj != "" {
		projectName = envProj
	} else {
		absDir, _ := filepath.Abs(configDir)
		projectName = filepath.Base(absDir)
	}
	projectName = strings.ToLower(projectName)

	// Collect file paths to snapshot
	trackedFiles := make(map[string]bool)
	addFile := func(p string) {
		if p == "" {
			return
		}
		p = filepath.Clean(p)
		if stat, err := os.Stat(p); err == nil && !stat.IsDir() {
			abs, err := filepath.Abs(p)
			if err == nil {
				trackedFiles[abs] = true
			} else {
				trackedFiles[p] = true
			}
		}
	}

	addFile(envPath)
	addFile(filepath.Join(configDir, ".env.example"))
	addFile(filepath.Join(filepath.Dir(envPath), ".env.example"))
	for _, f := range filesToLoad {
		addFile(f)
	}

	// Parse compose configs to find service env_files
	var parsedConfigs []*config.ComposeConfig
	for _, file := range filesToLoad {
		if _, err := os.Stat(file); err == nil {
			if comp, err := config.ParseCompose(file); err == nil {
				parsedConfigs = append(parsedConfigs, comp)
			}
		}
	}
	mergedComp := config.MergeComposeConfigs(parsedConfigs...)

	for _, svc := range mergedComp.Services {
		for _, ef := range svc.EnvFiles {
			resolvedPath := diagnostics.ResolveShellExpr(ef.File, envMap)
			path := resolvedPath
			if !filepath.IsAbs(path) {
				baseDir := ef.BaseDir
				if baseDir == "" {
					baseDir = configDir
				}
				path = filepath.Join(baseDir, path)
			}
			addFile(path)
		}
	}

	// 1. Create File snapshots
	filesSnap := make(map[string]FileSnapshot)
	varsSnap := make(map[string]map[string]string)

	for p := range trackedFiles {
		stat, err := os.Stat(p)
		if err != nil {
			continue
		}
		hash, err := computeSHA256(p)
		if err != nil {
			continue
		}
		
		relPath, err := filepath.Rel(configDir, p)
		if err != nil {
			relPath = p
		}

		filesSnap[relPath] = FileSnapshot{
			Path:    relPath,
			Size:    stat.Size(),
			ModTime: stat.ModTime(),
			Hash:    hash,
		}

		// If it is an env file, load its variables
		if strings.HasSuffix(filepath.Base(p), ".env") || strings.Contains(filepath.Base(p), ".env.") || filepath.Base(p) == ".env" {
			if vars, err := config.ParseEnv(p); err == nil {
				varsSnap[relPath] = vars
			}
		}
	}

	// 2. Setup Docker client
	var dockerCli client.APIClient
	var dockerErr error
	dockerCli, dockerErr = client.New(client.FromEnv)
	if dockerErr == nil {
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, dockerErr = dockerCli.Ping(pingCtx, client.PingOptions{})
		pingCancel()
	}

	if dockerErr != nil {
		warnings = append(warnings, fmt.Sprintf("Docker daemon is unreachable: %v. Skipping container state and health checks.", dockerErr))
		dockerCli = nil
	} else {
		defer func() { _ = dockerCli.Close() }()
	}

	// 3. Port snapshots & Container snapshots
	var portsSnap []PortSnapshot
	servicesSnap := make(map[string]ContainerSnapshot)

	var containers client.ContainerListResult
	var listErr error
	if dockerCli != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		containers, listErr = dockerCli.ContainerList(ctx, client.ContainerListOptions{All: true})
		cancel()
		if listErr != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to list Docker containers: %v", listErr))
		}
	}

	for svcName, svc := range mergedComp.Services {
		// Ports check
		for _, rawPort := range svc.Ports {
			resolvedPort := diagnostics.ResolveShellExpr(rawPort, envMap)
			hostPortRange, proto := parseHostPortProto(resolvedPort)
			if hostPortRange == "" {
				continue
			}

			var ports []int
			parts := strings.Split(hostPortRange, "-")
			if len(parts) == 1 {
				if p, err := strconv.Atoi(hostPortRange); err == nil {
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

			// Gather published vs target ports
			targetPort := ""
			pParts := strings.Split(resolvedPort, ":")
			if len(pParts) >= 2 {
				targetPort = pParts[len(pParts)-1]
				if strings.Contains(targetPort, "/") {
					targetPort = strings.Split(targetPort, "/")[0]
				}
			}

			for _, p := range ports {
				pStr := strconv.Itoa(p)
				occupied := checkSinglePortCollision(pStr, proto)
				procName := ""
				pid := 0

				if occupied {
					procName, pid, _ = diagnostics.GetOccupyingProcess(pStr, proto)
				}

				portsSnap = append(portsSnap, PortSnapshot{
					Service:       svcName,
					HostPort:      pStr,
					ContainerPort: targetPort,
					Protocol:      proto,
					IsOccupied:    occupied,
					ProcessName:   procName,
					PID:           pid,
				})
			}
		}

		// Container check
		if dockerCli != nil && listErr == nil {
			var matchedContainer *container.Summary
			for _, c := range containers.Items {
				cProj := strings.ToLower(c.Labels["com.docker.compose.project"])
				cSvc := c.Labels["com.docker.compose.service"]
				if cProj == projectName && cSvc == svcName {
					matchedContainer = &c
					break
				}
			}

			if matchedContainer != nil {
				state := string(matchedContainer.State)
				status := ""

				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				inspect, err := dockerCli.ContainerInspect(ctx, matchedContainer.ID, client.ContainerInspectOptions{})
				cancel()

				if err == nil {
					if inspect.Container.State != nil {
						state = string(inspect.Container.State.Status)
						if inspect.Container.State.Health != nil {
							status = string(inspect.Container.State.Health.Status)
						}
					}
				}

				name := ""
				if len(matchedContainer.Names) > 0 {
					name = strings.TrimPrefix(matchedContainer.Names[0], "/")
				}

				servicesSnap[svcName] = ContainerSnapshot{
					ContainerID:   matchedContainer.ID,
					ContainerName: name,
					State:         state,
					Status:        status,
					Image:         matchedContainer.Image,
					ImageID:       matchedContainer.ImageID,
				}
			}
		}
	}

	snap := &EnvironmentSnapshot{
		CreatedAt: time.Now(),
		Project:   projectName,
		Files:     filesSnap,
		Variables: varsSnap,
		Ports:     portsSnap,
		Services:  servicesSnap,
	}

	return snap, warnings, nil
}
