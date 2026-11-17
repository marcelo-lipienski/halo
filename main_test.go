package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/marcelo-lipienski/halo/diagnostics"
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

func TestGetWatchFilesServiceEnvFilesWithEnvVar(t *testing.T) {
	tempDir := t.TempDir()

	envPath := filepath.Join(tempDir, ".env")
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	svcEnvPath := filepath.Join(tempDir, "src", "myapp", ".env")

	_ = os.MkdirAll(filepath.Dir(svcEnvPath), 0755)
	_ = os.WriteFile(envPath, []byte("APP_ENV_DIR=src/myapp\n"), 0644)
	_ = os.WriteFile(svcEnvPath, []byte(""), 0644)

	composeContent := `
services:
  app:
    image: nginx
    env_file:
      - ${APP_ENV_DIR}/.env
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

func TestCLIDoctorCustomComposeFile(t *testing.T) {
	tempDir := t.TempDir()
	customComposePath := filepath.Join(tempDir, "custom-compose.yml")

	composeContent := `
services:
  app:
    image: nginx:latest
    deploy:
      resources:
        limits:
          memory: 512MiB
`
	if err := os.WriteFile(customComposePath, []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	stdoutStr, _, exitCode := runInProcess([]string{"doctor", "--config-dir", tempDir, "--compose-file", customComposePath})
	if exitCode != 0 && exitCode != 2 {
		t.Errorf("unexpected exit code: %d", exitCode)
	}

	if !strings.Contains(stdoutStr, "512.0 MiB") && !strings.Contains(stdoutStr, "System Memory") {
		t.Errorf("expected doctor report to output system memory check with parsed compose limit, got: %q", stdoutStr)
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
	if stat, err := os.Stat(snapFile); err != nil {
		t.Fatalf("snapshot file was not created: %v", err)
	} else if runtime.GOOS != "windows" {
		if stat.Mode().Perm() != 0600 {
			t.Errorf("expected snapshot file permissions 0600, got %o", stat.Mode().Perm())
		}
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

func TestCLIFixCommand(t *testing.T) {
	origDockerUp := diagnostics.DockerComposeUpFunc
	defer func() { diagnostics.DockerComposeUpFunc = origDockerUp }()
	diagnostics.DockerComposeUpFunc = func(ctx context.Context, dir, composeFile, envFile, svcName string) error {
		return nil
	}

	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	composeContent := `
services:
  web:
    image: nginx
    volumes:
      - ./data_dir:/data
`
	_ = os.WriteFile(composePath, []byte(composeContent), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, ".env"), []byte(""), 0644)

	// Run halo fix
	stdoutStr, stderrStr, exitCode := runInProcess([]string{"fix", "--config-dir", tempDir})
	if exitCode != 0 {
		t.Errorf("expected exit code 0 on fix, got %d. stderr: %s, stdout: %s", exitCode, stderrStr, stdoutStr)
	}

	// Verify missing directory was auto-created
	if _, err := os.Stat(filepath.Join(tempDir, "data_dir")); err != nil {
		t.Errorf("expected data_dir to be created by halo fix, err: %v", err)
	}

	// Verify output contains auto-fixed indication
	if !strings.Contains(stdoutStr, "auto-fixed") {
		t.Errorf("expected stdout to mention auto-fixed, got: %q", stdoutStr)
	}
}

func TestCLIFixDryRun(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	composeContent := `
services:
  web:
    image: nginx
    volumes:
      - ./data_dir_dryrun:/data
`
	_ = os.WriteFile(composePath, []byte(composeContent), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, ".env"), []byte(""), 0644)

	// Run halo fix --dry-run / -d
	stdoutStr, stderrStr, exitCode := runInProcess([]string{"fix", "--config-dir", tempDir, "-d"})
	if exitCode != 2 {
		t.Errorf("expected exit code 2 on dry-run failure, got %d. stderr: %s, stdout: %s", exitCode, stderrStr, stdoutStr)
	}

	// Verify missing directory was NOT created in dry-run mode
	if _, err := os.Stat(filepath.Join(tempDir, "data_dir_dryrun")); err == nil {
		t.Error("expected data_dir_dryrun to NOT be created in dry-run mode")
	}

	if !strings.Contains(stdoutStr, "[Dry-Run]") {
		t.Errorf("expected stdout to contain [Dry-Run], got: %q", stdoutStr)
	}
}

func TestCLISnapshotAndDiffEdgeCases(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	envPath := filepath.Join(tempDir, ".env")
	_ = os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx:latest\n"), 0644)
	_ = os.WriteFile(envPath, []byte("PORT=8080\n"), 0644)

	// 1. Invalid format
	_, stderrStr, code := runInProcess([]string{"snapshot", "--config-dir", tempDir, "--format", "invalid"})
	if code != 1 || !strings.Contains(stderrStr, "Invalid format") {
		t.Errorf("expected exit code 1 for invalid format, got %d, stderr: %s", code, stderrStr)
	}

	// 2. Snapshot dry-run json
	stdoutStr, _, code := runInProcess([]string{"snapshot", "--config-dir", tempDir, "--format", "json", "-d"})
	if code != 0 || !strings.Contains(stdoutStr, `"dry_run":true`) {
		t.Errorf("expected dry_run json success, got code %d, stdout: %s", code, stdoutStr)
	}

	// 3. Snapshot execution json
	snapPath := filepath.Join(tempDir, "snap.json")
	stdoutStr, _, code = runInProcess([]string{"snapshot", snapPath, "--config-dir", tempDir, "--format", "json"})
	if code != 0 || !strings.Contains(stdoutStr, `"status":"success"`) {
		t.Errorf("expected snapshot json success, got code %d, stdout: %s", code, stdoutStr)
	}

	// 4. Diff execution json against generated snapPath
	stdoutStr, _, code = runInProcess([]string{"diff", snapPath, "--config-dir", tempDir, "--format", "json"})
	if code != 0 {
		t.Errorf("expected diff json success, got code %d, stdout: %s", code, stdoutStr)
	}

	// 5. Diff missing snapshot
	stdoutStr, stderrStr, code = runInProcess([]string{"diff", filepath.Join(tempDir, "missing.json"), "--config-dir", tempDir})
	outCombined := stdoutStr + stderrStr
	if code != 1 || !strings.Contains(outCombined, "Snapshot file not found") {
		t.Errorf("expected missing snapshot error, got code %d, combined output: %s", code, outCombined)
	}
}

func TestCLIInitComprehensive(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Invalid format
	_, stderrStr, code := runInProcess([]string{"init", "--config-dir", tempDir, "--format", "yaml"})
	if code != 1 || !strings.Contains(stderrStr, "Invalid format") {
		t.Errorf("expected exit code 1 on invalid format, got %d, stderr: %s", code, stderrStr)
	}

	// 2. Missing .env.example (text)
	_, stderrStr, code = runInProcess([]string{"init", "--config-dir", tempDir})
	if code != 1 || !strings.Contains(stderrStr, "failed to read example file") {
		t.Errorf("expected exit code 1 on missing example, got %d, stderr: %s", code, stderrStr)
	}

	// 3. Missing .env.example (json)
	stdoutStr, _, code := runInProcess([]string{"init", "--config-dir", tempDir, "--format", "json"})
	if code != 1 || !strings.Contains(stdoutStr, `"system_failure"`) {
		t.Errorf("expected system failure json on missing example, got code %d, stdout: %s", code, stdoutStr)
	}

	// 4. Missing .env.example (quiet)
	stdoutStr, stderrStr, code = runInProcess([]string{"init", "--config-dir", tempDir, "--quiet"})
	if code != 1 || stdoutStr != "" || stderrStr != "" {
		t.Errorf("expected quiet failure with empty output, got code %d, stdout: %q, stderr: %q", code, stdoutStr, stderrStr)
	}

	// Create valid .env.example with regular key and placeholder
	exampleContent := "PORT=8080\nAPI_KEY=your_api_key_here\n"
	if err := os.WriteFile(filepath.Join(tempDir, ".env.example"), []byte(exampleContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 5. Dry-run text
	stdoutStr, _, code = runInProcess([]string{"init", "--config-dir", tempDir, "-d"})
	if code != 0 || !strings.Contains(stdoutStr, "[dry-run]") {
		t.Errorf("expected dry-run success, got code %d, stdout: %s", code, stdoutStr)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".env")); err == nil {
		t.Error("expected .env NOT to exist after dry-run")
	}

	// 6. Dry-run json
	stdoutStr, _, code = runInProcess([]string{"init", "--config-dir", tempDir, "--dry-run", "--format", "json"})
	if code != 0 || !strings.Contains(stdoutStr, `"dry_run":true`) || !strings.Contains(stdoutStr, `"placeholders":1`) {
		t.Errorf("expected dry-run json output, got code %d, stdout: %s", code, stdoutStr)
	}

	// 7. Quiet run (should create .env with 0 stdout/stderr)
	stdoutStr, stderrStr, code = runInProcess([]string{"init", "--config-dir", tempDir, "--quiet"})
	if code != 0 || stdoutStr != "" || stderrStr != "" {
		t.Errorf("expected quiet creation with 0 code and empty output, got code %d, stdout: %q, stderr: %q", code, stdoutStr, stderrStr)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".env")); err != nil {
		t.Errorf("expected .env to be created by init, err: %v", err)
	}

	// Remove .env to test JSON creation
	_ = os.Remove(filepath.Join(tempDir, ".env"))

	// 8. JSON creation
	stdoutStr, _, code = runInProcess([]string{"init", "--config-dir", tempDir, "--format", "json"})
	if code != 0 || !strings.Contains(stdoutStr, `"status":"success"`) || !strings.Contains(stdoutStr, `"created":true`) {
		t.Errorf("expected json created success, got code %d, stdout: %s", code, stdoutStr)
	}

	// 9. Text rerun with already-present keys and new placeholder in example
	if err := os.WriteFile(filepath.Join(tempDir, ".env.example"), []byte("PORT=8080\nAPI_KEY=your_key\nNEW_KEY=change-me\n"), 0644); err != nil {
		t.Fatal(err)
	}
	stdoutStr, _, code = runInProcess([]string{"init", "--config-dir", tempDir})
	if code != 0 || !strings.Contains(stdoutStr, "merging missing keys") || !strings.Contains(stdoutStr, "needs value") {
		t.Errorf("expected text update output with needs value indication, got code %d, stdout: %s", code, stdoutStr)
	}

	// 10. Dry-run quiet (no output)
	stdoutStr, stderrStr, code = runInProcess([]string{"init", "--config-dir", tempDir, "-d", "-q"})
	if code != 0 || stdoutStr != "" || stderrStr != "" {
		t.Errorf("expected dry-run quiet to produce empty output, got code %d, stdout: %q, stderr: %q", code, stdoutStr, stderrStr)
	}
}

func TestCLISnapshotAndDiffEdgeCasesExtended(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Snapshot with missing configuration files
	stdoutStr, stderrStr, code := runInProcess([]string{"snapshot", "--config-dir", tempDir})
	combined := stdoutStr + stderrStr
	if code != 1 || !strings.Contains(combined, "Missing configuration files") {
		t.Errorf("expected missing configuration files failure, got code %d, combined: %s", code, combined)
	}

	// Setup valid files
	_ = os.WriteFile(filepath.Join(tempDir, ".env"), []byte("PORT=3000\n"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "docker-compose.yml"), []byte("services:\n  app:\n    image: nginx:latest\n"), 0644)

	// 2. Snapshot --quiet
	snapFile := filepath.Join(tempDir, "snap-quiet.json")
	stdoutStr, stderrStr, code = runInProcess([]string{"snapshot", snapFile, "--config-dir", tempDir, "--quiet"})
	if code != 0 || stdoutStr != "" || stderrStr != "" {
		t.Errorf("expected quiet snapshot with empty output, got code %d, stdout: %q, stderr: %q", code, stdoutStr, stderrStr)
	}

	// 3. Diff --quiet
	stdoutStr, stderrStr, code = runInProcess([]string{"diff", snapFile, "--config-dir", tempDir, "--quiet"})
	if code != 0 || stdoutStr != "" || stderrStr != "" {
		t.Errorf("expected quiet diff with empty output, got code %d, stdout: %q, stderr: %q", code, stdoutStr, stderrStr)
	}

	// 4. Diff invalid JSON parse
	corruptSnap := filepath.Join(tempDir, "corrupt.json")
	_ = os.WriteFile(corruptSnap, []byte("not valid json"), 0644)
	stdoutStr, stderrStr, code = runInProcess([]string{"diff", corruptSnap, "--config-dir", tempDir})
	combined = stdoutStr + stderrStr
	if code != 1 || !strings.Contains(combined, "Failed to parse snapshot file") {
		t.Errorf("expected parse failure on corrupt snapshot, got code %d, combined: %s", code, combined)
	}

	// 5. Diff with project mismatch warning
	mismatchedSnap := filepath.Join(tempDir, "mismatched.json")
	snapData, _ := os.ReadFile(snapFile)
	var snapObj map[string]interface{}
	_ = json.Unmarshal(snapData, &snapObj)
	snapObj["project"] = "different-project-name"
	modifiedData, _ := json.Marshal(snapObj)
	_ = os.WriteFile(mismatchedSnap, modifiedData, 0644)

	_, stderrStr, code = runInProcess([]string{"diff", mismatchedSnap, "--config-dir", tempDir})
	if code != 0 || !strings.Contains(stderrStr, "Comparing snapshot from project 'different-project-name'") {
		t.Errorf("expected project mismatch warning on stderr, got code %d, stderr: %s", code, stderrStr)
	}

	// 6. Snapshot dry-run text and quiet
	stdoutStr, _, code = runInProcess([]string{"snapshot", "--config-dir", tempDir, "-d"})
	if code != 0 || !strings.Contains(stdoutStr, "[dry-run]") {
		t.Errorf("expected text snapshot dry-run output, got code %d, stdout: %s", code, stdoutStr)
	}
	stdoutStr, stderrStr, code = runInProcess([]string{"snapshot", "--config-dir", tempDir, "-d", "-q"})
	if code != 0 || stdoutStr != "" || stderrStr != "" {
		t.Errorf("expected quiet snapshot dry-run output, got code %d, stdout: %q, stderr: %q", code, stdoutStr, stderrStr)
	}

	// 7. Diff invalid format
	_, stderrStr, code = runInProcess([]string{"diff", snapFile, "--config-dir", tempDir, "--format", "xml"})
	if code != 1 || !strings.Contains(stderrStr, "Invalid format") {
		t.Errorf("expected exit code 1 for invalid diff format, got code %d, stderr: %s", code, stderrStr)
	}

	// 8. Diff with changes formatted as JSON
	_ = os.WriteFile(filepath.Join(tempDir, ".env"), []byte("PORT=3000\nDIFF_KEY=added\n"), 0644)
	stdoutStr, _, code = runInProcess([]string{"diff", snapFile, "--config-dir", tempDir, "--format", "json"})
	if code != 2 || !strings.Contains(stdoutStr, `"variables"`) || !strings.Contains(stdoutStr, "DIFF_KEY") {
		t.Errorf("expected exit code 2 and json diff with DIFF_KEY, got code %d, stdout: %s", code, stdoutStr)
	}

	// 9. Version command
	stdoutStr, _, code = runInProcess([]string{"version"})
	if code != 0 || !strings.Contains(stdoutStr, "halo version") {
		t.Errorf("expected version output, got code %d, stdout: %s", code, stdoutStr)
	}
}
