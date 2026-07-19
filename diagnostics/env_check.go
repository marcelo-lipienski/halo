package diagnostics

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

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
		matches := envVarRegex.FindAllStringSubmatch(s, -1)
		for _, match := range matches {
			varName := ""
			hasDefault := false
			if len(match) > 1 && match[1] != "" {
				varName = match[1]
				fullMatch := match[0]
				if strings.HasPrefix(fullMatch, "${") && strings.Contains(fullMatch, "-") {
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

	refs := e.extractReferencedEnvVars()

	variablesCheckPassed := true
	mismatchedTypesPassed := true

	for _, ref := range refs {
		select {
		case <-ctx.Done():
			return results
		default:
		}

		val, exists := e.Env[ref.name]
		if !exists {
			// Check system environment variable fallback
			sysVal, sysExists := os.LookupEnv(ref.name)
			if sysExists {
				val = sysVal
				exists = true
			}
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
				Status:     output.CheckFailed,
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
