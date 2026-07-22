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

	writeFile := func(path string, data []byte) {
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	// 2. All keys present -> both checks pass
	writeFile(examplePath, []byte("A=1\nB=2\n"))
	writeFile(envPath, []byte("A=1\nB=2\n"))
	res, err = CheckEnvExampleDrift(envPath, examplePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].Status != output.CheckPassed || res[1].Status != output.CheckPassed {
		t.Errorf("expected both passing checks, got: %+v", res)
	}

	// 3. Missing keys -> failure check
	writeFile(envPath, []byte("A=1\n"))
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
	writeFile(examplePath, []byte("A=1\n"))
	writeFile(envPath, []byte("A=1\nB=2\n"))
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
	writeFile(examplePath, []byte("A=1\nC=3\n"))
	writeFile(envPath, []byte("A=1\nB=2\n"))
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

func TestCheckEnvironmentalAlignmentDuplicateRef(t *testing.T) {
	engine := &Engine{
		ConfigDir: t.TempDir(),
		Compose: config.ComposeConfig{
			Services: map[string]config.ComposeService{
				"web": {
					Environment: map[string]string{"DATABASE_URL": ""},
				},
				"api": {
					Environment: map[string]string{"DATABASE_URL": ""},
				},
			},
		},
		Env: map[string]string{},
	}

	results := engine.checkEnvironmentalAlignment(t.Context())

	missingCount := 0
	for _, res := range results {
		if res.Name == "Variable DATABASE_URL missing" {
			missingCount++
		}
	}

	if missingCount != 1 {
		t.Fatalf("expected exactly 1 result for missing DATABASE_URL, got %d", missingCount)
	}
}

