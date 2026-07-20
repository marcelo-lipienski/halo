package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCLIQuietFlag(t *testing.T) {
	// 1. Build the binary first
	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "halo_test_bin")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary for CLI testing: %v", err)
	}

	// 2. Test quiet mode on system failure (missing configs)
	// We run check in an empty temp directory so it fails with system failure.
	cmdQuiet := exec.Command(binaryPath, "check", "--quiet", "--config-dir", tempDir)
	var stdoutQuiet, stderrQuiet bytes.Buffer
	cmdQuiet.Stdout = &stdoutQuiet
	cmdQuiet.Stderr = &stderrQuiet

	errQuiet := cmdQuiet.Run()
	// Should fail with exit code 1
	if errQuiet == nil {
		t.Error("expected command to fail due to missing configuration files")
	}

	if stdoutQuiet.Len() > 0 {
		t.Errorf("expected stdout to be completely empty in quiet mode, got: %q", stdoutQuiet.String())
	}

	if stderrQuiet.Len() == 0 {
		t.Error("expected stderr to contain error messages, but got nothing")
	} else if !strings.Contains(stderrQuiet.String(), "Missing configuration files") {
		t.Errorf("expected stderr to report missing configuration files, got: %q", stderrQuiet.String())
	}

	// 3. Test non-quiet mode to verify report goes to stdout
	cmdNormal := exec.Command(binaryPath, "check", "--config-dir", tempDir)
	var stdoutNormal, stderrNormal bytes.Buffer
	cmdNormal.Stdout = &stdoutNormal
	cmdNormal.Stderr = &stderrNormal

	_ = cmdNormal.Run()

	if stdoutNormal.Len() == 0 {
		t.Error("expected stdout to contain report output in normal mode, but it was empty")
	}
	if stderrNormal.Len() > 0 {
		t.Errorf("expected stderr to be empty in normal mode when failing via exitWithSystemFailure output, got: %q", stderrNormal.String())
	}
}

func TestGetWatchFiles(t *testing.T) {
	tempDir := t.TempDir()

	envPath := filepath.Join(tempDir, ".env")
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	examplePath := filepath.Join(tempDir, ".env.example")

	_ = os.WriteFile(envPath, []byte(""), 0644)
	_ = os.WriteFile(composePath, []byte(""), 0644)
	_ = os.WriteFile(examplePath, []byte(""), 0644)

	origConfigDir := configDir
	origEnvFile := envFile
	origComposeFiles := composeFiles
	defer func() {
		configDir = origConfigDir
		envFile = origEnvFile
		composeFiles = origComposeFiles
	}()

	configDir = tempDir
	envFile = ""
	composeFiles = nil

	files := getWatchFiles()

	expectedFiles := map[string]bool{
		envPath:     true,
		composePath: true,
		examplePath: true,
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 watch files, got %d: %v", len(files), files)
	}

	for _, f := range files {
		if !expectedFiles[f] {
			t.Errorf("unexpected file in watch list: %s", f)
		}
	}
}

func TestGetWatchFilesServiceEnvFiles(t *testing.T) {
	tempDir := t.TempDir()

	envPath := filepath.Join(tempDir, ".env")
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	svcEnvPath := filepath.Join(tempDir, "src", "myapp", ".env")

	_ = os.MkdirAll(filepath.Dir(svcEnvPath), 0755)
	_ = os.WriteFile(envPath, []byte(""), 0644)
	_ = os.WriteFile(svcEnvPath, []byte(""), 0644)

	composeContent := `
services:
  app:
    image: nginx
    env_file:
      - src/myapp/.env
`
	_ = os.WriteFile(composePath, []byte(composeContent), 0644)

	origConfigDir := configDir
	origEnvFile := envFile
	origComposeFiles := composeFiles
	defer func() {
		configDir = origConfigDir
		envFile = origEnvFile
		composeFiles = origComposeFiles
	}()

	configDir = tempDir
	envFile = ""
	composeFiles = nil

	files := getWatchFiles()

	expectedFiles := map[string]bool{
		envPath:     true,
		composePath: true,
		svcEnvPath:  true,
	}

	absExpected := make(map[string]bool)
	for f := range expectedFiles {
		abs, _ := filepath.Abs(f)
		absExpected[abs] = true
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 watch files, got %d: %v", len(files), files)
	}

	for _, f := range files {
		abs, _ := filepath.Abs(f)
		if !absExpected[abs] {
			t.Errorf("unexpected file in watch list: %s (abs: %s)", f, abs)
		}
	}
}
