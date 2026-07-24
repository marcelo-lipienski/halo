package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Status represents overall tool execution status.
type Status string

const (
	// StatusHealthy: config parsed and checks passed.
	StatusHealthy Status = "healthy"
	// StatusSystemFailure: missing config, docker down, or invalid usage. See ADR-0002.
	StatusSystemFailure Status = "system_failure"
	// StatusEnvironmentBroken: config parsed, but check(s) failed. See ADR-0002.
	StatusEnvironmentBroken Status = "environment_broken"
)

// CheckStatus represents individual check status.
type CheckStatus string

const (
	// CheckPassed: check passed.
	CheckPassed CheckStatus = "passed"
	// CheckFailed: check failed.
	CheckFailed CheckStatus = "failed"
	// CheckWarning: non-critical check issue.
	CheckWarning CheckStatus = "warning"
)

// CheckResult is a single check's outcome.
type CheckResult struct {
	Group      string      `json:"group"`
	Name       string      `json:"name"`
	Status     CheckStatus `json:"status"`
	Error      string      `json:"error,omitempty"`
	Mitigation string      `json:"mitigation,omitempty"`
}

// DiagnosticsReport is the test run summary. See ADR-0002.
type DiagnosticsReport struct {
	Status     Status        `json:"status"`
	DurationMs int64         `json:"duration_ms"`
	Checks     []CheckResult `json:"checks"`
}

// RenderJSON writes minified JSON report. See ADR-0002.
func RenderJSON(w io.Writer, report *DiagnosticsReport) error {
	enc := json.NewEncoder(w)
	return enc.Encode(report)
}

// isTTY checks if writer is a terminal.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// UseColor returns true if color is allowed.
func UseColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isTTY(w)
}

// Colorize wraps s in ANSI escape sequence.
func Colorize(s, ansi string, color bool) string {
	if !color {
		return s
	}
	return "\033[" + ansi + "m" + s + "\033[0m"
}

// RenderText writes ANSI/plain report to writer. See ADR-0002.
func RenderText(w io.Writer, report *DiagnosticsReport, verbose bool) {
	color := UseColor(w)

	fmt.Fprintln(w, "=== halo Diagnostics Report ===")
	fmt.Fprintf(w, "Status:   ")
	switch report.Status {
	case StatusHealthy:
		fmt.Fprintln(w, Colorize(string(report.Status), "32", color))
	case StatusSystemFailure:
		fmt.Fprintln(w, Colorize(string(report.Status), "31;1", color))
	case StatusEnvironmentBroken:
		fmt.Fprintln(w, Colorize(string(report.Status), "33;1", color))
	}
	fmt.Fprintf(w, "Duration: %dms\n", report.DurationMs)

	currentGroup := ""
	passed := 0
	warned := 0
	total := 0
	for _, check := range report.Checks {
		total++
		if check.Group != currentGroup {
			currentGroup = check.Group
			fmt.Fprintf(w, "\n[%s]\n", currentGroup)
		}

		switch check.Status {
		case CheckPassed:
			passed++
			fmt.Fprintf(w, "  %s %s\n", Colorize("✓", "32", color), check.Name)
		case CheckWarning:
			warned++
			fmt.Fprintf(w, "  %s %s\n", Colorize("⚠", "33", color), check.Name)
			if check.Error != "" && verbose {
				fmt.Fprintf(w, "    %s %s\n", Colorize("Warning:", "90", color), check.Error)
			}
			if check.Mitigation != "" {
				fmt.Fprintf(w, "    %s     %s\n", Colorize("Fix:", "36", color), check.Mitigation)
			}
		default: // CheckFailed
			fmt.Fprintf(w, "  %s %s\n", Colorize("✗", "31", color), check.Name)
			if check.Error != "" {
				fmt.Fprintf(w, "    %s   %s\n", Colorize("Error:", "90", color), check.Error)
			}
			if check.Mitigation != "" {
				fmt.Fprintf(w, "    %s     %s\n", Colorize("Fix:", "36", color), check.Mitigation)
			}
		}
	}

	fmt.Fprintln(w)
	if total > 0 {
		// Warnings do not count as failures.
		failed := total - passed - warned
		if failed == 0 {
			fmt.Fprintln(w, Colorize(fmt.Sprintf("%d of %d checks passed.", passed, total), "32", color))
		} else {
			fmt.Fprintln(w, Colorize(fmt.Sprintf("%d of %d checks passed (%d failed).", passed, total, failed), "31", color))
		}
	}
	fmt.Fprintln(w)
}
