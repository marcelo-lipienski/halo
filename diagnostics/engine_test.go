package diagnostics

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/output"
)

type mockDockerClient struct {
	client.ContainerAPIClient
	listFunc    func(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	inspectFunc func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
}

func (m *mockDockerClient) ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, options)
	}
	return client.ContainerListResult{}, nil
}

func (m *mockDockerClient) ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	if m.inspectFunc != nil {
		return m.inspectFunc(ctx, containerID, options)
	}
	return client.ContainerInspectResult{}, nil
}

func TestEngineRun(t *testing.T) {
	tempDir := t.TempDir()

	// Write a mock docker-compose.yml
	composeContent := `
services:
  app:
    environment:
      - PORT=${APP_PORT:-8080}
      - DB_USER=${DB_USER}
    ports:
      - "${HOST_PORT:-8081}:8080"
    volumes:
      - ./data:/app/data
`
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	// Create volume source folder
	dataPath := filepath.Join(tempDir, "data")
	if err := os.Mkdir(dataPath, 0755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	// Case 1: Healthy configuration
	env := map[string]string{
		"APP_PORT":  "9000",
		"DB_USER":   "postgres",
		"HOST_PORT": "12345", // Use a high port that is unlikely to collide
	}

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse compose: %v", err)
	}

	projName := filepath.Base(tempDir)
	mockDocker := &mockDockerClient{
		listFunc: func(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
			return client.ContainerListResult{
				Items: []container.Summary{
					{
						ID:    "mock-id",
						State: "running",
						Labels: map[string]string{
							"com.docker.compose.project": projName,
							"com.docker.compose.service": "app",
						},
					},
				},
			}, nil
		},
		inspectFunc: func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			return client.ContainerInspectResult{
				Container: container.InspectResponse{
					State: &container.State{
						Running: true,
						Health:  nil,
					},
				},
			}, nil
		},
	}

	engine := NewEngine(tempDir, composePath, env, comp, mockDocker)
	report := engine.Run(context.Background())

	if report.Status != output.StatusHealthy {
		t.Errorf("expected status healthy, got: %s", report.Status)
		for _, check := range report.Checks {
			if check.Status == output.CheckFailed {
				t.Logf("failed check: %s - Error: %s", check.Name, check.Error)
			}
		}
	}

	// Case 2: Missing env variable
	badEnv := map[string]string{
		"APP_PORT": "9000",
		// DB_USER is missing
	}
	engine = NewEngine(tempDir, composePath, badEnv, comp, mockDocker)
	report = engine.Run(context.Background())

	if report.Status != output.StatusEnvironmentBroken {
		t.Errorf("expected status environment_broken for missing env, got: %s", report.Status)
	}

	// Check that we found the specific error
	foundMissingErr := false
	for _, check := range report.Checks {
		if check.Status == output.CheckFailed && check.Group == "Environmental Alignment" {
			foundMissingErr = true
		}
	}
	if !foundMissingErr {
		t.Error("expected to find missing env variable check failure")
	}
}

func TestEngineCustomFilePaths(t *testing.T) {
	tempDir := t.TempDir()

	composeContent := `
services:
  app:
    environment:
      - PORT=${APP_PORT}
`
	composePath := filepath.Join(tempDir, "docker-compose.custom.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write custom compose file: %v", err)
	}

	env := map[string]string{
		"APP_PORT": "9000",
	}

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse custom compose: %v", err)
	}

	projName := filepath.Base(tempDir)
	mockDocker := &mockDockerClient{
		listFunc: func(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
			return client.ContainerListResult{
				Items: []container.Summary{
					{
						ID:    "mock-id",
						State: "running",
						Labels: map[string]string{
							"com.docker.compose.project": projName,
							"com.docker.compose.service": "app",
						},
					},
				},
			}, nil
		},
		inspectFunc: func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			return client.ContainerInspectResult{
				Container: container.InspectResponse{
					State: &container.State{
						Running: true,
					},
				},
			}, nil
		},
	}

	engine := NewEngine(tempDir, composePath, env, comp, mockDocker)
	report := engine.Run(context.Background())

	if report.Status != output.StatusHealthy {
		t.Errorf("expected status healthy for custom paths, got: %s", report.Status)
	}
}

func TestEngineVariableDefaults(t *testing.T) {
	tempDir := t.TempDir()

	composeContent := `
services:
  app:
    environment:
      - PORT=${APP_PORT:-8080}
      - PLATFORM=${DOCKER_PLATFORM-linux/amd}
`
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	env := map[string]string{}

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse compose: %v", err)
	}

	projName := filepath.Base(tempDir)
	mockDocker := &mockDockerClient{
		listFunc: func(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
			return client.ContainerListResult{
				Items: []container.Summary{
					{
						ID:    "mock-id",
						State: "running",
						Labels: map[string]string{
							"com.docker.compose.project": projName,
							"com.docker.compose.service": "app",
						},
					},
				},
			}, nil
		},
		inspectFunc: func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			return client.ContainerInspectResult{
				Container: container.InspectResponse{
					State: &container.State{
						Running: true,
					},
				},
			}, nil
		},
	}

	engine := NewEngine(tempDir, composePath, env, comp, mockDocker)
	report := engine.Run(context.Background())

	if report.Status != output.StatusHealthy {
		t.Errorf("expected status healthy when missing variables have compose defaults, got: %s", report.Status)
		for _, check := range report.Checks {
			if check.Status == output.CheckFailed {
				t.Logf("Failed check: %s (%s) - %s", check.Name, check.Group, check.Error)
			}
		}
	}
}

func TestEngineOwnContainerPortBypass(t *testing.T) {
	tempDir := t.TempDir()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on local port: %v", err)
	}
	defer l.Close()

	_, portStr, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}

	composeContent := fmt.Sprintf(`
services:
  web:
    ports:
      - "%s:80"
`, portStr)

	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse compose: %v", err)
	}

	projName := filepath.Base(tempDir)
	portVal, _ := strconv.Atoi(portStr)

	mockDockerA := &mockDockerClient{
		listFunc: func(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
			return client.ContainerListResult{
				Items: []container.Summary{
					{
						ID:    "mock-id",
						State: "running",
						Labels: map[string]string{
							"com.docker.compose.project": projName,
							"com.docker.compose.service": "web",
						},
						Ports: []container.PortSummary{
							{
								PublicPort: uint16(portVal),
								Type:       "tcp",
							},
						},
					},
				},
			}, nil
		},
		inspectFunc: func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			return client.ContainerInspectResult{
				Container: container.InspectResponse{
					State: &container.State{
						Running: true,
					},
				},
			}, nil
		},
	}

	engineA := NewEngine(tempDir, composePath, nil, comp, mockDockerA)
	reportA := engineA.Run(context.Background())

	if reportA.Status != output.StatusHealthy {
		t.Errorf("expected status healthy in Scenario A (bypass active), got: %s", reportA.Status)
		for _, check := range reportA.Checks {
			if check.Status == output.CheckFailed {
				t.Logf("Failed check: %s (%s) - %s", check.Name, check.Group, check.Error)
			}
		}
	}

	mockDockerB := &mockDockerClient{
		listFunc: func(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
			return client.ContainerListResult{
				Items: []container.Summary{},
			}, nil
		},
		inspectFunc: func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			return client.ContainerInspectResult{}, fmt.Errorf("not found")
		},
	}

	engineB := NewEngine(tempDir, composePath, nil, comp, mockDockerB)
	reportB := engineB.Run(context.Background())

	if reportB.Status != output.StatusEnvironmentBroken {
		t.Errorf("expected status environment_broken in Scenario B (no bypass), got: %s", reportB.Status)
	}

	foundCollision := false
	for _, check := range reportB.Checks {
		if check.Status == output.CheckFailed && check.Group == "Network & Port Availability" && strings.Contains(check.Name, "Port Collision") {
			foundCollision = true
			break
		}
	}
	if !foundCollision {
		t.Error("expected to find port collision failure in Scenario B")
	}
}

func TestEnginePortBindErrorState(t *testing.T) {
	tempDir := t.TempDir()

	// Use a random free port for testing. Since we don't listen on it, it's now available.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	_, portStr, _ := net.SplitHostPort(l.Addr().String())
	l.Close() // Close immediately so the port is free

	composeContent := fmt.Sprintf(`
services:
  web:
    ports:
      - "%s:80"
`, portStr)

	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse compose: %v", err)
	}

	projName := filepath.Base(tempDir)
	mockDocker := &mockDockerClient{
		listFunc: func(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
			return client.ContainerListResult{
				Items: []container.Summary{
					{
						ID:    "mock-id",
						State: "exited",
						Labels: map[string]string{
							"com.docker.compose.project": projName,
							"com.docker.compose.service": "web",
						},
					},
				},
			}, nil
		},
		inspectFunc: func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			return client.ContainerInspectResult{
				Container: container.InspectResponse{
					State: &container.State{
						Running: false,
						Status:  "exited",
						Error:   fmt.Sprintf("driver failed programming external connectivity: port %s is already allocated", portStr),
					},
				},
			}, nil
		},
	}

	engine := NewEngine(tempDir, composePath, nil, comp, mockDocker)
	report := engine.Run(context.Background())

	if report.Status != output.StatusEnvironmentBroken {
		t.Errorf("expected environment_broken status, got: %s", report.Status)
	}

	foundSpecificError := false
	for _, check := range report.Checks {
		if check.Status == output.CheckFailed && check.Group == "Network & Port Availability" {
			if check.Name == "Service web failed to start" &&
				strings.Contains(check.Error, "port is now available") &&
				strings.Contains(check.Mitigation, "Simply restart the service") {
				foundSpecificError = true
				break
			}
		}
	}

	if !foundSpecificError {
		t.Error("expected to find port-specific service start failure with 'now available' mitigation in report")
		for _, check := range report.Checks {
			if check.Status == output.CheckFailed {
				t.Logf("Failed check: Name=%s, Group=%s, Error=%s, Mitigation=%s", check.Name, check.Group, check.Error, check.Mitigation)
			}
		}
	}
}

func TestEnginePortBindErrorStateWithActiveCollision(t *testing.T) {
	tempDir := t.TempDir()

	// Occupy a port during execution to trigger an active collision
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on local port: %v", err)
	}
	defer l.Close()

	_, portStr, _ := net.SplitHostPort(l.Addr().String())

	composeContent := fmt.Sprintf(`
services:
  web:
    ports:
      - "%s:80"
`, portStr)

	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse compose: %v", err)
	}

	projName := filepath.Base(tempDir)
	mockDocker := &mockDockerClient{
		listFunc: func(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
			return client.ContainerListResult{
				Items: []container.Summary{
					{
						ID:    "mock-id",
						State: "exited",
						Labels: map[string]string{
							"com.docker.compose.project": projName,
							"com.docker.compose.service": "web",
						},
					},
				},
			}, nil
		},
		inspectFunc: func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			return client.ContainerInspectResult{
				Container: container.InspectResponse{
					State: &container.State{
						Running: false,
						Status:  "exited",
						Error:   fmt.Sprintf("driver failed programming external connectivity: port %s is already allocated", portStr),
					},
				},
			}, nil
		},
	}

	engine := NewEngine(tempDir, composePath, nil, comp, mockDocker)
	report := engine.Run(context.Background())

	if report.Status != output.StatusEnvironmentBroken {
		t.Errorf("expected environment_broken status, got: %s", report.Status)
	}

	foundSpecificError := false
	for _, check := range report.Checks {
		if check.Status == output.CheckFailed && check.Group == "Network & Port Availability" {
			if check.Name == "Service web failed to start" &&
				strings.Contains(check.Error, "due to host port collision") &&
				strings.Contains(check.Mitigation, "Stop the process occupying the port") {
				foundSpecificError = true
				break
			}
		}
	}

	if !foundSpecificError {
		t.Error("expected to find port-specific service start failure with 'stop process' mitigation in report")
		for _, check := range report.Checks {
			if check.Status == output.CheckFailed {
				t.Logf("Failed check: Name=%s, Group=%s, Error=%s, Mitigation=%s", check.Name, check.Group, check.Error, check.Mitigation)
			}
		}
	}
}
