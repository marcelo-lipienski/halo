package diagnostics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/output"
)

type svcEnvRef struct {
	ref         shellEnvRef
	serviceName string
}

func (e *Engine) extractReferencedEnvVars() []svcEnvRef {
	var refs []svcEnvRef

	// Sort service names.
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

		// Read and parse env file.
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

		// System env takes precedence over .env.
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
			// Skip if fallback exists. See ADR-0003.
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
				// Required non-empty variable check. See ADR-0003.
				variablesCheckPassed = false
				results = append(results, output.CheckResult{
					Group:      "Environmental Alignment",
					Name:       fmt.Sprintf("Variable %s is required but empty", ref.ref.name),
					Status:     output.CheckFailed,
					Error:      fmt.Sprintf("Environment variable %s uses the :? operator in docker-compose.yml and must be set to a non-empty value", ref.ref.name),
					Mitigation: fmt.Sprintf("Set a non-empty value for %s in your .env file or host environment", ref.ref.name),
				})
			} else if !ref.ref.hasDefault {
				// Warn on empty variable without default. See ADR-0003.
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

	// .env.example schema validation check
	schemaCheckPassed := true
	hasSchemaFile := false

	examplePath := filepath.Join(e.ConfigDir, ".env.example")
	if _, err := os.Stat(examplePath); err != nil {
		// Fallback to compose directory.
		examplePath = filepath.Join(filepath.Dir(e.ComposePath), ".env.example")
	}

	if _, err := os.Stat(examplePath); err == nil {
		hasSchemaFile = true
		if exampleEnv, parseErr := config.ParseEnv(examplePath); parseErr == nil {
			// Sort keys.
			var keys []string
			for k := range exampleEnv {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			isPlaceholder := func(v string) bool {
				vl := strings.ToLower(v)
				return strings.Contains(vl, "change-me") ||
					strings.Contains(vl, "changeme") ||
					strings.Contains(vl, "todo") ||
					strings.Contains(vl, "insert") ||
					strings.Contains(vl, "placeholder") ||
					strings.Contains(vl, "your-") ||
					strings.Contains(vl, "your_") ||
					strings.Contains(vl, "replace-me") ||
					strings.Contains(vl, "replaceme")
			}

			for _, key := range keys {
				select {
				case <-ctx.Done():
					return results
				default:
				}

				// Check if key is defined.
				var val string
				var exists bool
				if v, ok := os.LookupEnv(key); ok {
					val = v
					exists = true
				} else if v, ok := e.Env[key]; ok {
					val = v
					exists = true
				} else {
					// Check in service env_file.
					for _, svc := range e.Compose.Services {
						svcEnv := e.loadServiceEnvFiles(svc)
						if v, ok := svcEnv[key]; ok {
							val = v
							exists = true
							break
						}
					}
				}

				if !exists {
					variablesCheckPassed = false
					schemaCheckPassed = false
					results = append(results, output.CheckResult{
						Group:      "Environmental Alignment",
						Name:       fmt.Sprintf("Variable %s missing from .env", key),
						Status:     output.CheckFailed,
						Error:      fmt.Sprintf("Environment variable %s is defined in .env.example but missing from .env or host environment", key),
						Mitigation: fmt.Sprintf("Add %s=your_value to your .env file", key),
					})
				} else if isPlaceholder(val) {
					results = append(results, output.CheckResult{
						Group:      "Environmental Alignment",
						Name:       fmt.Sprintf("Variable %s has placeholder value", key),
						Status:     output.CheckWarning,
						Error:      fmt.Sprintf("Environment variable %s has placeholder value '%s'", key, val),
						Mitigation: fmt.Sprintf("Set a non-placeholder value for %s in your .env file", key),
					})
				}
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
	if hasSchemaFile && schemaCheckPassed {
		results = append(results, output.CheckResult{
			Group:  "Environmental Alignment",
			Name:   "Schema Alignment Check",
			Status: output.CheckPassed,
		})
	}

	envPath := e.EnvPath
	if envPath == "" {
		envPath = filepath.Join(e.ConfigDir, ".env")
	}
	// Only run drift check if .env file exists on disk.
	if _, statErr := os.Stat(envPath); statErr == nil {
		examplePath2 := filepath.Join(filepath.Dir(envPath), ".env.example")
		driftResults, _ := CheckEnvExampleDrift(envPath, examplePath2)
		results = append(results, driftResults...)
	}

	imageResults := e.CheckImageTags()
	results = append(results, imageResults...)

	return results
}

// CheckEnvExampleDrift compares .env against .env.example.
func CheckEnvExampleDrift(envPath, examplePath string) ([]output.CheckResult, error) {
	var results []output.CheckResult

	if _, err := os.Stat(examplePath); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	exampleMap, err := config.ParseEnv(examplePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse example file: %w", err)
	}

	envMap := make(map[string]string)
	if _, err := os.Stat(envPath); err == nil {
		parsedMap, err := config.ParseEnv(envPath)
		if err == nil {
			envMap = parsedMap
		}
	}

	var missingKeys []string
	for k := range exampleMap {
		if _, exists := envMap[k]; !exists {
			missingKeys = append(missingKeys, k)
		}
	}
	sort.Strings(missingKeys)

	var undeclaredKeys []string
	for k := range envMap {
		if _, exists := exampleMap[k]; !exists {
			undeclaredKeys = append(undeclaredKeys, k)
		}
	}
	sort.Strings(undeclaredKeys)

	if len(missingKeys) > 0 {
		results = append(results, output.CheckResult{
			Group:      "Environmental Alignment",
			Name:       ".env.example Drift",
			Status:     output.CheckFailed,
			Error:      fmt.Sprintf("%d keys in .env.example are missing from .env: %s", len(missingKeys), strings.Join(missingKeys, ", ")),
			Mitigation: "Run 'halo init' to automatically merge missing keys from .env.example",
		})
	} else {
		results = append(results, output.CheckResult{
			Group:  "Environmental Alignment",
			Name:   ".env.example Drift",
			Status: output.CheckPassed,
		})
	}

	if len(undeclaredKeys) > 0 {
		results = append(results, output.CheckResult{
			Group:      "Environmental Alignment",
			Name:       "Undeclared Keys",
			Status:     output.CheckWarning,
			Error:      fmt.Sprintf("%d keys in .env are not declared in .env.example: %s", len(undeclaredKeys), strings.Join(undeclaredKeys, ", ")),
			Mitigation: "Add these keys to .env.example or remove them from .env",
		})
	} else {
		results = append(results, output.CheckResult{
			Group:  "Environmental Alignment",
			Name:   "Undeclared Keys",
			Status: output.CheckPassed,
		})
	}

	return results, nil
}
