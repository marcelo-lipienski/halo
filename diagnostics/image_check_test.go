package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/output"
)

func TestCheckImageTags(t *testing.T) {
	tests := []struct {
		name           string
		services       map[string]config.ComposeService
		expectedStatus map[string]output.CheckStatus // service name -> check status
		expectedErrors map[string]string             // service name -> partial error string
	}{
		{
			name:     "no services",
			services: nil,
		},
		{
			name: "pinned tags and digests",
			services: map[string]config.ComposeService{
				"web":   {Image: "nginx:1.25.1"},
				"db":    {Image: "postgres:16-alpine"},
				"cache": {Image: "redis@sha256:56885dfb221085025a176882c5f114cf39a3f25bdfeb5ff68f0cb3c4f526369c"},
				"api":   {Image: "localhost:5000/my-app:v1.0.0"},
			},
			expectedStatus: map[string]output.CheckStatus{
				"Image Security: web":   output.CheckPassed,
				"Image Security: db":    output.CheckPassed,
				"Image Security: cache": output.CheckPassed,
				"Image Security: api":   output.CheckPassed,
			},
		},
		{
			name: "missing tag implicitly latest",
			services: map[string]config.ComposeService{
				"web": {Image: "nginx"},
				"api": {Image: "localhost:5000/my-app"},
			},
			expectedStatus: map[string]output.CheckStatus{
				"Image Security: web": output.CheckWarning,
				"Image Security: api": output.CheckWarning,
			},
			expectedErrors: map[string]string{
				"Image Security: web": "without an explicit tag (implicitly uses latest)",
				"Image Security: api": "without an explicit tag (implicitly uses latest)",
			},
		},
		{
			name: "mutable tags",
			services: map[string]config.ComposeService{
				"web":   {Image: "nginx:latest"},
				"db":    {Image: "postgres:dev"},
				"api":   {Image: "localhost:5000/my-app:staging"},
				"built": {Image: ""}, // skipped
			},
			expectedStatus: map[string]output.CheckStatus{
				"Image Security: web": output.CheckWarning,
				"Image Security: db":  output.CheckWarning,
				"Image Security: api": output.CheckWarning,
			},
			expectedErrors: map[string]string{
				"Image Security: web": "uses image 'nginx:latest' with mutable tag 'latest'",
				"Image Security: db":  "uses image 'postgres:dev' with mutable tag 'dev'",
				"Image Security: api": "uses image 'localhost:5000/my-app:staging' with mutable tag 'staging'",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comp := &config.ComposeConfig{
				Services: tc.services,
			}
			engine := NewEngine(".", "docker-compose.yml", nil, comp, nil)
			results := engine.CheckImageTags()

			if tc.services == nil {
				if len(results) != 0 {
					t.Errorf("expected 0 results, got %d", len(results))
				}
				return
			}

			// We expect one check per service that has an image
			expectedCount := 0
			for _, svc := range tc.services {
				if svc.Image != "" {
					expectedCount++
				}
			}
			if len(results) != expectedCount {
				t.Errorf("expected %d results, got %d", expectedCount, len(results))
			}

			for _, res := range results {
				expectedStatus, ok := tc.expectedStatus[res.Name]
				if !ok {
					t.Errorf("unexpected check name: %q", res.Name)
					continue
				}
				if res.Status != expectedStatus {
					t.Errorf("check %q status = %s, expected %s", res.Name, res.Status, expectedStatus)
				}
				if expectedErr, hasErr := tc.expectedErrors[res.Name]; hasErr {
					if res.Error == "" || !contains(res.Error, expectedErr) {
						t.Errorf("check %q error = %q, expected to contain %q", res.Name, res.Error, expectedErr)
					}
					if res.Mitigation == "" {
						t.Errorf("check %q expected to have a mitigation", res.Name)
					}
				}
			}
		})
	}
}

func TestEngineRunsImageTagsCheck(t *testing.T) {
	tempDir := t.TempDir()
	comp := &config.ComposeConfig{
		Services: map[string]config.ComposeService{
			"web": {Image: "nginx:latest"},
		},
	}

	engine := NewEngine(tempDir, "docker-compose.yml", nil, comp, nil)
	report := engine.Run(context.Background())

	hasImageCheck := false
	for _, check := range report.Checks {
		if check.Name == "Image Security: web" {
			hasImageCheck = true
			if check.Status != output.CheckWarning {
				t.Errorf("expected Image Security to be Warning, got %s", check.Status)
			}
		}
	}
	if !hasImageCheck {
		t.Error("expected report to execute image tag checks")
	}
}

func contains(s, substr string) bool {
	// simple helper to avoid importing strings
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestCheckDockerfileImageTags(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Create a Dockerfile with a mutable base image
	dockerfileContent := `
# A comment
FROM --platform=linux/amd64 nginx:latest AS base
FROM base AS runner
RUN echo "hello"
`
	if err := os.WriteFile(filepath.Join(tempDir, "Dockerfile"), []byte(dockerfileContent), 0644); err != nil {
		t.Fatal(err)
	}

	comp := &config.ComposeConfig{
		Services: map[string]config.ComposeService{
			"web": {
				Build: config.ComposeBuild{
					Context:    ".",
					Dockerfile: "Dockerfile",
				},
			},
		},
	}

	engine := NewEngine(tempDir, filepath.Join(tempDir, "docker-compose.yml"), nil, comp, nil)
	results := engine.CheckImageTags()

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	res := results[0]
	if res.Name != "Image Security (Dockerfile): web" {
		t.Errorf("expected check name 'Image Security (Dockerfile): web', got %q", res.Name)
	}
	if res.Status != output.CheckWarning {
		t.Errorf("expected check status warning, got %s", res.Status)
	}
	if !contains(res.Error, "mutable base image(s) in Dockerfile: base image 'nginx:latest' (mutable tag 'latest')") {
		t.Errorf("unexpected error message: %q", res.Error)
	}

	// 2. Create a Dockerfile with a pinned base image
	dockerfileContent2 := `
FROM golang:1.20-alpine
`
	if err := os.WriteFile(filepath.Join(tempDir, "Dockerfile.pinned"), []byte(dockerfileContent2), 0644); err != nil {
		t.Fatal(err)
	}

	comp2 := &config.ComposeConfig{
		Services: map[string]config.ComposeService{
			"api": {
				Build: config.ComposeBuild{
					Context:    ".",
					Dockerfile: "Dockerfile.pinned",
				},
			},
		},
	}

	engine2 := NewEngine(tempDir, filepath.Join(tempDir, "docker-compose.yml"), nil, comp2, nil)
	results2 := engine2.CheckImageTags()

	if len(results2) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results2))
	}
	if results2[0].Status != output.CheckPassed {
		t.Errorf("expected check to pass, got %s. Error: %s", results2[0].Status, results2[0].Error)
	}
}
