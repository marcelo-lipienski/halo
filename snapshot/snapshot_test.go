package snapshot

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiff(t *testing.T) {
	now := time.Now()

	oldSnap := &EnvironmentSnapshot{
		CreatedAt: now,
		Project:   "test-project",
		Files: map[string]FileSnapshot{
			".env":               {Path: ".env", Size: 100, Hash: "hash1"},
			"docker-compose.yml": {Path: "docker-compose.yml", Size: 200, Hash: "hash2"},
		},
		Variables: map[string]map[string]string{
			".env": {
				"DB_HOST": "localhost",
				"DB_PASS": "secret",
			},
		},
		Ports: []PortSnapshot{
			{Service: "web", HostPort: "80", Protocol: "tcp", IsOccupied: true, ProcessName: "nginx", PID: 123},
		},
		Services: map[string]ContainerSnapshot{
			"web": {ContainerID: "id1", ContainerName: "web-c", State: "running", Status: "healthy", ImageID: "img1"},
		},
	}

	newSnap := &EnvironmentSnapshot{
		CreatedAt: now.Add(time.Minute),
		Project:   "test-project",
		Files: map[string]FileSnapshot{
			".env":                        {Path: ".env", Size: 110, Hash: "hash1-modified"},
			"docker-compose.override.yml": {Path: "docker-compose.override.yml", Size: 50, Hash: "hash3"},
		},
		Variables: map[string]map[string]string{
			".env": {
				"DB_HOST": "localhost",
				"DB_PASS": "secret-new",
				"NEW_VAR": "value",
			},
		},
		Ports: []PortSnapshot{
			{Service: "web", HostPort: "80", Protocol: "tcp", IsOccupied: false},
		},
		Services: map[string]ContainerSnapshot{
			"web": {ContainerID: "id1", ContainerName: "web-c", State: "exited", Status: "", ImageID: "img1"},
		},
	}

	diff := Diff(oldSnap, newSnap)

	// Check files diff
	if len(diff.Files) != 3 {
		t.Errorf("Expected 3 file changes, got %d", len(diff.Files))
	}

	var modifiedEnv, removedCompose, addedOverride bool
	for _, f := range diff.Files {
		switch f.Path {
		case ".env":
			if f.Change == "modified" {
				modifiedEnv = true
			}
		case "docker-compose.yml":
			if f.Change == "removed" {
				removedCompose = true
			}
		case "docker-compose.override.yml":
			if f.Change == "added" {
				addedOverride = true
			}
		}
	}
	if !modifiedEnv || !removedCompose || !addedOverride {
		t.Errorf("Files diff failed: modifiedEnv=%v, removedCompose=%v, addedOverride=%v", modifiedEnv, removedCompose, addedOverride)
	}

	// Check variables diff
	if len(diff.Variables) != 2 {
		t.Errorf("Expected 2 variable changes, got %d", len(diff.Variables))
	}

	// Check ports diff
	if len(diff.Ports) != 1 || diff.Ports[0].Change != "status_changed" || diff.Ports[0].NewOccupied {
		t.Errorf("Ports diff failed: %+v", diff.Ports)
	}

	// Check containers diff
	if len(diff.Containers) != 1 || diff.Containers[0].Change != "state_changed" || diff.Containers[0].NewState != "exited" {
		t.Errorf("Containers diff failed: %+v", diff.Containers)
	}

	// Test RenderText
	var buf bytes.Buffer
	RenderText(&buf, diff, oldSnap.CreatedAt)
	out := buf.String()

	if !bytes.Contains(buf.Bytes(), []byte(".env (modified)")) {
		t.Errorf("RenderText missing modified file: %s", out)
	}
	if !bytes.Contains(buf.Bytes(), []byte("DB_PASS: modified")) {
		t.Errorf("RenderText missing modified var: %s", out)
	}
}

func TestCreateSnapshot(t *testing.T) {
	tempDir := t.TempDir()

	envPath := filepath.Join(tempDir, ".env")
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	envExamplePath := filepath.Join(tempDir, ".env.example")
	svcEnvPath := filepath.Join(tempDir, "svc.env")

	_ = os.WriteFile(envPath, []byte("PORT=8080\nSVC_ENV_FILE=svc.env"), 0644)
	_ = os.WriteFile(envExamplePath, []byte("PORT=placeholder"), 0644)
	_ = os.WriteFile(svcEnvPath, []byte("DB_HOST=db-host"), 0644)

	composeContent := `
services:
  web:
    image: nginx:latest
    ports:
      - "${PORT}:80"
    env_file:
      - ${SVC_ENV_FILE}
`
	_ = os.WriteFile(composePath, []byte(composeContent), 0644)

	snap, _, err := CreateSnapshot(tempDir, envPath, []string{composePath})
	if err != nil {
		t.Fatalf("unexpected error creating snapshot: %v", err)
	}

	if snap.Project != filepath.Base(tempDir) {
		t.Errorf("expected project name %q, got %q", filepath.Base(tempDir), snap.Project)
	}

	expectedFiles := []string{".env", ".env.example", "docker-compose.yml", "svc.env"}
	for _, f := range expectedFiles {
		if _, ok := snap.Files[f]; !ok {
			t.Errorf("expected file %q to be tracked in snapshot", f)
		}
	}

	envVars, ok := snap.Variables[".env"]
	if !ok {
		t.Fatal("expected .env variables in snapshot")
	}
	if envVars["PORT"] != "8080" || envVars["SVC_ENV_FILE"] != "svc.env" {
		t.Errorf("unexpected .env variables: %v", envVars)
	}

	svcVars, ok := snap.Variables["svc.env"]
	if !ok {
		t.Fatal("expected svc.env variables in snapshot")
	}
	if svcVars["DB_HOST"] != "db-host" {
		t.Errorf("unexpected svc.env variables: %v", svcVars)
	}
}

func TestDiffEdgeCases(t *testing.T) {
	now := time.Now()

	oldSnap := &EnvironmentSnapshot{
		CreatedAt: now,
		Project:   "test-project",
		Files: map[string]FileSnapshot{
			"removed-file.txt": {Path: "removed-file.txt", Size: 10, Hash: "hash-old"},
			"mod-file.txt":     {Path: "mod-file.txt", Size: 20, Hash: "hash-old"},
		},
		Variables: map[string]map[string]string{
			"config.env": {
				"REMOVED_VAR": "val1",
				"MOD_VAR":     "val2",
			},
		},
		Ports: []PortSnapshot{
			{Service: "web", HostPort: "80", Protocol: "tcp", IsOccupied: true, ProcessName: "nginx", PID: 100},
			{Service: "db", HostPort: "5432", Protocol: "tcp", IsOccupied: false},
		},
		Services: map[string]ContainerSnapshot{
			"web": {ContainerID: "id1", ContainerName: "web-c", State: "running", Status: "healthy", Image: "nginx:latest", ImageID: "img1"},
			"db":  {ContainerID: "id2", ContainerName: "db-c", State: "running", Status: "healthy", Image: "postgres:13", ImageID: "img-db-old"},
		},
	}

	newSnap := &EnvironmentSnapshot{
		CreatedAt: now.Add(time.Minute),
		Project:   "test-project",
		Files: map[string]FileSnapshot{
			"added-file.txt": {Path: "added-file.txt", Size: 15, Hash: "hash-new"},
			"mod-file.txt":   {Path: "mod-file.txt", Size: 25, Hash: "hash-new"},
		},
		Variables: map[string]map[string]string{
			"config.env": {
				"ADDED_VAR": "val3",
				"MOD_VAR":   "val2-new",
			},
		},
		Ports: []PortSnapshot{
			{Service: "web", HostPort: "80", Protocol: "tcp", IsOccupied: true, ProcessName: "nginx", PID: 101}, // PID changed
			{Service: "redis", HostPort: "6379", Protocol: "tcp", IsOccupied: true},                             // Port added
		},
		Services: map[string]ContainerSnapshot{
			"web":   {ContainerID: "id1", ContainerName: "web-c", State: "running", Status: "unhealthy", Image: "nginx:latest", ImageID: "img1"},   // Health status changed
			"db":    {ContainerID: "id2", ContainerName: "db-c", State: "running", Status: "healthy", Image: "postgres:14", ImageID: "img-db-new"}, // Image changed
			"redis": {ContainerID: "id3", ContainerName: "redis-c", State: "running", Status: "healthy", Image: "redis:alpine"},                    // Container added
		},
	}

	diff := Diff(oldSnap, newSnap)

	var hasAddedFile, hasRemovedFile, hasModFile bool
	for _, f := range diff.Files {
		switch f.Path {
		case "added-file.txt":
			if f.Change == "added" {
				hasAddedFile = true
			}
		case "removed-file.txt":
			if f.Change == "removed" {
				hasRemovedFile = true
			}
		case "mod-file.txt":
			if f.Change == "modified" {
				hasModFile = true
			}
		}
	}
	if !hasAddedFile || !hasRemovedFile || !hasModFile {
		t.Errorf("Expected file changes not found: %+v", diff.Files)
	}

	var hasAddedVar, hasRemovedVar, hasModVar bool
	for _, v := range diff.Variables {
		switch v.Key {
		case "ADDED_VAR":
			if v.Change == "added" {
				hasAddedVar = true
			}
		case "REMOVED_VAR":
			if v.Change == "removed" {
				hasRemovedVar = true
			}
		case "MOD_VAR":
			if v.Change == "modified" {
				hasModVar = true
			}
		}
	}
	if !hasAddedVar || !hasRemovedVar || !hasModVar {
		t.Errorf("Expected variable changes not found: %+v", diff.Variables)
	}

	var hasStatusChangedPort, hasAddedPort, hasRemovedPort bool
	for _, p := range diff.Ports {
		if p.Port == "80" && p.Change == "status_changed" {
			hasStatusChangedPort = true
		}
		if p.Port == "6379" && p.Change == "added" {
			hasAddedPort = true
		}
		if p.Port == "5432" && p.Change == "removed" {
			hasRemovedPort = true
		}
	}
	if !hasStatusChangedPort || !hasAddedPort || !hasRemovedPort {
		t.Errorf("Expected port changes not found: %+v", diff.Ports)
	}

	var hasHealthChanged, hasImageChanged, hasAddedContainer, hasRemovedContainer bool
	for _, c := range diff.Containers {
		if c.Service == "web" && c.Change == "health_changed" {
			hasHealthChanged = true
		}
		if c.Service == "db" && c.Change == "image_changed" {
			hasImageChanged = true
		}
		if c.Service == "redis" && c.Change == "added" {
			hasAddedContainer = true
		}
	}

	oldSnap2 := &EnvironmentSnapshot{
		Services: map[string]ContainerSnapshot{
			"db": {ContainerID: "id2", State: "running"},
		},
	}
	newSnap2 := &EnvironmentSnapshot{}
	diff2 := Diff(oldSnap2, newSnap2)
	for _, c := range diff2.Containers {
		if c.Service == "db" && c.Change == "removed" {
			hasRemovedContainer = true
		}
	}

	if !hasHealthChanged || !hasImageChanged || !hasAddedContainer || !hasRemovedContainer {
		t.Errorf("Expected container changes not found: health=%v, image=%v, added=%v, removed=%v",
			hasHealthChanged, hasImageChanged, hasAddedContainer, hasRemovedContainer)
	}
}
