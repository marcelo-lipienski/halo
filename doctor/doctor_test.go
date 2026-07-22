package doctor

import (
	"context"
	"path/filepath"
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

	// Run doctor on temp dir with nil config
	report := RunDoctor(context.Background(), tmpDir, nil)
	if report == nil {
		t.Fatal("expected report to be non-nil")
	}

	// It should have at least 5 checks
	if len(report.Checks) < 5 {
		t.Errorf("expected at least 5 checks, got %d", len(report.Checks))
	}

	// Test with a mock compose config memory limit exceeding host memory or normal
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
			"huge_app": {
				Deploy: config.ComposeDeploy{
					Resources: config.ComposeResources{
						Limits: config.ComposeResourceLimits{
							Memory: "1000000TiB",
						},
					},
				},
			},
			"invalid_app": {
				Deploy: config.ComposeDeploy{
					Resources: config.ComposeResources{
						Limits: config.ComposeResourceLimits{
							Memory: "invalid_limit",
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

	// Verify System Memory check has parsed memory limit warning
	hasMemoryCheck := false
	for _, check := range reportWithCompose.Checks {
		if check.Group == "Host Resources" && check.Name == "System Memory" {
			hasMemoryCheck = true
			if check.Status != "warning" {
				t.Errorf("expected memory check warning due to huge compose limit, got status %s", check.Status)
			}
		}
	}
	if !hasMemoryCheck {
		t.Error("expected to find a memory check in report")
	}

	// Compose memory limit within bounds
	compPass := &config.ComposeConfig{
		Services: map[string]config.ComposeService{
			"app": {
				Deploy: config.ComposeDeploy{
					Resources: config.ComposeResources{
						Limits: config.ComposeResourceLimits{
							Memory: "1K",
						},
					},
				},
			},
		},
	}
	reportPass := RunDoctor(context.Background(), tmpDir, compPass)
	for _, check := range reportPass.Checks {
		if check.Group == "Host Resources" && check.Name != "Free Disk Space" && check.Status != "passed" {
			t.Errorf("expected host resource check to pass, got: %+v", check)
		}
	}
}



func TestCheckComposeVersion(t *testing.T) {
	ctx := context.Background()
	res := checkComposeVersion(ctx)
	if res.Group != "System Prerequisites" {
		t.Errorf("unexpected group: %s", res.Group)
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
	// Test when all tools exist
	res := checkRequiredTools([]string{"ls"})
	if res.Status != "passed" {
		t.Errorf("expected passed status for 'ls', got %s", res.Status)
	}

	// Test when tool is missing
	resMissing := checkRequiredTools([]string{"non_existent_binary_xyz_123"})
	if resMissing.Status != "warning" {
		t.Errorf("expected warning status for missing binary, got %s", resMissing.Status)
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
