package diagnostics

import (
	"context"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/output"
	"github.com/moby/moby/client"
)

// Engine runs the diagnostic suites
type Engine struct {
	ConfigDir   string
	ComposePath string
	Env         map[string]string
	Compose     *config.ComposeConfig
	DockerCli   client.ContainerAPIClient
	AutoFix     bool
	DryRun      bool
	Interactive bool
}

// NewEngine instantiates a new Diagnostics Engine
func NewEngine(configDir, composePath string, env map[string]string, compose *config.ComposeConfig, dockerCli client.ContainerAPIClient) *Engine {
	return &Engine{
		ConfigDir:   configDir,
		ComposePath: composePath,
		Env:         env,
		Compose:     compose,
		DockerCli:   dockerCli,
	}
}

// Run executes the diagnostic check groups concurrently with appropriate timeouts
func (e *Engine) Run(ctx context.Context) *output.DiagnosticsReport {
	start := time.Now()
	var resultsA []output.CheckResult
	var resultsB []output.CheckResult
	var resultsC []output.CheckResult

	var wg sync.WaitGroup
	wg.Add(3)

	// Group A: Environmental Alignment
	go func() {
		defer wg.Done()
		gCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		resultsA = e.checkEnvironmentalAlignment(gCtx)
	}()

	// Group B: Network & Port Availability
	go func() {
		defer wg.Done()
		gCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		resultsB = e.checkNetworkAndPort(gCtx)
	}()

	// Group C: Volume & File Permissions
	go func() {
		defer wg.Done()
		gCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		resultsC = e.checkVolumeAndPermissions(gCtx)
	}()

	wg.Wait()

	var results []output.CheckResult
	results = append(results, resultsA...)
	results = append(results, resultsB...)
	results = append(results, resultsC...)

	// Determine overall status
	status := output.StatusHealthy
	for _, res := range results {
		if res.Status == output.CheckFailed {
			status = output.StatusEnvironmentBroken
			break
		}
	}

	report := &output.DiagnosticsReport{
		Status:     status,
		DurationMs: time.Since(start).Milliseconds(),
		Checks:     results,
	}

	e.redactReport(report)
	return report
}

func (e *Engine) getSensitiveValues() []string {
	var values []string
	isSensitiveKey := func(k string) bool {
		k = strings.ToUpper(k)
		return strings.Contains(k, "KEY") ||
			strings.Contains(k, "SECRET") ||
			strings.Contains(k, "PASSWORD") ||
			strings.Contains(k, "PASS") ||
			strings.Contains(k, "TOKEN") ||
			strings.Contains(k, "CREDENTIAL") ||
			strings.Contains(k, "AUTH") ||
			strings.Contains(k, "CERT") ||
			strings.Contains(k, "JWT") ||
			strings.Contains(k, "API")
	}

	// Scan project-level env
	for k, v := range e.Env {
		if isSensitiveKey(k) && v != "" && len(v) > 2 {
			values = append(values, v)
		}
	}

	// Scan host env
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			k, v := parts[0], parts[1]
			if isSensitiveKey(k) && v != "" && len(v) > 2 {
				values = append(values, v)
			}
		}
	}

	// Scan service env files
	for _, svc := range e.Compose.Services {
		svcEnv := e.loadServiceEnvFiles(svc)
		for k, v := range svcEnv {
			if isSensitiveKey(k) && v != "" && len(v) > 2 {
				values = append(values, v)
			}
		}
	}

	return values
}

func (e *Engine) redactReport(report *output.DiagnosticsReport) {
	sensitiveVals := e.getSensitiveValues()
	if len(sensitiveVals) == 0 {
		return
	}

	// Sort sensitive values by length descending so that if one secret is a substring of another,
	// the longer one gets replaced first.
	sort.Slice(sensitiveVals, func(i, j int) bool {
		return len(sensitiveVals[i]) > len(sensitiveVals[j])
	})

	redactStr := func(s string) string {
		for _, val := range sensitiveVals {
			s = strings.ReplaceAll(s, val, "[REDACTED]")
		}
		return s
	}

	for i := range report.Checks {
		report.Checks[i].Name = redactStr(report.Checks[i].Name)
		report.Checks[i].Error = redactStr(report.Checks[i].Error)
		report.Checks[i].Mitigation = redactStr(report.Checks[i].Mitigation)
	}
}
