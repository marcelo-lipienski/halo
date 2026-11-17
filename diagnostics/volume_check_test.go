package diagnostics

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/output"
)

func TestFixPermissionsMocking(t *testing.T) {
	origFunc := fixPermissionsFunc
	defer func() { fixPermissionsFunc = origFunc }()

	called := false
	var targetPath string
	var targetPerm os.FileMode

	fixPermissionsFunc = func(path string, perm os.FileMode) error {
		called = true
		targetPath = path
		targetPerm = perm
		return nil
	}

	err := fixPermissions("/tmp/test_file", 0644)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected fixPermissionsFunc to be called")
	}
	if targetPath != "/tmp/test_file" || targetPerm != 0644 {
		t.Errorf("got path=%s perm=%v, expected /tmp/test_file 0644", targetPath, targetPerm)
	}
}

func TestGetPermissionMitigationDirect(t *testing.T) {
	mitigation := getPermissionMitigation("/path/to/data", true, true)
	if !strings.Contains(mitigation, "/path/to/data") {
		t.Errorf("expected mitigation string to contain path, got: %s", mitigation)
	}
}

func TestIsLikelyFilePath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/app/data.txt", true},
		{"/app/.env", true},
		{"/app/.gitignore", true},
		{"/app/Dockerfile", true},
		{"/app/data_dir", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isLikelyFilePath(tt.path)
			if result != tt.expected {
				t.Errorf("isLikelyFilePath(%q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsReadableAndWritable(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	readable, err := isReadable(testFile)
	if err != nil || !readable {
		t.Errorf("expected testFile to be readable, got readable=%v err=%v", readable, err)
	}

	writable, err := isWritable(testFile)
	if err != nil || !writable {
		t.Errorf("expected testFile to be writable, got writable=%v err=%v", writable, err)
	}
}

func TestPromptConfirm(t *testing.T) {
	origStdin := os.Stdin
	origWriter := promptWriter
	defer func() {
		os.Stdin = origStdin
		promptWriter = origWriter
	}()
	promptWriter = io.Discard

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r

	go func() {
		_, _ = w.WriteString("y\n")
		_ = w.Close()
	}()

	if !promptConfirm("Test question?") {
		t.Error("expected promptConfirm to return true for 'y\\n'")
	}
}

func TestWindowsPathResolutionWithEnvVar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping non-Windows check on Windows")
	}
	tempDir := t.TempDir()
	comp := &config.ComposeConfig{
		Services: map[string]config.ComposeService{
			"app": {
				Volumes: []config.ComposeVolume{
					{
						Source: "${WIN_DIR}",
						Target: "/app",
						Type:   "bind",
					},
				},
			},
		},
	}
	engine := NewEngine(tempDir, filepath.Join(tempDir, "docker-compose.yml"), map[string]string{
		"WIN_DIR": `C:\data`,
	}, comp, nil)

	results := engine.checkVolumeAndPermissions(context.Background())
	found := false
	for _, res := range results {
		if strings.Contains(res.Name, "Incompatible OS Path") && res.Status == output.CheckFailed {
			found = true
			if !strings.Contains(res.Error, `C:\data`) {
				t.Errorf("expected error to mention resolved path C:\\data, got: %s", res.Error)
			}
		}
	}
	if !found {
		t.Error("expected Incompatible OS Path check failure for resolved Windows path on non-Windows OS")
	}
}

func TestEnvVarVolumeResolutionAndNamedVolumeBypass(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	content := `
services:
  app:
    volumes:
      - ${DATA_DIR}:/app/data
      - ${NAMED_DATA}:/var/lib/data
volumes:
  my_named_vol:
`
	if err := os.WriteFile(composePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatalf("failed to parse compose file: %v", err)
	}

	engine := NewEngine(tempDir, composePath, map[string]string{
		"DATA_DIR":   "./nonexistent_dir",
		"NAMED_DATA": "my_named_vol",
	}, comp, nil)

	results := engine.checkVolumeAndPermissions(context.Background())

	foundMissingBind := false
	foundNamedVolError := false
	for _, res := range results {
		if strings.Contains(res.Name, "Volume source missing") && strings.Contains(res.Error, "nonexistent_dir") {
			foundMissingBind = true
		}
		if strings.Contains(res.Error, "my_named_vol") {
			foundNamedVolError = true
		}
	}

	if !foundMissingBind {
		t.Error("expected missing volume source check for bind mount ${DATA_DIR}")
	}
	if foundNamedVolError {
		t.Error("expected top-level named volume ${NAMED_DATA} (my_named_vol) to be bypassed, but got an error")
	}
}
