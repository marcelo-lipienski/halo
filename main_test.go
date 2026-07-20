package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var testMu sync.Mutex

func runInProcess(args []string) (string, string, int) {
	testMu.Lock()
	defer testMu.Unlock()

	// Reset globals
	configDir = "."
	envFile = ""
	composeFiles = []string{}
	format = "text"
	verbose = false
	fix = false
	quiet = false
	dryRun = false
	interactive = false
	watch = false

	origStdout := stdout
	origStderr := stderr
	origOsExit := osExit
	defer func() {
		stdout = origStdout
		stderr = origStderr
		osExit = origOsExit
	}()

	var outBuf, errBuf bytes.Buffer
	stdout = &outBuf
	stderr = &errBuf

	var exitCode int
	osExit = func(code int) {
		exitCode = code
	}

	rootCmd := newRootCmd()
	rootCmd.SetArgs(args)
	_ = rootCmd.Execute()

	return outBuf.String(), errBuf.String(), exitCode
}

func TestCLIQuietFlag(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Run in-process with --quiet
	stdoutStr, stderrStr, exitCode := runInProcess([]string{"check", "--quiet", "--config-dir", tempDir})

	// Since tempDir is empty, it should fail (exitCode 1) due to missing configs
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	if len(stdoutStr) > 0 {
		t.Errorf("expected stdout to be empty in quiet mode, got: %q", stdoutStr)
	}

	if len(stderrStr) == 0 {
		t.Error("expected stderr to contain error messages, but got nothing")
	} else if !strings.Contains(stderrStr, "Missing configuration files") {
		t.Errorf("expected stderr to report missing configuration files, got: %q", stderrStr)
	}

	// 2. Run in-process in normal mode
	stdoutStr, stderrStr, exitCode = runInProcess([]string{"check", "--config-dir", tempDir})

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	if len(stdoutStr) == 0 {
		t.Error("expected stdout to contain report output in normal mode, but it was empty")
	}
	if len(stderrStr) > 0 {
		t.Errorf("expected stderr to be empty in normal mode, got: %q", stderrStr)
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

func TestCLIVersion(t *testing.T) {
	stdoutStr, stderrStr, exitCode := runInProcess([]string{"version"})
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d. stderr: %s", exitCode, stderrStr)
	}
	if !strings.Contains(stdoutStr, "halo version") {
		t.Errorf("expected version output, got: %q", stdoutStr)
	}
}

func TestCLIInvalidFormat(t *testing.T) {
	_, stderrStr, exitCode := runInProcess([]string{"check", "--format", "invalid"})
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderrStr, "Invalid format") {
		t.Errorf("expected invalid format error, got: %q", stderrStr)
	}
}

func TestCLIBrokenCompose(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	_ = os.WriteFile(composePath, []byte("invalid:::yaml"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, ".env"), []byte(""), 0644)

	stdoutStr, _, exitCode := runInProcess([]string{"check", "--config-dir", tempDir})
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stdoutStr, "System Discovery") || !strings.Contains(stdoutStr, "failed to parse docker-compose file") {
		t.Errorf("expected parse failure report in stdout, got: %q", stdoutStr)
	}
}

func TestCLISuccessCheck(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	composeContent := `
services:
  web:
    image: nginx
`
	_ = os.WriteFile(composePath, []byte(composeContent), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, ".env"), []byte(""), 0644)

	stdoutStr, stderrStr, exitCode := runInProcess([]string{"check", "--config-dir", tempDir})
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d. stderr: %s, stdout: %s", exitCode, stderrStr, stdoutStr)
	}
	if !strings.Contains(stdoutStr, "Docker Daemon Status") && !strings.Contains(stdoutStr, "Service web reachability") {
		t.Errorf("expected Docker Daemon Status or Service web reachability warning in output, got: %q", stdoutStr)
	}
}

func TestCLIWatchMode(t *testing.T) {
	tempDir := t.TempDir()

	envPath := filepath.Join(tempDir, ".env")
	composePath := filepath.Join(tempDir, "docker-compose.yml")

	_ = os.WriteFile(envPath, []byte(""), 0644)
	_ = os.WriteFile(composePath, []byte("services:\n  app:\n    image: nginx"), 0644)

	// Set up cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Set up capturing output buffers
	var outBuf, errBuf bytes.Buffer
	testMu.Lock()
	origStdout := stdout
	origStderr := stderr
	stdout = &outBuf
	stderr = &errBuf
	// Reset CLI globals
	configDir = tempDir
	envFile = ""
	composeFiles = []string{}
	format = "text"
	verbose = false
	fix = false
	quiet = false
	dryRun = false
	interactive = false
	watch = true
	testMu.Unlock()

	defer func() {
		testMu.Lock()
		stdout = origStdout
		stderr = origStderr
		testMu.Unlock()
	}()

	// Start runWatch in a separate goroutine
	go func() {
		runWatch(ctx)
	}()

	// Wait for watch mode to execute initial run and print watch prompt
	time.Sleep(200 * time.Millisecond)

	// Verify initial run completed and printed the watcher start text
	if !strings.Contains(outBuf.String(), "Watching for configuration changes") {
		t.Errorf("expected watcher prompt, got: %q", outBuf.String())
	}

	// Trigger a file change
	outBuf.Reset()
	_ = os.WriteFile(envPath, []byte("APP_NAME=test"), 0644)

	// Wait for reload ticker to fire and print the change detection log
	time.Sleep(300 * time.Millisecond)

	// Cancel the context to stop the watcher goroutine
	cancel()

	// Verify reload was triggered
	if !strings.Contains(outBuf.String(), "Change detected") {
		t.Errorf("expected reload trigger log, got: %q", outBuf.String())
	}
}

func TestCLISuccessCheckQuiet(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	composeContent := `
services:
  web:
    image: nginx
`
	_ = os.WriteFile(composePath, []byte(composeContent), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, ".env"), []byte(""), 0644)

	stdoutStr, stderrStr, exitCode := runInProcess([]string{"check", "--config-dir", tempDir, "--quiet"})
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d. stderr: %s, stdout: %s", exitCode, stderrStr, stdoutStr)
	}
	if len(stdoutStr) > 0 {
		t.Errorf("expected stdout to be empty in quiet mode, got: %q", stdoutStr)
	}
}

func TestCLIMultipleCompose(t *testing.T) {
	tempDir := t.TempDir()
	composePath1 := filepath.Join(tempDir, "docker-compose1.yml")
	composePath2 := filepath.Join(tempDir, "docker-compose2.yml")
	_ = os.WriteFile(composePath1, []byte("services:\n  web1:\n    image: nginx"), 0644)
	_ = os.WriteFile(composePath2, []byte("services:\n  web2:\n    image: redis"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, ".env"), []byte(""), 0644)

	stdoutStr, _, exitCode := runInProcess([]string{"check", "--compose-file", composePath1, "--compose-file", composePath2, "--config-dir", tempDir})
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdoutStr, "web1") || !strings.Contains(stdoutStr, "web2") {
		t.Errorf("expected both web1 and web2 checks to be in output, got: %q", stdoutStr)
	}
}

func TestCLIRootCheck(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	composeContent := `
services:
  web:
    image: nginx
`
	_ = os.WriteFile(composePath, []byte(composeContent), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, ".env"), []byte(""), 0644)

	stdoutStr, _, exitCode := runInProcess([]string{"--config-dir", tempDir})
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdoutStr, "web") {
		t.Errorf("expected web checks to be in output, got: %q", stdoutStr)
	}
}
