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
		t.Errorf("missing error details in text output when verbose is true: %s", outputStr)
	}
	if !strings.Contains(outputStr, "fix it now") {
		t.Errorf("missing mitigation details in text output: %s", outputStr)
	}
}
