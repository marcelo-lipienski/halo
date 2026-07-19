package diagnostics

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/marcelo-lipienski/halo/output"
)

var envVarRegex = regexp.MustCompile(`\$(?:\{([a-zA-Z0-9_]+)(?::?-([^}]*))?\}|([a-zA-Z0-9_]+))`)

type envVarRef struct {
	name       string
	hasDefault bool
}

func (e *Engine) extractReferencedEnvVars() []envVarRef {
	var refs []envVarRef
	seen := make(map[string]bool)

	parseStr := func(s string) {
		matches := envVarRegex.FindAllStringSubmatchIndex(s, -1)
		for _, matchIdx := range matches {
			start := matchIdx[0]
			if start > 0 && s[start-1] == '$' {
				// Escaped, skip
				continue
			}
			match := envVarRegex.FindStringSubmatch(s[start:matchIdx[1]])
			if len(match) == 0 {
				continue
			}

			varName := ""
			hasDefault := false
			if len(match) > 1 && match[1] != "" {
				varName = match[1]
				// Capture group 2 holds the default value portion (e.g. "80" in ${PORT:-80}).
				// A non-empty capture group 2 means a default was explicitly declared.
				if len(match) > 2 && match[2] != "" {
					hasDefault = true
				}
			} else if len(match) > 3 && match[3] != "" {
				varName = match[3]
			}

			if varName != "" && !seen[varName] {
				seen[varName] = true
				refs = append(refs, envVarRef{
					name:       varName,
					hasDefault: hasDefault,
				})
			}
		}
	}

	for _, svc := range e.Compose.Services {
		for key, val := range svc.Environment {
			if val == "" {
				// Pass-through variable (e.g. - DB_PASSWORD or DB_PASSWORD:)
				if !seen[key] {
					seen[key] = true
					refs = append(refs, envVarRef{
						name:       key,
						hasDefault: false,
					})
				}
			} else {
				parseStr(val)
			}
		}
		for _, port := range svc.Ports {
			parseStr(port)
		}
		for _, vol := range svc.Volumes {
			parseStr(vol.Source)
			parseStr(vol.Target)
		}
		if svc.Image != "" {
			parseStr(svc.Image)
		}
		if svc.ContainerName != "" {
			parseStr(svc.ContainerName)
		}
		for _, ep := range svc.Entrypoint {
			parseStr(ep)
		}
		for _, cmd := range svc.Command {
			parseStr(cmd)
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

		// Check system environment variable first (takes precedence)
		val, exists := os.LookupEnv(ref.name)
		if !exists {
			val, exists = e.Env[ref.name]
		}

		if !exists {
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
		} else if val == "" && !ref.hasDefault {
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
