package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/output"
)

func TestCheckEnvExampleDrift(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	examplePath := filepath.Join(tmpDir, ".env.example")
	engine := &Engine{}

	// 1. No .env.example
	res, err := engine.CheckEnvExampleDrift(envPath, examplePath)
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
	res, err = engine.CheckEnvExampleDrift(envPath, examplePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].Status != output.CheckPassed || res[1].Status != output.CheckPassed {
		t.Errorf("expected both passing checks, got: %+v", res)
	}

	// 3. Missing keys -> failure check
	writeFile(envPath, []byte("A=1\n"))
	res, err = engine.CheckEnvExampleDrift(envPath, examplePath)
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
	res, err = engine.CheckEnvExampleDrift(envPath, examplePath)
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
	res, err = engine.CheckEnvExampleDrift(envPath, examplePath)
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
		Compose: &config.ComposeConfig{
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

func TestCheckEnvExampleDriftAutoFixAndDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	examplePath := filepath.Join(tmpDir, ".env.example")

	_ = os.WriteFile(examplePath, []byte("FOO=bar\nBAR=baz\n"), 0644)
	_ = os.WriteFile(envPath, []byte("FOO=bar\n"), 0644)

	// DryRun test
	eDry := &Engine{DryRun: true}
	resDry, err := eDry.CheckEnvExampleDrift(envPath, examplePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(resDry) == 0 || resDry[0].Status != output.CheckFailed {
		t.Errorf("expected failed dry-run result, got: %+v", resDry)
	}

	// AutoFix test
	eFix := &Engine{AutoFix: true}
	resFix, err := eFix.CheckEnvExampleDrift(envPath, examplePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(resFix) == 0 || resFix[0].Status != output.CheckPassed {
		t.Errorf("expected passed auto-fixed result, got: %+v", resFix)
	}

	envContent, _ := os.ReadFile(envPath)
	if !strings.Contains(string(envContent), "BAR=baz") {
		t.Errorf("expected .env to contain BAR=baz after auto-fix, got: %s", string(envContent))
	}
}

func TestCheckEnvironmentalAlignmentAutoFixConsolidation(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	examplePath := filepath.Join(tmpDir, ".env.example")
	composePath := filepath.Join(tmpDir, "docker-compose.yml")

	_ = os.WriteFile(examplePath, []byte("APP_ENV=dev\nAPI_KEY=my-secret\n"), 0644)
	_ = os.WriteFile(envPath, []byte("APP_ENV=dev\n"), 0644)
	_ = os.WriteFile(composePath, []byte("services:\n  app:\n    image: nginx:1.25.0\n"), 0644)

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		t.Fatal(err)
	}
	envMap, err := config.ParseEnv(envPath)
	if err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(tmpDir, composePath, envMap, comp, nil)
	engine.EnvPath = envPath
	engine.AutoFix = true

	report := engine.Run(t.Context())

	for _, check := range report.Checks {
		if check.Group == "Environmental Alignment" && check.Status == output.CheckFailed {
			t.Errorf("unexpected failed check in Environmental Alignment after auto-fix: %+v", check)
		}
	}

	// Verify that API_KEY was added to .env
	updatedEnv, _ := os.ReadFile(envPath)
	if !strings.Contains(string(updatedEnv), "API_KEY=my-secret") {
		t.Errorf("expected API_KEY to be auto-fixed into .env, content: %s", string(updatedEnv))
	}
}
