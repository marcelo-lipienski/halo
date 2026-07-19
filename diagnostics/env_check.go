package diagnostics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/marcelo-lipienski/halo/output"
)

var envVarRegex = regexp.MustCompile(`\$(?:\{([a-zA-Z0-9_]+)(?::?-([^}]*))?\}|([a-zA-Z0-9_]+))`)

type envVarRef struct {
	name       string
	hasDefault bool
}

func extractReferencedEnvVars(composePath string) ([]envVarRef, error) {
	content, err := os.ReadFile(composePath)
	if err != nil {
		return nil, err
	}
	matches := envVarRegex.FindAllSubmatch(content, -1)
	var refs []envVarRef
	seen := make(map[string]bool)
	for _, match := range matches {
		varName := ""
		hasDefault := false
		if len(match) > 1 && string(match[1]) != "" {
			varName = string(match[1])
			if len(match) > 2 && string(match[2]) != "" {
				hasDefault = true
			}
		} else if len(match) > 3 && string(match[3]) != "" {
			varName = string(match[3])
		}

		if varName != "" && !seen[varName] {
			seen[varName] = true
			refs = append(refs, envVarRef{
				name:       varName,
				hasDefault: hasDefault,
			})
		}
	}
	return refs, nil
}

func (e *Engine) checkEnvironmentalAlignment(ctx context.Context) []output.CheckResult {
	var results []output.CheckResult

	refs, err := extractReferencedEnvVars(e.ComposePath)
	if err != nil {
		results = append(results, output.CheckResult{
			Group:      "Environmental Alignment",
			Name:       "Variables Check",
			Status:     output.CheckFailed,
			Error:      fmt.Sprintf("Failed to read docker-compose file to scan env vars: %v", err),
			Mitigation: fmt.Sprintf("Ensure %s exists and is readable.", filepath.Base(e.ComposePath)),
		})
		return results
	}

	variablesCheckPassed := true
	mismatchedTypesPassed := true

	for _, ref := range refs {
		val, exists := e.Env[ref.name]
		if !exists {
			variablesCheckPassed = false
			results = append(results, output.CheckResult{
				Group:      "Environmental Alignment",
				Name:       fmt.Sprintf("Variable %s missing", ref.name),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("Environment variable %s is referenced in docker-compose.yml but not defined in .env", ref.name),
				Mitigation: fmt.Sprintf("Add %s=your_value to your .env file", ref.name),
			})
		} else if val == "" && !ref.hasDefault {
			mismatchedTypesPassed = false
			results = append(results, output.CheckResult{
				Group:      "Environmental Alignment",
				Name:       fmt.Sprintf("Variable %s is empty", ref.name),
				Status:     output.CheckFailed,
				Error:      fmt.Sprintf("Environment variable %s is defined but empty in .env and has no default fallback in docker-compose.yml", ref.name),
				Mitigation: fmt.Sprintf("Set a non-empty value for %s in your .env file", ref.name),
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
