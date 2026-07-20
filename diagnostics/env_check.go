package diagnostics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/output"
)

type svcEnvRef struct {
	ref         shellEnvRef
	serviceName string
}

func (e *Engine) extractReferencedEnvVars() []svcEnvRef {
	var refs []svcEnvRef

	// Sort service names for deterministic environment extraction.
	var svcNames []string
	for name := range e.Compose.Services {
		svcNames = append(svcNames, name)
	}
	sort.Strings(svcNames)

	for _, svcName := range svcNames {
		svc := e.Compose.Services[svcName]
		seen := make(map[string]bool)

		addRefs := func(s string) {
			for _, ref := range extractShellEnvRefs(s) {
				if !seen[ref.name] {
					seen[ref.name] = true
					refs = append(refs, svcEnvRef{ref: ref, serviceName: svcName})
				}
			}
		}

		for key, val := range svc.Environment {
			if val == "" {
				if !seen[key] {
					seen[key] = true
					refs = append(refs, svcEnvRef{
						ref:         shellEnvRef{name: key, hasDefault: false},
						serviceName: svcName,
					})
				}
			} else {
				addRefs(val)
			}
		}
		for _, port := range svc.Ports {
			addRefs(port)
		}
		for _, vol := range svc.Volumes {
			addRefs(vol.Source)
			addRefs(vol.Target)
		}
		if svc.Image != "" {
			addRefs(svc.Image)
		}
		if svc.ContainerName != "" {
			addRefs(svc.ContainerName)
		}
		for _, ep := range svc.Entrypoint {
			addRefs(ep)
		}
		for _, cmd := range svc.Command {
			addRefs(cmd)
		}
	}
	return refs
}

func (e *Engine) loadServiceEnvFiles(svc config.ComposeService) map[string]string {
	svcEnv := make(map[string]string)
	for _, ef := range svc.EnvFiles {
		resolvedPath := resolveShellExpr(ef.File, e.Env)
		if resolvedPath == "" {
			continue
		}
		path := resolvedPath
		if !filepath.IsAbs(path) {
			baseDir := ef.BaseDir
			if baseDir == "" {
				baseDir = e.ConfigDir
			}
			path = filepath.Join(baseDir, path)
		}
		path = filepath.Clean(path)

		// Read and parse env file
		if vars, err := config.ParseEnv(path); err == nil {
			for k, v := range vars {
				svcEnv[k] = v
			}
		}
	}
	return svcEnv
}

func (e *Engine) checkEnvironmentalAlignment(ctx context.Context) []output.CheckResult {
	var results []output.CheckResult

	if err := ctx.Err(); err != nil {
		results = append(results, output.CheckResult{
			Group:      "Environmental Alignment",
			Name:       "Check Timeout",
			Status:     output.CheckFailed,
			Error:      fmt.Sprintf("Environmental alignment check was cancelled: %v", err),
			Mitigation: "Verify local performance and resource allocation.",
		})
		return results
	}

	refs := e.extractReferencedEnvVars()

	variablesCheckPassed := true
	mismatchedTypesPassed := true

	for _, ref := range refs {
		select {
		case <-ctx.Done():
			results = append(results, output.CheckResult{
				Group:      "Environmental Alignment",
				Name:       "Check Timeout",
				Status:     output.CheckFailed,
				Error:      "Environmental alignment check timed out",
				Mitigation: "Verify local performance and resource allocation.",
			})
			return results
		default:
		}

		// Check system environment variable first (takes precedence over .env).
		val, exists := os.LookupEnv(ref.ref.name)
		if !exists {
			val, exists = e.Env[ref.ref.name]
		}
		if !exists && ref.serviceName != "" {
			svc := e.Compose.Services[ref.serviceName]
			svcEnv := e.loadServiceEnvFiles(svc)
			val, exists = svcEnv[ref.ref.name]
		}

		if !exists {
			// Variables with ${VAR:-default} have a fallback and are not required.
			if ref.ref.hasDefault {
				continue
			}
			variablesCheckPassed = false
			results = append(results, output.CheckResult{
				Group:      "Environmental Alignment",
				Name:       fmt.Sprintf("Variable %s missing", ref.ref.name),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("Environment variable %s is referenced in docker-compose.yml but not defined in .env or host environment", ref.ref.name),
				Mitigation: fmt.Sprintf("Add %s=your_value to your .env file or export it in your environment", ref.ref.name),
			})
		} else if val == "" {
			if ref.ref.required {
				// ${VAR:?error} — variable must be set AND non-empty.
				variablesCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Environmental Alignment",
					Name:       fmt.Sprintf("Variable %s is required but empty", ref.ref.name),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Environment variable %s uses the :? operator in docker-compose.yml and must be set to a non-empty value", ref.ref.name),
					Mitigation: fmt.Sprintf("Set a non-empty value for %s in your .env file or host environment", ref.ref.name),
				})
			} else if !ref.ref.hasDefault {
				// Defined but empty and no inline default — warn, not fatal.
				mismatchedTypesPassed = false
				results = append(results, output.CheckResult{
					Group:      "Environmental Alignment",
					Name:       fmt.Sprintf("Variable %s is empty", ref.ref.name),
					Status:     output.CheckWarning,
					Error:      fmt.Sprintf("Environment variable %s is defined but empty in .env/host environment and has no default fallback in docker-compose.yml", ref.ref.name),
					Mitigation: fmt.Sprintf("Set a non-empty value for %s in your .env file or host environment", ref.ref.name),
				})
			}
		}
	}

	if variablesCheckPassed {
		results = append(results, output.CheckResult{
			Group:  "Environmental Alignment",
			Name:   "Variables Check",
			Status: output.CheckPassed,
		})
	}
	if mismatchedTypesPassed {
		results = append(results, output.CheckResult{
			Group:  "Environmental Alignment",
			Name:   "Mismatched Types Check",
			Status: output.CheckPassed,
		})
	}

	return results
}
