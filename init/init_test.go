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
