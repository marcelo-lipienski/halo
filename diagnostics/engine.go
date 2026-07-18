package diagnostics

import (
	"context"
	"sync"
	"time"

	"github.com/moby/moby/client"
	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/output"
)

// Engine runs the diagnostic suites
type Engine struct {
	ConfigDir string
	Env       map[string]string
	Compose   *config.ComposeConfig
	DockerCli client.ContainerAPIClient
}

// NewEngine instantiates a new Diagnostics Engine
func NewEngine(configDir string, env map[string]string, compose *config.ComposeConfig, dockerCli client.ContainerAPIClient) *Engine {
	return &Engine{
		ConfigDir: configDir,
		Env:       env,
		Compose:   compose,
		DockerCli: dockerCli,
	}
}

// Run executes the diagnostic check groups concurrently with appropriate timeouts
func (e *Engine) Run(ctx context.Context) *output.DiagnosticsReport {
	start := time.Now()
	var results []output.CheckResult
	var mu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(3)

	runGroup := func(groupCtx context.Context, checkFn func(context.Context) []output.CheckResult) {
		defer wg.Done()
		groupResults := checkFn(groupCtx)
		mu.Lock()
		results = append(results, groupResults...)
		mu.Unlock()
	}

	// Group A: Environmental Alignment
	go func() {
		gCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		runGroup(gCtx, e.checkEnvironmentalAlignment)
	}()

	// Group B: Network & Port Availability
	go func() {
		gCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		runGroup(gCtx, e.checkNetworkAndPort)
	}()

	// Group C: Volume & File Permissions
	go func() {
		gCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		runGroup(gCtx, e.checkVolumeAndPermissions)
	}()

	wg.Wait()

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
