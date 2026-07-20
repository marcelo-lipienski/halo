package diagnostics

import (
	"context"
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

	return &output.DiagnosticsReport{
		Status:     status,
		DurationMs: time.Since(start).Milliseconds(),
		Checks:     results,
	}
}
