package init_cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsPlaceholder(t *testing.T) {
	tests := []struct {
		val      string
		expected bool
	}{
		{"", true},
		{" ", true},
		{"changeme", true},
		{"CHANGEME", true},
		{"TODO", true},
		{"replace_me", true},
		{"your_api_key", true},
		{"YOUR_PASSWORD", true},
		{"<required>", true},
		{"<insert here>", true},
		{"valid_value", false},
		{"12345", false},
	}

	for _, tt := range tests {
		if got := IsPlaceholder(tt.val); got != tt.expected {
			t.Errorf("IsPlaceholder(%q) = %v, want %v", tt.val, got, tt.expected)
		}
	}
}

func TestMergeEnvFiles(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name           string
		exampleContent string
		targetContent  string
		dryRun         bool
		targetExists   bool
		expectedAdded  []string
		expectedTotal  int
		expectedOutput string
	}{
		{
			name: "Fresh init",
			exampleContent: `
# Database
DB_URL=<required>
DB_PORT=5432
`,
			targetExists:  false,
			expectedAdded: []string{"DB_URL", "DB_PORT"},
			expectedTotal: 2,
			expectedOutput: `
# Database
DB_URL=<required>
DB_PORT=5432
`,
		},
		{
			name:           "Idempotent init",
			exampleContent: `DB_URL=<required>`,
			targetContent:  `DB_URL=postgres://localhost`,
			targetExists:   true,
			expectedAdded:  nil,
			expectedTotal:  1,
			expectedOutput: `DB_URL=postgres://localhost`,
		},
		{
			name: "Partial merge",
			exampleContent: `
DB_URL=<required>
DB_PORT=5432
`,
			targetContent: `DB_URL=postgres://localhost`,
			targetExists:  true,
			expectedAdded: []string{"DB_PORT"},
			expectedTotal: 1,
			expectedOutput: `DB_URL=postgres://localhost

# Added by halo init
DB_PORT=5432
`,
		},
		{
			name:           "Dry run",
			exampleContent: `DB_URL=<required>`,
			targetExists:   false,
			dryRun:         true,
			expectedAdded:  []string{"DB_URL"},
			expectedTotal:  0,
			expectedOutput: ``,
		},
		{
			name: "Duplicate keys in example file (fresh init)",
			exampleContent: `
DB_URL=postgres://localhost
DB_URL=postgres://other
DB_PORT=5432
`,
			targetExists:  false,
			expectedAdded: []string{"DB_URL", "DB_PORT"},
			expectedTotal: 2,
			expectedOutput: `
DB_URL=postgres://localhost
DB_URL=postgres://other
DB_PORT=5432
`,
		},
		{
			name: "Duplicate keys in example file with target existing",
			exampleContent: `
DB_URL=postgres://localhost
DB_URL=postgres://other
DB_PORT=5432
`,
			targetContent: `DB_HOST=localhost`,
			targetExists:  true,
			expectedAdded: []string{"DB_URL", "DB_PORT"},
			expectedTotal: 2,
			expectedOutput: `DB_HOST=localhost

# Added by halo init
DB_URL=postgres://localhost
DB_PORT=5432
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exampleFile := filepath.Join(tmpDir, "example_"+tt.name)
			targetFile := filepath.Join(tmpDir, "target_"+tt.name)

			if err := os.WriteFile(exampleFile, []byte(tt.exampleContent), 0644); err != nil {
				t.Fatalf("failed to write example file: %v", err)
			}

			if tt.targetExists {
				if err := os.WriteFile(targetFile, []byte(tt.targetContent), 0644); err != nil {
					t.Fatalf("failed to write target file: %v", err)
				}
			}

			res, err := MergeEnvFiles(exampleFile, targetFile, tt.dryRun)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(res.Added) != len(tt.expectedAdded) {
				t.Errorf("expected %d added, got %d", len(tt.expectedAdded), len(res.Added))
			}

			for i, key := range tt.expectedAdded {
				if i < len(res.Added) && res.Added[i].Key != key {
					t.Errorf("expected added key %q, got %q", key, res.Added[i].Key)
				}
			}

			if !tt.dryRun {
				content, err := os.ReadFile(targetFile)
				if err != nil {
					t.Fatalf("failed to read target file: %v", err)
				}
				if string(content) != tt.expectedOutput {
					t.Errorf("expected output %q, got %q", tt.expectedOutput, string(content))
				}
			}
		})
	}
}

func TestMergeEnvFiles_MissingExample(t *testing.T) {
	_, err := MergeEnvFiles("nonexistent", "target", false)
	if err == nil {
		t.Error("expected error for missing example file")
	}
}

func TestMergeEnvFilesEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	examplePath := filepath.Join(tmpDir, ".env.example")
	targetPath := filepath.Join(tmpDir, ".env")

	// 1. Export syntax and quoted values
	exampleContent := "export DB_HOST=\"localhost\"\nexport DB_PASS='secret'\n"
	_ = os.WriteFile(examplePath, []byte(exampleContent), 0644)
	_ = os.WriteFile(targetPath, []byte("EXISTING=1"), 0644) // no trailing newline

	res, err := MergeEnvFiles(examplePath, targetPath, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Added) != 2 {
		t.Errorf("expected 2 keys added, got %d", len(res.Added))
	}
}

func BenchmarkMergeEnvFiles(b *testing.B) {
	tmpDir := b.TempDir()
	exampleFile := filepath.Join(tmpDir, ".env.example")
	targetFile := filepath.Join(tmpDir, ".env")
	content := "DB_HOST=localhost\nDB_PORT=5432\nAPI_KEY=secret\n"
	_ = os.WriteFile(exampleFile, []byte(content), 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = MergeEnvFiles(exampleFile, targetFile, false)
	}
}
