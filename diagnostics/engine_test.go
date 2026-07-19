package diagnostics

import (
	"context"
	"os"
	"path/filepath"
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
      - DB_USER
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
