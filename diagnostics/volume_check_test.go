package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	defer func() { os.Stdin = origStdin }()

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
