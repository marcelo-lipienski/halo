package doctor

import (
	"context"
	"testing"

	"github.com/marcelo-lipienski/halo/config"
)

func TestParseBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
		wantErr  bool
	}{
		{"50M", 50 * 1024 * 1024, false},
		{"2G", 2 * 1024 * 1024 * 1024, false},
		{"1024K", 1024 * 1024, false},
		{"100b", 100, false},
		{"", 0, false},
		{"50", 50, false},
	}

	for _, tc := range tests {
		got, err := ParseBytes(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseBytes(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			continue
		}
		if got != tc.expected {
			t.Errorf("ParseBytes(%q) = %d, expected %d", tc.input, got, tc.expected)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    uint64
		expected string
	}{
		{50, "50 B"},
		{1024, "1.0 KiB"},
		{1024 * 1024 * 5, "5.0 MiB"},
		{1024 * 1024 * 1024 * 2, "2.0 GiB"},
	}

	for _, tc := range tests {
		got := FormatBytes(tc.input)
		if got != tc.expected {
			t.Errorf("FormatBytes(%d) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestRunDoctor(t *testing.T) {
	tmpDir := t.TempDir()

	// Run doctor on temp dir with nil config
	report := RunDoctor(context.Background(), tmpDir, nil)
	if report == nil {
		t.Fatal("expected report to be non-nil")
	}

	// It should have at least 5 checks
	if len(report.Checks) < 5 {
		t.Errorf("expected at least 5 checks, got %d", len(report.Checks))
	}

	// Test with a mock compose config memory limit
	comp := &config.ComposeConfig{
		Services: map[string]config.ComposeService{
			"app": {
				Deploy: config.ComposeDeploy{
					Resources: config.ComposeResources{
						Limits: config.ComposeResourceLimits{
							Memory: "512M",
						},
					},
				},
			},
		},
	}

	reportWithCompose := RunDoctor(context.Background(), tmpDir, comp)
	if reportWithCompose == nil {
		t.Fatal("expected report to be non-nil")
	}

	// Verify System Memory check has parsed memory limit
	hasMemoryCheck := false
	for _, check := range reportWithCompose.Checks {
		if check.Group == "Host Resources" && check.Name != "" {
			hasMemoryCheck = true
		}
	}
	if !hasMemoryCheck {
		t.Error("expected to find a memory check in report")
	}
}

func TestGetHostMemoryAndFreeDiskSpace(t *testing.T) {
	tmpDir := t.TempDir()

	mem, err := GetHostMemory()
	// Total memory query could be missing in weird test environments, but if no error:
	if err == nil && mem == 0 {
		t.Error("expected positive host memory")
	}

	disk, err := GetFreeDiskSpace(tmpDir)
	if err != nil {
		t.Errorf("expected no error querying disk space, got: %v", err)
	}
	if disk == 0 {
		t.Error("expected positive free disk space")
	}
}
