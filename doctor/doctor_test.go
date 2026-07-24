package doctor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/output"
)

func TestParseBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
		wantErr  bool
	}{
		{"50M", 50 * 1024 * 1024, false},
		{"512MiB", 512 * 1024 * 1024, false},
		{"2G", 2 * 1024 * 1024 * 1024, false},
		{"2GiB", 2 * 1024 * 1024 * 1024, false},
		{"1024K", 1024 * 1024, false},
		{"100KiB", 100 * 1024, false},
		{"1TiB", 1 * 1024 * 1024 * 1024 * 1024, false},
		{"50Mi", 50 * 1024 * 1024, false},
		{"100b", 100, false},
		{"1.5g", uint64(1.5 * 1024 * 1024 * 1024), false},
		{"500.5mb", uint64(500.5 * 1024 * 1024), false},
		{"0.5GiB", uint64(0.5 * 1024 * 1024 * 1024), false},
		{"", 0, false},
		{"50", 50, false},
		{"invalid", 0, true},
		{"1.2.3G", 0, true},
		{"100XB", 100, false},
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
		{1024 * 1024 * 1024 * 1024 * 3, "3.0 TiB"},
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

	tests := []struct {
		name               string
		compose            *config.ComposeConfig
		minChecks          int
		verifyMemoryStatus output.CheckStatus
	}{
		{
			name:               "nil compose config",
			compose:            nil,
			minChecks:          5,
			verifyMemoryStatus: output.CheckPassed,
		},
		{
			name: "huge memory limit warning",
			compose: &config.ComposeConfig{
				Services: map[string]config.ComposeService{
					"app": {
						Deploy: config.ComposeDeploy{
							Resources: config.ComposeResources{
								Limits: config.ComposeResourceLimits{Memory: "512M"},
							},
						},
					},
					"huge_app": {
						Deploy: config.ComposeDeploy{
							Resources: config.ComposeResources{
								Limits: config.ComposeResourceLimits{Memory: "1000000TiB"},
							},
						},
					},
					"invalid_app": {
						Deploy: config.ComposeDeploy{
							Resources: config.ComposeResources{
								Limits: config.ComposeResourceLimits{Memory: "invalid_limit"},
							},
						},
					},
				},
			},
			minChecks:          5,
			verifyMemoryStatus: output.CheckWarning,
		},
		{
			name: "small memory limit pass",
			compose: &config.ComposeConfig{
				Services: map[string]config.ComposeService{
					"app": {
						Deploy: config.ComposeDeploy{
							Resources: config.ComposeResources{
								Limits: config.ComposeResourceLimits{Memory: "1K"},
							},
						},
					},
				},
			},
			minChecks:          5,
			verifyMemoryStatus: output.CheckPassed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report := RunDoctor(context.Background(), tmpDir, tc.compose)
			if report == nil {
				t.Fatal("expected report to be non-nil")
			}
			if len(report.Checks) < tc.minChecks {
				t.Errorf("expected at least %d checks, got %d", tc.minChecks, len(report.Checks))
			}
			for _, check := range report.Checks {
				if check.Group == "Host Resources" && check.Name == "System Memory" {
					if check.Status != tc.verifyMemoryStatus {
						t.Errorf("memory check status = %s, expected %s", check.Status, tc.verifyMemoryStatus)
					}
				}
			}
		})
	}
}

func BenchmarkRunDoctor(b *testing.B) {
	tmpDir := b.TempDir()
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RunDoctor(ctx, tmpDir, nil)
	}
}

func TestCheckComposeVersion(t *testing.T) {
	ctx := context.Background()
	res := checkComposeVersion(ctx)
	if res.Group != "System Prerequisites" {
		t.Errorf("unexpected group: %s", res.Group)
	}
}

func TestCheckComposeVersionCanceledCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := checkComposeVersion(ctx)
	if res.Status != output.CheckFailed {
		t.Errorf("expected CheckFailed on canceled context, got %s", res.Status)
	}
}

func TestParseComposeVersionStr(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Docker Compose version v2.24.1", "version v2.24.1"},
		{"  version 1.29.2  ", "version 1.29.2"},
		{"custom build 1.0", "custom build 1.0"},
	}
	for _, tc := range tests {
		got := parseComposeVersionStr(tc.input)
		if got != tc.expected {
			t.Errorf("parseComposeVersionStr(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestCheckRequiredTools(t *testing.T) {
	ctx := context.Background()
	// Test when all tools exist
	res := checkRequiredTools(ctx, []string{"ls"})
	if res.Status != "passed" {
		t.Errorf("expected passed status for 'ls', got %s", res.Status)
	}

	// Test when tool is missing
	resMissing := checkRequiredTools(ctx, []string{"non_existent_binary_xyz_123"})
	if resMissing.Status != "warning" {
		t.Errorf("expected warning status for missing binary, got %s", resMissing.Status)
	}

	// Test context cancellation
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	resCanceled := checkRequiredTools(canceledCtx, []string{"ls", "make"})
	if resCanceled.Status != "failed" {
		t.Errorf("expected failed status on canceled context, got %s", resCanceled.Status)
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

	// Non-existent directory handling
	_, errInvalid := GetFreeDiskSpace(filepath.Join(tmpDir, "non-existent-sub-dir-12345"))
	if errInvalid == nil {
		t.Error("expected error for non-existent path in GetFreeDiskSpace")
	}
}
