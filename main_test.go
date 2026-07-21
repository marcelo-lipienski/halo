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

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func (s *safeBuffer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.Reset()
}

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

	stdout.mu.RLock()
	origStdout := stdout.w
	stdout.mu.RUnlock()

	stderr.mu.RLock()
	origStderr := stderr.w
	stderr.mu.RUnlock()

	osExit.mu.RLock()
	origOsExit := osExit.fn
	osExit.mu.RUnlock()

	defer func() {
		stdout.Set(origStdout)
		stderr.Set(origStderr)
		osExit.Set(origOsExit)
	}()

	outBuf := &safeBuffer{}
	errBuf := &safeBuffer{}
	stdout.Set(outBuf)
	stderr.Set(errBuf)

	var exitCode int
	osExit.Set(func(code int) {
		exitCode = code
	})

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
	_ = os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(".env\n"), 0644)

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
	defer cancel()

	// Set up capturing output buffers
	outBuf := &safeBuffer{}
	errBuf := &safeBuffer{}

	// Acquire testMu for the full lifetime of the background goroutine so that
	// no other test can modify the shared CLI globals while runWatch is running.
	testMu.Lock()
	stdout.mu.RLock()
	origStdout := stdout.w
	stdout.mu.RUnlock()
	stderr.mu.RLock()
	origStderr := stderr.w
	stderr.mu.RUnlock()

	stdout.Set(outBuf)
	stderr.Set(errBuf)
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

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runWatch(ctx)
	}()

	// Wait for watch mode to execute initial run and print watch prompt.
	time.Sleep(200 * time.Millisecond)

	prompt := errBuf.String()
	errBuf.Reset()

	// Trigger a file change
	_ = os.WriteFile(envPath, []byte("APP_NAME=test"), 0644)

	// Wait for reload ticker to fire and print the change detection log.
	time.Sleep(300 * time.Millisecond)

	reloaded := errBuf.String()

	// Stop watcher and wait until the goroutine has fully exited before
	// restoring globals — this eliminates the data race with the next test.
	cancel()
	wg.Wait()

	stdout.Set(origStdout)
	stderr.Set(origStderr)
	testMu.Unlock()

	if !strings.Contains(prompt, "Watching for configuration changes") {
		t.Errorf("expected watcher prompt, got: %q", prompt)
	}
	if !strings.Contains(reloaded, "Change detected") {
		t.Errorf("expected reload trigger log, got: %q", reloaded)
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
	_ = os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(".env\n"), 0644)

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
	_ = os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(".env\n"), 0644)

	stdoutStr, _, exitCode := runInProcess([]string{"check", "--compose-file", composePath1, "--compose-file", composePath2, "--config-dir", tempDir})
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	// When Docker is not available, service reachability checks are skipped and
	// replaced with a Docker Daemon Status warning. Verify the suite still ran
	// all phases by checking for the environment alignment and volume groups.
	if !strings.Contains(stdoutStr, "Environmental Alignment") || !strings.Contains(stdoutStr, "Volume") {
		t.Errorf("expected full diagnostic report, got: %q", stdoutStr)
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
	_ = os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(".env\n"), 0644)

	stdoutStr, _, exitCode := runInProcess([]string{"--config-dir", tempDir})
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	// Verify the root command ran the full diagnostic suite (same as `halo check`).
	if !strings.Contains(stdoutStr, "halo Diagnostics Report") {
		t.Errorf("expected diagnostics report in output, got: %q", stdoutStr)
	}
}

func TestCLIInit(t *testing.T) {
	tmpDir := t.TempDir()
	examplePath := filepath.Join(tmpDir, ".env.example")

	exampleContent := "KEY1=value1\nKEY2=<required>\n"
	if err := os.WriteFile(examplePath, []byte(exampleContent), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runInProcess([]string{"init", "--config-dir", tmpDir})

	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if stderr != "" {
		t.Errorf("expected no stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "Added 2 keys") {
		t.Errorf("expected output to contain 'Added 2 keys', got %q", stdout)
	}

	// run again
	stdout, _, code = runInProcess([]string{"init", "--config-dir", tmpDir})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout, "up to date") {
		t.Errorf("expected output to contain 'up to date', got %q", stdout)
	}
}

func TestCLIEnvDrift(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	examplePath := filepath.Join(tmpDir, ".env.example")
	composePath := filepath.Join(tmpDir, "docker-compose.yml")

	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(examplePath, []byte("DB_URL=<required>\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte("EXTRA=1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(".env\n"), 0644); err != nil {
		t.Fatal(err)
	}

	stdoutStr, _, _ := runInProcess([]string{"check", "--config-dir", tmpDir})

	if !strings.Contains(stdoutStr, ".env.example Drift") {
		t.Errorf("expected drift check in output, got %q", stdoutStr)
	}
	if !strings.Contains(stdoutStr, "missing from .env") {
		t.Errorf("expected missing keys error in output")
	}
	if !strings.Contains(stdoutStr, "Undeclared Keys") {
		t.Errorf("expected undeclared keys in output")
	}
}

func TestCLIDoctor(t *testing.T) {
	tmpDir := t.TempDir()

	stdoutStr, _, code := runInProcess([]string{"doctor", "--config-dir", tmpDir})
	if code != 0 && code != 2 {
		t.Errorf("expected exit code 0 or 2, got %d", code)
	}

	if !strings.Contains(stdoutStr, "System Prerequisites") {
		t.Errorf("expected doctor report group 'System Prerequisites', got %q", stdoutStr)
	}
	if !strings.Contains(stdoutStr, "Host Resources") {
		t.Errorf("expected doctor report group 'Host Resources', got %q", stdoutStr)
	}
}

func TestCLIImageSecurity(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	composePath := filepath.Join(tmpDir, "docker-compose.yml")

	if err := os.WriteFile(envPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(".env\n"), 0644); err != nil {
		t.Fatal(err)
	}
	composeContent := `
services:
  web:
    image: nginx:latest
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	stdoutStr, _, _ := runInProcess([]string{"check", "--config-dir", tmpDir, "--verbose"})

	if !strings.Contains(stdoutStr, "Image Security: web") {
		t.Errorf("expected Image Security in output, got: %q", stdoutStr)
	}
	if !strings.Contains(stdoutStr, "mutable tag 'latest'") {
		t.Errorf("expected mutable tag warning in output, got: %q", stdoutStr)
	}
}

func TestCLIDockerfileImageSecurity(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	composePath := filepath.Join(tmpDir, "docker-compose.yml")
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")

	if err := os.WriteFile(envPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(".env\n"), 0644); err != nil {
		t.Fatal(err)
	}
	composeContent := `
services:
  web:
    build:
      context: .
      dockerfile: Dockerfile
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	dockerfileContent := `
FROM alpine:latest
`
	if err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644); err != nil {
		t.Fatal(err)
	}

	stdoutStr, _, _ := runInProcess([]string{"check", "--config-dir", tmpDir, "--verbose"})

	if !strings.Contains(stdoutStr, "Image Security (Dockerfile): web") {
		t.Errorf("expected Image Security (Dockerfile) in output, got: %q", stdoutStr)
	}
	if !strings.Contains(stdoutStr, "base image 'alpine:latest'") {
		t.Errorf("expected mutable base image warning in output, got: %q", stdoutStr)
	}
}

func TestCLISnapshotAndDiff(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	composePath := filepath.Join(tmpDir, "docker-compose.yml")

	if err := os.WriteFile(envPath, []byte("DB_HOST=localhost\n"), 0644); err != nil {
		t.Fatal(err)
	}
	composeContent := `
services:
  web:
    image: nginx:alpine
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	snapFile := filepath.Join(tmpDir, "custom-snapshot.json")

	// 1. Run snapshot
	stdoutStr, stderrStr, exitCode := runInProcess([]string{"snapshot", snapFile, "--config-dir", tmpDir})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0 on snapshot, got %d. stderr: %s", exitCode, stderrStr)
	}
	if !strings.Contains(stdoutStr, "Captured state snapshot") {
		t.Errorf("expected snapshot confirmation, got: %q", stdoutStr)
	}

	// Verify file exists
	if _, err := os.Stat(snapFile); err != nil {
		t.Fatalf("snapshot file was not created: %v", err)
	}

	// 2. Run diff (no changes)
	stdoutStr, stderrStr, exitCode = runInProcess([]string{"diff", snapFile, "--config-dir", tmpDir})
	if exitCode != 0 {
		t.Errorf("expected exit code 0 on no-change diff, got %d. stderr: %s", exitCode, stderrStr)
	}
	if !strings.Contains(stdoutStr, "Environment matches snapshot exactly") {
		t.Errorf("expected matching environment message, got: %q", stdoutStr)
	}

	// 3. Modify env file
	if err := os.WriteFile(envPath, []byte("DB_HOST=localhost\nNEW_VAR=hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Run diff (has changes)
	stdoutStr, stderrStr, exitCode = runInProcess([]string{"diff", snapFile, "--config-dir", tmpDir})
	if exitCode != 2 {
		t.Errorf("expected exit code 2 on modified diff, got %d. stderr: %s, stdout: %s", exitCode, stderrStr, stdoutStr)
	}
	if !strings.Contains(stdoutStr, "NEW_VAR: added") {
		t.Errorf("expected added variable in diff output, got: %q", stdoutStr)
	}
}
