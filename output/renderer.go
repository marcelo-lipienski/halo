package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Status represents the overall execution status of the diagnostics tool
type Status string

const (
	// StatusHealthy indicates all configuration parsed and all checks passed
	StatusHealthy Status = "healthy"
	// StatusSystemFailure indicates configuration files are missing, docker is down, or command usage is incorrect
	StatusSystemFailure Status = "system_failure"
	// StatusEnvironmentBroken indicates configuration parsed, but check(s) failed
	StatusEnvironmentBroken Status = "environment_broken"
)

// CheckStatus represents individual check results
type CheckStatus string

const (
	// CheckPassed indicates the check passed successfully
	CheckPassed CheckStatus = "passed"
	// CheckFailed indicates the check failed
	CheckFailed CheckStatus = "failed"
	// CheckWarning indicates the check found a non-critical issue
	CheckWarning CheckStatus = "warning"
)

// CheckResult contains metadata and output of a diagnostic check execution
type CheckResult struct {
	Group      string      `json:"group"`
	Name       string      `json:"name"`
	Status     CheckStatus `json:"status"`
	Error      string      `json:"error,omitempty"`
	Mitigation string      `json:"mitigation,omitempty"`
}

// DiagnosticsReport aggregates check results and durations
type DiagnosticsReport struct {
	Status     Status        `json:"status"`
	DurationMs int64         `json:"duration_ms"`
	Checks     []CheckResult `json:"checks"`
}

// RenderJSON formats the report to minified JSON and writes to target writer
func RenderJSON(w io.Writer, report *DiagnosticsReport) error {
	enc := json.NewEncoder(w)
	return enc.Encode(report)
}

// isTTY returns true if the given writer is an os.File connected to a terminal.
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

// useColor returns true when ANSI colour output should be emitted.
// It is suppressed when the NO_COLOR environment variable is set (any value)
// or when the target writer is not a TTY.
func useColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isTTY(w)
}

// colorize wraps s in the given ANSI escape sequence when colour is enabled.
func colorize(s, ansi string, color bool) string {
	if !color {
		return s
	}
	return "\033[" + ansi + "m" + s + "\033[0m"
}

// RenderText formats the report as ANSI text and writes to target writer.
// ANSI colour sequences are suppressed when NO_COLOR is set or the writer is
// not a terminal.
func RenderText(w io.Writer, report *DiagnosticsReport, verbose bool) {
	color := useColor(w)

	fmt.Fprintln(w, "=== halo Diagnostics Report ===")
	fmt.Fprintf(w, "Status:   ")
	switch report.Status {
	case StatusHealthy:
		fmt.Fprintln(w, colorize(string(report.Status), "32", color))
	case StatusSystemFailure:
		fmt.Fprintln(w, colorize(string(report.Status), "31;1", color))
	case StatusEnvironmentBroken:
		fmt.Fprintln(w, colorize(string(report.Status), "33;1", color))
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
			fmt.Fprintf(w, "  %s %s\n", colorize("✓", "32", color), check.Name)
		case CheckWarning:
			warned++
			fmt.Fprintf(w, "  %s %s\n", colorize("⚠", "33", color), check.Name)
			if check.Error != "" && verbose {
				fmt.Fprintf(w, "    %s %s\n", colorize("Warning:", "90", color), check.Error)
			}
			if check.Mitigation != "" {
				fmt.Fprintf(w, "    %s     %s\n", colorize("Fix:", "36", color), check.Mitigation)
			}
		default: // CheckFailed
			fmt.Fprintf(w, "  %s %s\n", colorize("✗", "31", color), check.Name)
			// Always show the error detail on failures — it's the actionable reason.
			if check.Error != "" {
				fmt.Fprintf(w, "    %s   %s\n", colorize("Error:", "90", color), check.Error)
			}
			if check.Mitigation != "" {
				fmt.Fprintf(w, "    %s     %s\n", colorize("Fix:", "36", color), check.Mitigation)
			}
		}
	}

	fmt.Fprintln(w)
	if total > 0 {
		// Warnings are non-fatal: they do not count as failures in the summary.
		failed := total - passed - warned
		if failed == 0 {
			fmt.Fprintln(w, colorize(fmt.Sprintf("%d of %d checks passed.", passed, total), "32", color))
		} else {
			fmt.Fprintln(w, colorize(fmt.Sprintf("%d of %d checks passed (%d failed).", passed, total, failed), "31", color))
		}
	}
	fmt.Fprintln(w)
}
