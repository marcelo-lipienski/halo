package diagnostics

import (
	"context"
	"os"
	"path/filepath"
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
