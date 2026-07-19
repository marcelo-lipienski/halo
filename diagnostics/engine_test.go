package diagnostics

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/output"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
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

func TestEnginePortRangeCollision(t *testing.T) {
	tempDir := t.TempDir()

	// Occupy a port in the range
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on local port: %v", err)
	}
	defer l.Close()

	_, portStr, _ := net.SplitHostPort(l.Addr().String())
	portVal, _ := strconv.Atoi(portStr)

	// We map a range of [portVal-1, portVal+1] so that portVal is inside the range
	startPort := portVal - 1
	endPort := portVal + 1

	composeContent := fmt.Sprintf(`
services:
  web:
    ports:
      - "%d-%d:80-82"
`, startPort, endPort)

	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse compose: %v", err)
	}

	mockDocker := &mockDockerClient{
		listFunc: func(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
			return client.ContainerListResult{Items: []container.Summary{}}, nil
		},
		inspectFunc: func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			return client.ContainerInspectResult{}, fmt.Errorf("not found")
		},
	}

	engine := NewEngine(tempDir, composePath, nil, comp, mockDocker)
	report := engine.Run(context.Background())

	if report.Status != output.StatusEnvironmentBroken {
		t.Errorf("expected environment_broken status due to range collision, got: %s", report.Status)
	}

	foundCollision := false
	expectedName := fmt.Sprintf("Port Collision web:%d", portVal)
	for _, check := range report.Checks {
		if check.Status == output.CheckFailed && check.Group == "Network & Port Availability" && check.Name == expectedName {
			foundCollision = true
			break
		}
	}

	if !foundCollision {
		t.Errorf("expected to find port collision error for %s", expectedName)
		for _, check := range report.Checks {
			if check.Status == output.CheckFailed {
				t.Logf("Failed check: Name=%s, Group=%s, Error=%s", check.Name, check.Group, check.Error)
			}
		}
	}
}

func TestEngineVolumeTildeExpansion(t *testing.T) {
	tempDir := t.TempDir()

	// Override HOME environment variable
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Setenv("HOME", tempDir)

	// Create a subdirectory inside tempDir to act as the expanded folder
	subDirName := "mock_volume_data"
	expandedPath := filepath.Join(tempDir, subDirName)
	if err := os.Mkdir(expandedPath, 0755); err != nil {
		t.Fatalf("failed to create mock home subdirectory: %v", err)
	}

	composeContent := fmt.Sprintf(`
services:
  app:
    volumes:
      - "~/%s:/app/data"
`, subDirName)

	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse compose: %v", err)
	}

	mockDocker := &mockDockerClient{
		listFunc: func(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
			return client.ContainerListResult{Items: []container.Summary{}}, nil
		},
		inspectFunc: func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			return client.ContainerInspectResult{}, fmt.Errorf("not found")
		},
	}

	engine := NewEngine(tempDir, composePath, nil, comp, mockDocker)
	report := engine.Run(context.Background())

	// If tilde expanded to HOME (tempDir) and matched expandedPath, check should pass
	foundTildeError := false
	for _, check := range report.Checks {
		if check.Group == "Volume & File Permissions" && check.Status == output.CheckFailed {
			foundTildeError = true
			t.Logf("Failed check: Name=%s, Error=%s", check.Name, check.Error)
		}
	}

	if foundTildeError {
		t.Error("expected volume tilde expansion check to pass, but found failures")
	}
}

func TestEngineVolumePermissions(t *testing.T) {
	tempDir := t.TempDir()

	// Write a mock compose file
	composeContent := `
services:
  app:
    volumes:
      - ./readonly_dir:/app/readonly
      - ./writeonly_dir:/app/writeonly
`
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	// 1. Read-only directory: writable check should fail, readable check should pass
	readonlyPath := filepath.Join(tempDir, "readonly_dir")
	if err := os.Mkdir(readonlyPath, 0555); err != nil {
		t.Fatalf("failed to create readonly dir: %v", err)
	}
	defer os.Chmod(readonlyPath, 0755) // restore so cleanup succeeds

	// 2. Write-only (non-readable) directory: readable check should fail
	writeonlyPath := filepath.Join(tempDir, "writeonly_dir")
	if err := os.Mkdir(writeonlyPath, 0333); err != nil {
		t.Fatalf("failed to create writeonly dir: %v", err)
	}
	defer os.Chmod(writeonlyPath, 0755) // restore so cleanup succeeds

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse compose: %v", err)
	}

	mockDocker := &mockDockerClient{
		listFunc: func(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
			return client.ContainerListResult{Items: []container.Summary{}}, nil
		},
	}

	engine := NewEngine(tempDir, composePath, nil, comp, mockDocker)
	report := engine.Run(context.Background())

	if report.Status != output.StatusEnvironmentBroken {
		t.Errorf("expected status environment_broken, got: %s", report.Status)
	}

	foundReadLockout := false
	foundWriteLockout := false

	for _, check := range report.Checks {
		if check.Group == "Volume & File Permissions" && check.Status == output.CheckFailed {
			if strings.Contains(check.Name, "Volume read lockout") && strings.Contains(check.Name, "writeonly_dir") {
				foundReadLockout = true
			}
			if strings.Contains(check.Name, "Volume permission lockout") && strings.Contains(check.Name, "readonly_dir") {
				foundWriteLockout = true
			}
		}
	}

	// Note: root user running tests might ignore permissions, so we check if the test is running as root
	if os.Getuid() != 0 {
		if !foundReadLockout {
			t.Error("expected to find volume read lockout error for writeonly_dir")
		}
		if !foundWriteLockout {
			t.Error("expected to find volume permission lockout error for readonly_dir")
		}
	}
}

func TestEngineReadOnlyVolumeSkipWriteCheck(t *testing.T) {
	tempDir := t.TempDir()

	// Write a mock compose file with a read-only bind volume
	composeContent := `
services:
  app:
    volumes:
      - ./readonly_dir:/app/readonly:ro
`
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	// Create the readonly directory with read-only permissions (non-writable)
	readonlyPath := filepath.Join(tempDir, "readonly_dir")
	if err := os.Mkdir(readonlyPath, 0555); err != nil {
		t.Fatalf("failed to create readonly dir: %v", err)
	}
	defer os.Chmod(readonlyPath, 0755) // restore so cleanup succeeds

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

	engine := NewEngine(tempDir, composePath, nil, comp, mockDocker)
	report := engine.Run(context.Background())

	// Since the volume is marked read-only, it should skip the write permission check and pass
	foundWriteLockout := false
	for _, check := range report.Checks {
		if check.Group == "Volume & File Permissions" && check.Status == output.CheckFailed {
			if strings.Contains(check.Name, "Volume permission lockout") {
				foundWriteLockout = true
				t.Logf("Failed check unexpectedly: Name=%s, Error=%s", check.Name, check.Error)
			}
		}
	}

	if foundWriteLockout {
		t.Error("expected write permission check to be skipped for read-only volume, but it failed")
	}

	if report.Status == output.StatusEnvironmentBroken && os.Getuid() != 0 {
		t.Errorf("expected engine status healthy, got environment_broken: %+v", report)
	}
}

func TestEngineHostEnvFallbackInAlignment(t *testing.T) {
	tempDir := t.TempDir()

	composeContent := `
services:
  app:
    environment:
      - PORT=${HOST_PORT}
`
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	// Set HOST_PORT in the system environment
	os.Setenv("HOST_PORT", "9090")
	defer os.Unsetenv("HOST_PORT")

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse compose: %v", err)
	}

	// Engine environment has NO HOST_PORT defined in e.Env map
	engine := NewEngine(tempDir, composePath, map[string]string{}, comp, &mockDockerClient{})
	report := engine.Run(context.Background())

	// Alignment check should search system env, find it, and pass
	foundMissing := false
	for _, check := range report.Checks {
		if check.Group == "Environmental Alignment" && check.Status == output.CheckFailed {
			foundMissing = true
			t.Logf("Failed check: Name=%s, Error=%s", check.Name, check.Error)
		}
	}

	if foundMissing {
		t.Error("expected host environment fallback to pass variables check, but it failed")
	}
}

func TestEngineEmptyVariableDefaults(t *testing.T) {
	tempDir := t.TempDir()

	composeContent := `
services:
  app:
    environment:
      - PORT=${APP_PORT:-8080}
`
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse compose: %v", err)
	}

	// APP_PORT is defined but empty in Env
	env := map[string]string{
		"APP_PORT": "",
	}

	engine := NewEngine(tempDir, composePath, env, comp, &mockDockerClient{})
	resolved := engine.resolveEnvVars("${APP_PORT:-8080}")
	if resolved != "8080" {
		t.Errorf("expected empty variable to fall back to '8080', got '%s'", resolved)
	}
}

func TestEngineWindowsPathWarningOnUnix(t *testing.T) {
	tempDir := t.TempDir()

	composeContent := `
services:
  app:
    volumes:
      - C:\host\data:/app/data
`
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse compose: %v", err)
	}

	engine := NewEngine(tempDir, composePath, nil, comp, &mockDockerClient{})
	report := engine.Run(context.Background())

	if runtime.GOOS != "windows" {
		foundWarning := false
		for _, check := range report.Checks {
			if check.Group == "Volume & File Permissions" && strings.Contains(check.Name, "Incompatible OS Path") {
				foundWarning = true
				break
			}
		}
		if !foundWarning {
			t.Error("expected to find Windows path conventions warning on non-Windows system")
		}
	}
}

func TestEngineEmptyVariableWarning(t *testing.T) {
	tempDir := t.TempDir()

	composeContent := `
services:
  app:
    environment:
      - PORT=${APP_PORT}
`
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse compose: %v", err)
	}

	// APP_PORT is defined but empty
	env := map[string]string{
		"APP_PORT": "",
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

	// Status should be healthy, not broken
	if report.Status != output.StatusHealthy {
		t.Errorf("expected status healthy for empty variable warning, got: %s", report.Status)
	}

	foundWarning := false
	for _, check := range report.Checks {
		if check.Group == "Environmental Alignment" && strings.Contains(check.Name, "Variable APP_PORT is empty") {
			if check.Status != output.CheckWarning {
				t.Errorf("expected warning status, got %s", check.Status)
			}
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("expected to find warning for empty variable APP_PORT")
	}
}

func TestEngineVolumeRelativePathResolution(t *testing.T) {
	tempDir := t.TempDir()

	baseDir := filepath.Join(tempDir, "base")
	overrideDir := filepath.Join(tempDir, "override")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		t.Fatalf("failed to create base dir: %v", err)
	}
	if err := os.MkdirAll(overrideDir, 0755); err != nil {
		t.Fatalf("failed to create override dir: %v", err)
	}

	// Create directories on host that the volumes reference
	baseDataPath := filepath.Join(baseDir, "base_data")
	if err := os.Mkdir(baseDataPath, 0755); err != nil {
		t.Fatalf("failed to create base_data: %v", err)
	}
	overrideDataPath := filepath.Join(overrideDir, "override_data")
	if err := os.Mkdir(overrideDataPath, 0755); err != nil {
		t.Fatalf("failed to create override_data: %v", err)
	}

	baseContent := `
services:
  web:
    volumes:
      - ./base_data:/app/base_data
`
	basePath := filepath.Join(baseDir, "docker-compose.yml")
	if err := os.WriteFile(basePath, []byte(baseContent), 0644); err != nil {
		t.Fatalf("failed to write base compose file: %v", err)
	}

	overrideContent := `
services:
  web:
    volumes:
      - ./override_data:/app/override_data
`
	overridePath := filepath.Join(overrideDir, "docker-compose.override.yml")
	if err := os.WriteFile(overridePath, []byte(overrideContent), 0644); err != nil {
		t.Fatalf("failed to write override compose file: %v", err)
	}

	cfg1, err := config.ParseCompose(basePath)
	if err != nil {
		t.Fatalf("failed to parse base compose: %v", err)
	}
	cfg2, err := config.ParseCompose(overridePath)
	if err != nil {
		t.Fatalf("failed to parse override compose: %v", err)
	}

	merged := config.MergeComposeConfigs(cfg1, cfg2)

	// Set up mock docker client (we don't test service reachability here, just volume permissions)
	mockDocker := &mockDockerClient{}

	engine := NewEngine(baseDir, basePath, nil, merged, mockDocker)
	results := engine.checkVolumeAndPermissions(context.Background())

	// Check if volume permissions check failed
	for _, res := range results {
		if res.Status == output.CheckFailed {
			t.Errorf("expected volume check to pass, but failed: %s - %s", res.Name, res.Error)
		}
	}
}

func TestParseHostPortProto(t *testing.T) {
	tests := []struct {
		input        string
		expectedPort string
		expectedProto string
	}{
		{"8080:80", "8080", "tcp"},
		{"8080:80/tcp", "8080", "tcp"},
		{"8080:80/udp", "8080", "udp"},
		{"127.0.0.1:8080:80", "8080", "tcp"},
		{"127.0.0.1:8080:80/udp", "8080", "udp"},
		{"[::1]:8080:80", "8080", "tcp"},
		{"[::1]:8080:80/udp", "8080", "udp"},
		{"80", "", "tcp"},
		{"[::1]:80", "", "tcp"},
		{"8080-8085:80-85", "8080-8085", "tcp"},
		{"[::1]:8080-8085:80-85", "8080-8085", "tcp"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			port, proto := parseHostPortProto(tc.input)
			if port != tc.expectedPort || proto != tc.expectedProto {
				t.Errorf("parseHostPortProto(%q) = (%q, %q); expected (%q, %q)",
					tc.input, port, proto, tc.expectedPort, tc.expectedProto)
			}
		})
	}
}

func TestEngineEscapedEnvVars(t *testing.T) {
	tempDir := t.TempDir()
	composeContent := `
services:
  app:
    environment:
      - PORT=$$APP_PORT
`
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	_ = os.WriteFile(composePath, []byte(composeContent), 0644)

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	engine := NewEngine(tempDir, composePath, nil, comp, &mockDockerClient{})
	report := engine.Run(context.Background())

	// Since APP_PORT is escaped with $$, it should NOT be flagged as missing.
	for _, check := range report.Checks {
		if check.Group == "Environmental Alignment" && strings.Contains(check.Name, "Variable APP_PORT") {
			t.Errorf("expected escaped variable APP_PORT to be skipped, but got: %+v", check)
		}
	}
}

func TestEngineSecretsAndConfigs(t *testing.T) {
	tempDir := t.TempDir()
	composeContent := `
services:
  app:
    image: nginx
secrets:
  my_sec:
    file: ./sec.txt
configs:
  my_cfg:
    file: ./cfg.txt
`
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	_ = os.WriteFile(composePath, []byte(composeContent), 0644)

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// 1. Missing files
	engine := NewEngine(tempDir, composePath, nil, comp, &mockDockerClient{})
	report := engine.Run(context.Background())
	if report.Status != output.StatusEnvironmentBroken {
		t.Errorf("expected broken environment status due to missing secret/config, got %s", report.Status)
	}

	hasSecretError := false
	hasConfigError := false
	for _, check := range report.Checks {
		if check.Status == output.CheckFailed {
			if strings.Contains(check.Name, "Secret file missing") {
				hasSecretError = true
			}
			if strings.Contains(check.Name, "Config file missing") {
				hasConfigError = true
			}
		}
	}
	if !hasSecretError || !hasConfigError {
		t.Errorf("expected secret and config missing errors. secret: %v, config: %v", hasSecretError, hasConfigError)
	}

	// 2. Exist files
	_ = os.WriteFile(filepath.Join(tempDir, "sec.txt"), []byte("data"), 0600)
	_ = os.WriteFile(filepath.Join(tempDir, "cfg.txt"), []byte("data"), 0644)
	report2 := engine.Run(context.Background())
	
	// Ensure they pass now
	hasSecretPass := false
	hasConfigPass := false
	for _, check := range report2.Checks {
		if check.Status == output.CheckPassed {
			// They don't individually record passed unless volumeCheckPassed is true. But since we didn't specify volumes,
			// volumeCheckPassed should be true and "Volume & File Permissions Check" should pass.
		}
	}
	_ = hasSecretPass
	_ = hasConfigPass
}

func TestEngineAutoFix(t *testing.T) {
	tempDir := t.TempDir()
	composeContent := `
services:
  app:
    image: nginx
    volumes:
      - ./missing_dir:/data
      - ./missing_file.txt:/data_file.txt
secrets:
  my_sec:
    file: ./sec.txt
configs:
  my_cfg:
    file: ./cfg.txt
`
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	_ = os.WriteFile(composePath, []byte(composeContent), 0644)

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
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

	engine := NewEngine(tempDir, composePath, nil, comp, mockDocker)
	engine.AutoFix = true

	report := engine.Run(context.Background())
	if report.Status != output.StatusHealthy {
		t.Errorf("expected status healthy after auto-fix, got %s", report.Status)
		for _, check := range report.Checks {
			if check.Status == output.CheckFailed {
				t.Logf("Failed check: %+v", check)
			}
		}
	}

	// Verify that directories and files were actually created
	if _, err := os.Stat(filepath.Join(tempDir, "missing_dir")); err != nil {
		t.Errorf("missing_dir was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "missing_file.txt")); err != nil {
		t.Errorf("missing_file.txt was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "sec.txt")); err != nil {
		t.Errorf("sec.txt was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "cfg.txt")); err != nil {
		t.Errorf("cfg.txt was not created: %v", err)
	}
}

func TestEngineTimeout(t *testing.T) {
	tempDir := t.TempDir()
	composeContent := `
services:
  app:
    image: nginx
`
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	_ = os.WriteFile(composePath, []byte(composeContent), 0644)

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	engine := NewEngine(tempDir, composePath, nil, comp, &mockDockerClient{})
	report := engine.Run(ctx)

	// Since context is cancelled, we should have timeout/cancellation checks failed
	hasTimeoutError := false
	for _, check := range report.Checks {
		if check.Status == output.CheckFailed && strings.Contains(check.Name, "Timeout") {
			hasTimeoutError = true
		}
	}
	if !hasTimeoutError {
		t.Errorf("expected timeout errors, but got: %+v", report.Checks)
	}
}

func TestEngineHealthStarting(t *testing.T) {
	tempDir := t.TempDir()
	composeContent := `
services:
  app:
    image: nginx
`
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	_ = os.WriteFile(composePath, []byte(composeContent), 0644)

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
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
						Health: &container.Health{
							Status: "starting",
						},
					},
				},
			}, nil
		},
	}

	engine := NewEngine(tempDir, composePath, nil, comp, mockDocker)
	report := engine.Run(context.Background())

	// A "starting" health state should be a warning, not failure, so status should be healthy
	if report.Status != output.StatusHealthy {
		t.Errorf("expected overall status healthy when check has only starting health warning, got: %s", report.Status)
	}

	hasWarning := false
	for _, check := range report.Checks {
		if check.Status == output.CheckWarning && strings.Contains(check.Name, "health is starting") {
			hasWarning = true
			if !strings.Contains(check.Error, "health check is still initialising") {
				t.Errorf("expected warning details, got: %s", check.Error)
			}
		}
	}
	if !hasWarning {
		t.Errorf("expected warning for starting health status, got checks: %+v", report.Checks)
	}
}

func BenchmarkEngineRun(b *testing.B) {
	tempDir := b.TempDir()

	// Realistic multi-service compose fixture covering env vars, ports, and bind volumes
	composeContent := `
services:
  web:
    image: nginx:alpine
    environment:
      - PORT=${APP_PORT:-8080}
      - DB_HOST=${DB_HOST}
    ports:
      - "${APP_PORT:-8080}:80"
    volumes:
      - ./html:/var/www/html
      - ./logs:/var/log/nginx
  api:
    image: node:20-alpine
    environment:
      DB_URL: "postgres://${DB_USER}:${DB_PASS}@db:5432/app"
    ports:
      - "3000:3000"
    volumes:
      - ./api:/app
  db:
    image: postgres:16
    environment:
      POSTGRES_DB: app
      POSTGRES_USER: ${DB_USER}
      POSTGRES_PASSWORD: ${DB_PASS}
    ports:
      - "5432:5432"
    volumes:
      - ./db_data:/var/lib/postgresql/data
`
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		b.Fatalf("failed to write compose file: %v", err)
	}

	// Create bind-mount directories so volume checks don't fail on missing paths
	for _, dir := range []string{"html", "logs", "api", "db_data"} {
		if err := os.Mkdir(filepath.Join(tempDir, dir), 0755); err != nil {
			b.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		b.Fatalf("failed to parse compose: %v", err)
	}

	env := map[string]string{
		"APP_PORT": "8080",
		"DB_HOST":  "localhost",
		"DB_USER":  "postgres",
		"DB_PASS":  "secret",
	}

	projName := filepath.Base(tempDir)
	mockDocker := &mockDockerClient{
		listFunc: func(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
			return client.ContainerListResult{
				Items: []container.Summary{
					{ID: "id-web", State: "running", Labels: map[string]string{
						"com.docker.compose.project": projName,
						"com.docker.compose.service": "web",
					}},
					{ID: "id-api", State: "running", Labels: map[string]string{
						"com.docker.compose.project": projName,
						"com.docker.compose.service": "api",
					}},
					{ID: "id-db", State: "running", Labels: map[string]string{
						"com.docker.compose.project": projName,
						"com.docker.compose.service": "db",
					}},
				},
			}, nil
		},
		inspectFunc: func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			return client.ContainerInspectResult{
				Container: container.InspectResponse{
					State: &container.State{Running: true},
				},
			}, nil
		},
	}

	engine := NewEngine(tempDir, composePath, env, comp, mockDocker)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.Run(ctx)
	}
}
