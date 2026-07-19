package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderJSON(t *testing.T) {
	report := &DiagnosticsReport{
		Status:     StatusHealthy,
		DurationMs: 42,
		Checks: []CheckResult{
			{
				Group:  "Test Group",
				Name:   "Test Check",
				Status: CheckPassed,
			},
		},
	}

	var buf bytes.Buffer
	err := RenderJSON(&buf, report)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}

	var parsed DiagnosticsReport
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse rendered JSON: %v", err)
	}

	if parsed.Status != report.Status || parsed.DurationMs != report.DurationMs || len(parsed.Checks) != len(report.Checks) {
		t.Errorf("rendered JSON does not match input. Got: %+v", parsed)
	}
}

// TestRenderJSONOmitEmpty ensures optional fields are omitted when empty.
func TestRenderJSONOmitEmpty(t *testing.T) {
	report := &DiagnosticsReport{
		Status:     StatusHealthy,
		DurationMs: 0,
		Checks: []CheckResult{
			{Group: "G", Name: "N", Status: CheckPassed},
		},
	}

	var buf bytes.Buffer
	if err := RenderJSON(&buf, report); err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}

	// "error" and "mitigation" should not appear in output when they are empty strings.
	raw := buf.String()
	if strings.Contains(raw, `"error"`) {
		t.Errorf("expected 'error' field to be omitted when empty, got: %s", raw)
	}
	if strings.Contains(raw, `"mitigation"`) {
		t.Errorf("expected 'mitigation' field to be omitted when empty, got: %s", raw)
	}
}

func TestRenderText(t *testing.T) {
	report := &DiagnosticsReport{
		Status:     StatusEnvironmentBroken,
		DurationMs: 100,
		Checks: []CheckResult{
			{
				Group:      "Test Group",
				Name:       "Test Check",
				Status:     CheckFailed,
				Error:      "something went wrong",
				Mitigation: "fix it now",
			},
		},
	}

	var buf bytes.Buffer
	RenderText(&buf, report, true)

	outputStr := buf.String()
	if !strings.Contains(outputStr, "=== halo Diagnostics Report ===") {
		t.Errorf("missing header in text output: %s", outputStr)
	}
	if !strings.Contains(outputStr, "something went wrong") {
		t.Errorf("missing error details in text output: %s", outputStr)
	}
	if !strings.Contains(outputStr, "fix it now") {
		t.Errorf("missing mitigation details in text output: %s", outputStr)
	}
}

// TestRenderTextFailureAlwaysShowsError verifies that error detail is shown on
// CheckFailed results even when verbose is false.
func TestRenderTextFailureAlwaysShowsError(t *testing.T) {
	report := &DiagnosticsReport{
		Status:     StatusEnvironmentBroken,
		DurationMs: 5,
		Checks: []CheckResult{
			{
				Group:      "G",
				Name:       "N",
				Status:     CheckFailed,
				Error:      "critical failure detail",
				Mitigation: "do the thing",
			},
		},
	}

	var buf bytes.Buffer
	RenderText(&buf, report, false) // verbose = false

	outputStr := buf.String()
	if !strings.Contains(outputStr, "critical failure detail") {
		t.Errorf("expected error detail to be shown on failures even with verbose=false, got: %s", outputStr)
	}
}

// TestRenderTextWarning verifies CheckWarning rendering respects the verbose flag.
func TestRenderTextWarning(t *testing.T) {
	report := &DiagnosticsReport{
		Status:     StatusEnvironmentBroken,
		DurationMs: 1,
		Checks: []CheckResult{
			{
				Group:      "G",
				Name:       "Warn Check",
				Status:     CheckWarning,
				Error:      "minor warning detail",
				Mitigation: "maybe fix this",
			},
		},
	}

	// verbose=false: warning error detail should be hidden
	var buf bytes.Buffer
	RenderText(&buf, report, false)
	if strings.Contains(buf.String(), "minor warning detail") {
		t.Errorf("expected warning error to be hidden when verbose=false, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "maybe fix this") {
		t.Errorf("expected mitigation to always be shown for warnings, got: %s", buf.String())
	}

	// verbose=true: warning error detail should be visible
	buf.Reset()
	RenderText(&buf, report, true)
	if !strings.Contains(buf.String(), "minor warning detail") {
		t.Errorf("expected warning error to be shown when verbose=true, got: %s", buf.String())
	}
}

// TestRenderTextHealthy verifies healthy report rendering with summary line.
func TestRenderTextHealthy(t *testing.T) {
	report := &DiagnosticsReport{
		Status:     StatusHealthy,
		DurationMs: 12,
		Checks: []CheckResult{
			{Group: "Group A", Name: "Check 1", Status: CheckPassed},
			{Group: "Group A", Name: "Check 2", Status: CheckPassed},
		},
	}

	var buf bytes.Buffer
	RenderText(&buf, report, false)

	outputStr := buf.String()
	if !strings.Contains(outputStr, "healthy") {
		t.Errorf("expected 'healthy' in output, got: %s", outputStr)
	}
	// Summary line: "2 of 2 checks passed."
	if !strings.Contains(outputStr, "2 of 2 checks passed") {
		t.Errorf("expected summary line '2 of 2 checks passed', got: %s", outputStr)
	}
}

// TestRenderTextEmptyChecks verifies graceful handling of an empty checks slice.
func TestRenderTextEmptyChecks(t *testing.T) {
	report := &DiagnosticsReport{
		Status:     StatusHealthy,
		DurationMs: 0,
		Checks:     []CheckResult{},
	}

	var buf bytes.Buffer
	// Should not panic
	RenderText(&buf, report, false)

	outputStr := buf.String()
	if !strings.Contains(outputStr, "=== halo Diagnostics Report ===") {
		t.Errorf("missing header for empty report: %s", outputStr)
	}
	// No summary line expected when there are no checks
	if strings.Contains(outputStr, "of 0 checks") {
		t.Errorf("unexpected summary line for empty checks: %s", outputStr)
	}
}

// TestRenderTextSummaryWithFailures verifies the failure count in the summary line.
func TestRenderTextSummaryWithFailures(t *testing.T) {
	report := &DiagnosticsReport{
		Status:     StatusEnvironmentBroken,
		DurationMs: 8,
		Checks: []CheckResult{
			{Group: "G", Name: "Pass", Status: CheckPassed},
			{Group: "G", Name: "Fail1", Status: CheckFailed, Error: "e1"},
			{Group: "G", Name: "Fail2", Status: CheckFailed, Error: "e2"},
		},
	}

	var buf bytes.Buffer
	RenderText(&buf, report, false)

	outputStr := buf.String()
	if !strings.Contains(outputStr, "1 of 3 checks passed (2 failed)") {
		t.Errorf("expected summary '1 of 3 checks passed (2 failed)', got: %s", outputStr)
	}
}

// TestRenderTextSummaryWarningsNotCountedAsFailed verifies that warnings are
// excluded from the failure count in the summary line.
func TestRenderTextSummaryWarningsNotCountedAsFailed(t *testing.T) {
	report := &DiagnosticsReport{
		Status:     StatusEnvironmentBroken,
		DurationMs: 5,
		Checks: []CheckResult{
			{Group: "G", Name: "Pass", Status: CheckPassed},
			{Group: "G", Name: "Warn", Status: CheckWarning, Error: "w", Mitigation: "m"},
			{Group: "G", Name: "Fail", Status: CheckFailed, Error: "f"},
		},
	}

	var buf bytes.Buffer
	RenderText(&buf, report, false)

	outputStr := buf.String()
	// 1 passed, 1 warned, 1 failed → "1 of 3 checks passed (1 failed)"
	if !strings.Contains(outputStr, "1 of 3 checks passed (1 failed)") {
		t.Errorf("expected summary '1 of 3 checks passed (1 failed)' — warnings must not count as failures, got: %s", outputStr)
	}
}

// TestRenderTextNoColorEnvVar verifies ANSI codes are absent when NO_COLOR is set.
func TestRenderTextNoColorEnvVar(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	report := &DiagnosticsReport{
		Status:     StatusHealthy,
		DurationMs: 1,
		Checks: []CheckResult{
			{Group: "G", Name: "Pass", Status: CheckPassed},
		},
	}

	var buf bytes.Buffer
	RenderText(&buf, report, false)

	if strings.Contains(buf.String(), "\033[") {
		t.Errorf("expected no ANSI codes when NO_COLOR is set, got: %q", buf.String())
	}
}
