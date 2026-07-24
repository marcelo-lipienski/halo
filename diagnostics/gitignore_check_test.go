package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/output"
)

func TestMatchGitignorePattern(t *testing.T) {
	tests := []struct {
		pattern string
		relPath string
		want    bool
	}{
		{"/.env", ".env", true},
		{"/.env", "sub/.env", false},
		{".env", ".env", true},
		{".env", "sub/.env", true},
		{"*.env", "my.env", true},
		{"*.env", "sub/my.env", true},
		{"config/*.env", "config/my.env", true},
		{"config/*.env", "sub/config/my.env", false},
		{"!/.env", ".env", true}, // negation pattern matches the path
	}

	for _, tt := range tests {
		got, err := matchGitignorePattern(tt.pattern, tt.relPath)
		if err != nil {
			t.Errorf("matchGitignorePattern(%q, %q) returned error: %v", tt.pattern, tt.relPath, err)
			continue
		}
		if got != tt.want {
			t.Errorf("matchGitignorePattern(%q, %q) = %v; want %v", tt.pattern, tt.relPath, got, tt.want)
		}
	}
}

func TestCheckGitignoreSecurity(t *testing.T) {
	tmpDir := t.TempDir()

	envPath := filepath.Join(tmpDir, ".env")
	envLocalPath := filepath.Join(tmpDir, ".env.local")

	if err := os.WriteFile(envPath, []byte("A=B"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envLocalPath, []byte("C=D"), 0644); err != nil {
		t.Fatal(err)
	}

	// Mock Engine
	e := &Engine{
		ConfigDir: tmpDir,
		Compose:   &config.ComposeConfig{},
		Env:       map[string]string{},
	}

	ctx := context.Background()

	// 1. Run checks (should fail since no .gitignore exists)
	results := e.CheckGitignoreSecurity(ctx)

	failedEnv := false
	failedLocal := false
	for _, r := range results {
		if r.Status == output.CheckFailed {
			if r.Name == "Unignored Env File: .env" {
				failedEnv = true
			}
			if r.Name == "Unignored Env File: .env.local" {
				failedLocal = true
			}
		}
	}

	if !failedEnv || !failedLocal {
		t.Errorf("expected both files to fail ignore check, got: %+v", results)
	}

	// 2. Add .gitignore
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(".env\n.env.local\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run checks again (should pass!)
	results = e.CheckGitignoreSecurity(ctx)
	passedEnv := false
	passedLocal := false
	for _, r := range results {
		if r.Status == output.CheckPassed {
			if r.Name == "Ignored Env File: .env" {
				passedEnv = true
			}
			if r.Name == "Ignored Env File: .env.local" {
				passedLocal = true
			}
		}
	}

	if !passedEnv || !passedLocal {
		t.Errorf("expected both files to pass ignore check, got: %+v", results)
	}

	// 3. Negate .env in .gitignore
	if err := os.WriteFile(gitignorePath, []byte(".env\n.env.local\n!.env\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run checks again (should fail for .env, pass for .env.local)
	results = e.CheckGitignoreSecurity(ctx)
	failedEnvAfterNegation := false
	passedLocalAfterNegation := false
	for _, r := range results {
		if r.Name == "Unignored Env File: .env" && r.Status == output.CheckFailed {
			failedEnvAfterNegation = true
		}
		if r.Name == "Ignored Env File: .env.local" && r.Status == output.CheckPassed {
			passedLocalAfterNegation = true
		}
	}

	if !failedEnvAfterNegation || !passedLocalAfterNegation {
		t.Errorf("expected negated .env to fail and .env.local to pass, got: %+v", results)
	}
}

func TestFindEnvFilesNamingPatterns(t *testing.T) {
	tmpDir := t.TempDir()

	_ = os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(""), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, ".env-local"), []byte(""), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, ".env_prod"), []byte(""), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, ".env.example"), []byte(""), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, ".env-sample"), []byte(""), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, ".env_template"), []byte(""), 0644)

	files, err := findEnvFiles(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("unexpected error finding env files: %v", err)
	}

	foundMap := make(map[string]bool)
	for _, f := range files {
		foundMap[filepath.Base(f)] = true
	}

	if !foundMap[".env"] || !foundMap[".env-local"] || !foundMap[".env_prod"] {
		t.Errorf("expected secret env files to be found, got: %v", files)
	}

	if foundMap[".env.example"] || foundMap[".env-sample"] || foundMap[".env_template"] {
		t.Errorf("expected example/sample/template files to be ignored, got: %v", files)
	}
}

func TestCheckGitignoreSecurityContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("SECRET=1"), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	e := &Engine{
		ConfigDir: tmpDir,
		Compose:   &config.ComposeConfig{},
		Env:       map[string]string{},
	}

	results := e.CheckGitignoreSecurity(ctx)
	if len(results) == 0 {
		t.Fatal("expected at least 1 result for cancelled context")
	}

	cancelled := false
	for _, r := range results {
		if r.Status == output.CheckFailed && r.Name == "Check Timeout" {
			cancelled = true
			break
		}
	}

	if !cancelled {
		t.Errorf("expected cancellation error result, got: %+v", results)
	}
}

func TestCheckGitignoreSecurityAutoFixAndDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envPath, []byte("SECRET=1"), 0644); err != nil {
		t.Fatal(err)
	}

	// DryRun test
	eDry := &Engine{
		ConfigDir: tmpDir,
		Compose:   &config.ComposeConfig{},
		Env:       map[string]string{},
		DryRun:    true,
	}
	resultsDry := eDry.CheckGitignoreSecurity(context.Background())
	if len(resultsDry) == 0 || resultsDry[0].Status != output.CheckFailed || !strings.Contains(resultsDry[0].Error, "[Dry-Run]") {
		t.Errorf("expected Dry-Run failure result, got: %+v", resultsDry)
	}

	// AutoFix test
	eFix := &Engine{
		ConfigDir: tmpDir,
		Compose:   &config.ComposeConfig{},
		Env:       map[string]string{},
		AutoFix:   true,
	}
	resultsFix := eFix.CheckGitignoreSecurity(context.Background())
	if len(resultsFix) == 0 || resultsFix[0].Status != output.CheckPassed {
		t.Errorf("expected passing auto-fixed result, got: %+v", resultsFix)
	}

	gitignoreContent, _ := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if !strings.Contains(string(gitignoreContent), ".env") {
		t.Errorf("expected .gitignore to contain .env, got: %s", string(gitignoreContent))
	}
}

func TestQuoteMetaByte(t *testing.T) {
	chars := []byte{'+', '?', '.', '*', '^', '$', '(', ')', '[', ']', '{', '}', '|', '\\', 'a', '1'}
	var buf strings.Builder
	for _, c := range chars {
		quoteMetaByte(&buf, c)
	}
	got := buf.String()
	if !strings.Contains(got, "\\+") || !strings.Contains(got, "\\?") || !strings.Contains(got, "\\*") {
		t.Errorf("quoteMetaByte failed to escape special characters, got %q", got)
	}
}
