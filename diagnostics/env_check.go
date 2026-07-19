package diagnostics

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/marcelo-lipienski/halo/output"
)

func (e *Engine) extractReferencedEnvVars() []shellEnvRef {
	seen := make(map[string]bool)
	var refs []shellEnvRef

	addRefs := func(s string) {
		for _, ref := range extractShellEnvRefs(s) {
			if !seen[ref.name] {
				seen[ref.name] = true
				refs = append(refs, ref)
			}
		}
	}

	// Sort service names for deterministic environment extraction.
	var svcNames []string
	for name := range e.Compose.Services {
		svcNames = append(svcNames, name)
	}
	sort.Strings(svcNames)

	for _, svcName := range svcNames {
		svc := e.Compose.Services[svcName]
		for key, val := range svc.Environment {
			if val == "" {
				// Pass-through variable (e.g. - DB_PASSWORD or DB_PASSWORD:)
				// requires the variable to be set in the environment.
				if !seen[key] {
					seen[key] = true
					refs = append(refs, shellEnvRef{name: key, hasDefault: false})
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
		val, exists := os.LookupEnv(ref.name)
		if !exists {
			val, exists = e.Env[ref.name]
		}

		if !exists {
			// Variables with ${VAR:-default} have a fallback and are not required.
			if ref.hasDefault {
				continue
			}
			variablesCheckPassed = false
			results = append(results, output.CheckResult{
				Group:      "Environmental Alignment",
				Name:       fmt.Sprintf("Variable %s missing", ref.name),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("Environment variable %s is referenced in docker-compose.yml but not defined in .env or host environment", ref.name),
				Mitigation: fmt.Sprintf("Add %s=your_value to your .env file or export it in your environment", ref.name),
			})
		} else if val == "" {
			if ref.required {
				// ${VAR:?error} — variable must be set AND non-empty.
				variablesCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Environmental Alignment",
					Name:       fmt.Sprintf("Variable %s is required but empty", ref.name),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Environment variable %s uses the :? operator in docker-compose.yml and must be set to a non-empty value", ref.name),
					Mitigation: fmt.Sprintf("Set a non-empty value for %s in your .env file or host environment", ref.name),
				})
			} else if !ref.hasDefault {
				// Defined but empty and no inline default — warn, not fatal.
				mismatchedTypesPassed = false
				results = append(results, output.CheckResult{
					Group:      "Environmental Alignment",
					Name:       fmt.Sprintf("Variable %s is empty", ref.name),
					Status:     output.CheckWarning,
					Error:      fmt.Sprintf("Environment variable %s is defined but empty in .env/host environment and has no default fallback in docker-compose.yml", ref.name),
					Mitigation: fmt.Sprintf("Set a non-empty value for %s in your .env file or host environment", ref.name),
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
