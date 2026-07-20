package diagnostics

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/marcelo-lipienski/halo/output"
)

func TestCheckEnvExampleDrift(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	examplePath := filepath.Join(tmpDir, ".env.example")

	// 1. No .env.example
	res, err := CheckEnvExampleDrift(envPath, examplePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Errorf("expected 0 checks, got %d", len(res))
	}

	// 2. All keys present -> both checks pass
	os.WriteFile(examplePath, []byte("A=1\nB=2\n"), 0644)
	os.WriteFile(envPath, []byte("A=1\nB=2\n"), 0644)
	res, err = CheckEnvExampleDrift(envPath, examplePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].Status != output.CheckPassed || res[1].Status != output.CheckPassed {
		t.Errorf("expected both passing checks, got: %+v", res)
	}

	// 3. Missing keys -> failure check
	os.WriteFile(envPath, []byte("A=1\n"), 0644)
	res, err = CheckEnvExampleDrift(envPath, examplePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(res))
	}
	if res[0].Status != output.CheckFailed || res[0].Name != ".env.example Drift" {
		t.Errorf("expected failure for missing keys, got: %+v", res[0])
	}

	// 4. Extra keys in .env -> warning check
	os.WriteFile(examplePath, []byte("A=1\n"), 0644)
	os.WriteFile(envPath, []byte("A=1\nB=2\n"), 0644)
	res, err = CheckEnvExampleDrift(envPath, examplePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(res))
	}
	if res[1].Status != output.CheckWarning || res[1].Name != "Undeclared Keys" {
		t.Errorf("expected warning for extra keys, got: %+v", res[1])
	}

	// 5. Both missing and extra keys
	os.WriteFile(examplePath, []byte("A=1\nC=3\n"), 0644)
	os.WriteFile(envPath, []byte("A=1\nB=2\n"), 0644)
	res, err = CheckEnvExampleDrift(envPath, examplePath)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Status != output.CheckFailed {
		t.Errorf("expected failure for missing keys, got: %+v", res[0])
	}
	if res[1].Status != output.CheckWarning {
		t.Errorf("expected warning for extra keys, got: %+v", res[1])
	}
}
