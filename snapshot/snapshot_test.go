package snapshot

import (
	"bytes"
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
