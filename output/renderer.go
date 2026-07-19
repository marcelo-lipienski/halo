package output

import (
	"encoding/json"
	"fmt"
	"io"
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

// RenderText formats the report as ANSI text and writes to target writer
func RenderText(w io.Writer, report *DiagnosticsReport, verbose bool) {
	fmt.Fprintln(w, "=== halo Diagnostics Report ===")
	fmt.Fprintf(w, "Status:   ")
	switch report.Status {
	case StatusHealthy:
		fmt.Fprintf(w, "\033[32m%s\033[0m\n", report.Status)
	case StatusSystemFailure:
		fmt.Fprintf(w, "\033[31;1m%s\033[0m\n", report.Status)
	case StatusEnvironmentBroken:
		fmt.Fprintf(w, "\033[33;1m%s\033[0m\n", report.Status)
	}
	fmt.Fprintf(w, "Duration: %dms\n", report.DurationMs)

	currentGroup := ""
	passed := 0
	total := 0
	for _, check := range report.Checks {
		total++
		if check.Group != currentGroup {
			currentGroup = check.Group
			fmt.Fprintf(w, "\n[%s]\n", currentGroup)
		}

		if check.Status == CheckPassed {
			passed++
			fmt.Fprintf(w, "  \033[32m✓\033[0m %s\n", check.Name)
		} else if check.Status == CheckWarning {
			fmt.Fprintf(w, "  \033[33m⚠\033[0m %s\n", check.Name)
			if check.Error != "" && verbose {
				fmt.Fprintf(w, "    \033[90mWarning:\033[0m %s\n", check.Error)
			}
			if check.Mitigation != "" {
				fmt.Fprintf(w, "    \033[36mFix:\033[0m     %s\n", check.Mitigation)
			}
		} else {
			fmt.Fprintf(w, "  \033[31m✗\033[0m %s\n", check.Name)
			// Always show the error detail on failures — it's the actionable reason.
			if check.Error != "" {
				fmt.Fprintf(w, "    \033[90mError:\033[0m   %s\n", check.Error)
			}
			if check.Mitigation != "" {
				fmt.Fprintf(w, "    \033[36mFix:\033[0m     %s\n", check.Mitigation)
			}
		}
	}

	fmt.Fprintln(w)
	if total > 0 {
		failed := total - passed
		if failed == 0 {
			fmt.Fprintf(w, "\033[32m%d of %d checks passed.\033[0m\n", passed, total)
		} else {
			fmt.Fprintf(w, "\033[31m%d of %d checks passed (%d failed).\033[0m\n", passed, total, failed)
		}
	}
	fmt.Fprintln(w)
}

