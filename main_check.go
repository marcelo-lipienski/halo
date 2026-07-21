package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/diagnostics"
	"github.com/marcelo-lipienski/halo/output"
	"github.com/moby/moby/client"
)

func executeCheck() int {
	format = strings.ToLower(format)
	if format != "text" && format != "json" {
		fmt.Fprintf(stderr, "Invalid format: %s. Supported formats: text, json\n", format)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	envPath := envFile
	if envPath == "" {
		envPath = filepath.Join(configDir, ".env")
	}

	// Determine compose files to load.
	var filesToLoad []string
	if len(composeFiles) > 0 {
		filesToLoad = composeFiles
	} else {
		// Auto-detect compose file. See ADR-0003.
		composePathYml := filepath.Join(configDir, "docker-compose.yml")
		composePathYaml := filepath.Join(configDir, "docker-compose.yaml")
		composePath := composePathYaml
		if _, err := os.Stat(composePathYml); err == nil {
			composePath = composePathYml
		}
		filesToLoad = append(filesToLoad, composePath)

		// Check for override compose file. See ADR-0003.
		overridePathYml := filepath.Join(configDir, "docker-compose.override.yml")
		overridePathYaml := filepath.Join(configDir, "docker-compose.override.yaml")
		if _, err := os.Stat(overridePathYml); err == nil {
			filesToLoad = append(filesToLoad, overridePathYml)
		} else if _, err := os.Stat(overridePathYaml); err == nil {
			filesToLoad = append(filesToLoad, overridePathYaml)
		}
	}

	// Stat config files.
	var missing []string
	var accessErrors []string

	if _, err := os.Stat(envPath); err != nil {
		if os.IsNotExist(err) {
			missing = append(missing, filepath.Base(envPath))
		} else {
			accessErrors = append(accessErrors, fmt.Sprintf("%s (%v)", filepath.Base(envPath), err))
		}
	}

	var parsedConfigs []*config.ComposeConfig
	for _, file := range filesToLoad {
		if _, err := os.Stat(file); err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, filepath.Base(file))
			} else {
				accessErrors = append(accessErrors, fmt.Sprintf("%s (%v)", filepath.Base(file), err))
			}
		}
	}

	if len(missing) > 0 {
		errStr := fmt.Sprintf("Missing configuration files: %s must exist.", strings.Join(missing, " and "))
		mitigationStr := "Ensure your .env file and all specified docker-compose files are present at their specified paths."
		renderSystemFailure(format, verbose, errStr, mitigationStr)
		return 1
	}

	if len(accessErrors) > 0 {
		errStr := fmt.Sprintf("Unable to access configuration files: %s.", strings.Join(accessErrors, " and "))
		mitigationStr := "Check file permissions and user privileges for the reported configuration files."
		renderSystemFailure(format, verbose, errStr, mitigationStr)
		return 1
	}

	var parseErrs []error
	env, envErr := config.ParseEnv(envPath)
	if envErr != nil {
		parseErrs = append(parseErrs, fmt.Errorf("failed to parse .env file: %w", envErr))
	}

	for _, file := range filesToLoad {
		comp, err := config.ParseCompose(file)
		if err != nil {
			parseErrs = append(parseErrs, fmt.Errorf("failed to parse docker-compose file (%s): %w", filepath.Base(file), err))
		} else {
			parsedConfigs = append(parsedConfigs, comp)
		}
	}

	if len(parseErrs) > 0 {
		joinedErr := errors.Join(parseErrs...)
		var mitigation string
		if len(parseErrs) > 1 {
			mitigation = "Check the syntax and format of the reported configuration files."
		} else {
			mitigation = "Verify file syntax is valid."
		}
		renderSystemFailure(format, verbose, joinedErr.Error(), mitigation)
		return 1
	}

	// Merge compose configs. See ADR-0003.
	mergedComp := config.MergeComposeConfigs(parsedConfigs...)

	var dockerCli client.APIClient
	var dockerErr error
	dockerCli, dockerErr = client.New(client.FromEnv)
	if dockerErr == nil {
		pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
		_, dockerErr = dockerCli.Ping(pingCtx, client.PingOptions{})
		pingCancel()
	}

	if dockerErr != nil {
		dockerCli = nil
	} else {
		defer func() { _ = dockerCli.Close() }()
	}

	engineConfigDir := filepath.Dir(filesToLoad[0])
	engine := diagnostics.NewEngine(engineConfigDir, filesToLoad[0], env, mergedComp, dockerCli)
	engine.EnvPath = envPath
	engine.AutoFix = fix
	engine.DryRun = dryRun
	engine.Interactive = interactive
	report := engine.Run(ctx)

	if !quiet {
		if format == "json" {
			_ = output.RenderJSON(stdout, report)
		} else {
			output.RenderText(stdout, report, verbose)
		}
	}

	if report.Status == output.StatusEnvironmentBroken {
		return 2
	}
	return 0
}

func getWatchFiles() []string {
	var files []string
	envPath := envFile
	if envPath == "" {
		envPath = filepath.Join(configDir, ".env")
	}
	if _, err := os.Stat(envPath); err == nil {
		files = append(files, envPath)
	}

	var filesToLoad []string
	if len(composeFiles) > 0 {
		filesToLoad = composeFiles
	} else {
		composePathYml := filepath.Join(configDir, "docker-compose.yml")
		composePathYaml := filepath.Join(configDir, "docker-compose.yaml")
		composePath := composePathYaml
		if _, err := os.Stat(composePathYml); err == nil {
			composePath = composePathYml
		}
		filesToLoad = append(filesToLoad, composePath)

		overridePathYml := filepath.Join(configDir, "docker-compose.override.yml")
		overridePathYaml := filepath.Join(configDir, "docker-compose.override.yaml")
		if _, err := os.Stat(overridePathYml); err == nil {
			filesToLoad = append(filesToLoad, overridePathYml)
		} else if _, err := os.Stat(overridePathYaml); err == nil {
			filesToLoad = append(filesToLoad, overridePathYaml)
		}
	}
	for _, f := range filesToLoad {
		if _, err := os.Stat(f); err == nil {
			files = append(files, f)
		}
	}

	examplePath := filepath.Join(configDir, ".env.example")
	if _, err := os.Stat(examplePath); err == nil {
		files = append(files, examplePath)
	}

	envMap, _ := config.ParseEnv(envPath)

	// Detect service env_files dynamically. See ADR-0003.
	for _, file := range filesToLoad {
		if comp, err := config.ParseCompose(file); err == nil {
			for _, svc := range comp.Services {
				for _, ef := range svc.EnvFiles {
					resolvedPath := diagnostics.ResolveShellExpr(ef.File, envMap)
					path := resolvedPath
					if !filepath.IsAbs(path) {
						baseDir := ef.BaseDir
						if baseDir == "" {
							baseDir = filepath.Dir(file)
						}
						path = filepath.Join(baseDir, path)
					}
					path = filepath.Clean(path)
					if _, err := os.Stat(path); err == nil {
						files = append(files, path)
					}
				}
			}
		}
	}

	// De-duplicate file paths.
	uniqueFiles := make(map[string]bool)
	var deduped []string
	for _, f := range files {
		abs, err := filepath.Abs(f)
		if err == nil {
			if !uniqueFiles[abs] {
				uniqueFiles[abs] = true
				deduped = append(deduped, abs)
			}
		} else {
			if !uniqueFiles[f] {
				uniqueFiles[f] = true
				deduped = append(deduped, f)
			}
		}
	}

	return deduped
}

func runWatch(ctx context.Context) {
	files := getWatchFiles()

	_ = executeCheck()

	lastMods := make(map[string]time.Time)
	for _, f := range files {
		if stat, err := os.Stat(f); err == nil {
			lastMods[f] = stat.ModTime()
		} else {
			lastMods[f] = time.Time{}
		}
	}

	fmt.Fprintln(stderr, "\nWatching for configuration changes... (Press Ctrl+C to stop)")

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			changed := false
			files = getWatchFiles()

			// 1. Detect deleted files.
			currentFiles := make(map[string]bool)
			for _, f := range files {
				currentFiles[f] = true
			}
			for f := range lastMods {
				if !currentFiles[f] {
					delete(lastMods, f)
					changed = true
				}
			}

			// 2. Detect modified or new files.
			for _, f := range files {
				stat, err := os.Stat(f)
				var modTime time.Time
				if err == nil {
					modTime = stat.ModTime()
				}
				if !modTime.Equal(lastMods[f]) {
					lastMods[f] = modTime
					changed = true
				}
			}

			if changed {
				fmt.Fprint(stderr, "\033[H\033[2J")
				fmt.Fprintln(stderr, "Change detected! Re-running diagnostics...")
				_ = executeCheck()
				fmt.Fprintln(stderr, "\nWatching for configuration changes... (Press Ctrl+C to stop)")
			}
		}
	}
}
