package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/marcelo-lipienski/halo/snapshot"
)

func executeSnapshot(args []string) int {
	format = strings.ToLower(format)
	if format != "text" && format != "json" {
		fmt.Fprintf(stderr, "Invalid format: %s. Supported formats: text, json\n", format)
		return 1
	}

	snapFile := ""
	if len(args) > 0 {
		snapFile = args[0]
	} else {
		snapFile = filepath.Join(configDir, ".halo-snapshot.json")
	}

	envPath := envFile
	if envPath == "" {
		envPath = filepath.Join(configDir, ".env")
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
	}

	var missing []string
	if _, err := os.Stat(envPath); err != nil && os.IsNotExist(err) {
		missing = append(missing, filepath.Base(envPath))
	}
	for _, file := range filesToLoad {
		if _, err := os.Stat(file); err != nil && os.IsNotExist(err) {
			missing = append(missing, filepath.Base(file))
		}
	}

	if len(missing) > 0 {
		errStr := fmt.Sprintf("Missing configuration files: %s must exist.", strings.Join(missing, " and "))
		mitigationStr := "Ensure your .env file and all specified docker-compose files are present at their specified paths."
		renderSystemFailure(format, verbose, errStr, mitigationStr)
		return 1
	}

	snap, warnings, err := snapshot.CreateSnapshot(context.Background(), configDir, envFile, composeFiles)
	if err != nil {
		if !quiet {
			if format == "json" {
				renderSystemFailure("json", verbose, err.Error(), "Verify compose configuration files are present and valid.")
			} else {
				fmt.Fprintf(stderr, "Error: %v\n", err)
			}
		}
		return 1
	}

	if !quiet && format == "text" && len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Fprintf(stderr, "Warning: %s\n", w)
		}
	}

	if dryRun {
		if !quiet {
			if format == "json" {
				_ = json.NewEncoder(stdout).Encode(map[string]interface{}{
					"status":        "success",
					"dry_run":       true,
					"snapshot_file": snapFile,
				})
			} else {
				fmt.Fprintf(stdout, "[dry-run] Would capture state snapshot of local environment to %s\n", snapFile)
			}
		}
		return 0
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		if !quiet {
			if format == "json" {
				renderSystemFailure("json", verbose, err.Error(), "Failed to serialize snapshot data.")
			} else {
				fmt.Fprintf(stderr, "Error serializing snapshot: %v\n", err)
			}
		}
		return 1
	}

	if err := os.WriteFile(snapFile, data, 0644); err != nil {
		if !quiet {
			if format == "json" {
				renderSystemFailure("json", verbose, err.Error(), fmt.Sprintf("Verify write permission to file path: %s", snapFile))
			} else {
				fmt.Fprintf(stderr, "Error writing snapshot file: %v\n", err)
			}
		}
		return 1
	}

	if !quiet {
		if format == "json" {
			_ = json.NewEncoder(stdout).Encode(map[string]interface{}{
				"status":        "success",
				"snapshot_file": snapFile,
			})
		} else {
			fmt.Fprintf(stdout, "✓ Captured state snapshot of local environment to %s\n", snapFile)
		}
	}
	return 0
}

func executeDiff(args []string) int {
	format = strings.ToLower(format)
	if format != "text" && format != "json" {
		fmt.Fprintf(stderr, "Invalid format: %s. Supported formats: text, json\n", format)
		return 1
	}

	snapFile := ""
	if len(args) > 0 {
		snapFile = args[0]
	} else {
		snapFile = filepath.Join(configDir, ".halo-snapshot.json")
	}

	data, err := os.ReadFile(snapFile)
	if err != nil {
		errStr := fmt.Sprintf("Snapshot file not found: %s", snapFile)
		mitigationStr := "Run 'halo snapshot' first to capture a baseline state of the environment."
		renderSystemFailure(format, verbose, errStr, mitigationStr)
		return 1
	}

	var oldSnap snapshot.EnvironmentSnapshot
	if err := json.Unmarshal(data, &oldSnap); err != nil {
		errStr := fmt.Sprintf("Failed to parse snapshot file: %v", err)
		mitigationStr := "Re-run 'halo snapshot' to capture a new baseline state of the environment."
		renderSystemFailure(format, verbose, errStr, mitigationStr)
		return 1
	}

	newSnap, warnings, err := snapshot.CreateSnapshot(context.Background(), configDir, envFile, composeFiles)
	if err != nil {
		if !quiet {
			if format == "json" {
				renderSystemFailure("json", verbose, err.Error(), "Verify compose configuration files are present and valid.")
			} else {
				fmt.Fprintf(stderr, "Error: %v\n", err)
			}
		}
		return 1
	}

	if !quiet && format == "text" {
		if len(warnings) > 0 {
			for _, w := range warnings {
				fmt.Fprintf(stderr, "Warning: %s\n", w)
			}
		}
		if oldSnap.Project != newSnap.Project {
			fmt.Fprintf(stderr, "Warning: Comparing snapshot from project '%s' with current project '%s'\n", oldSnap.Project, newSnap.Project)
		}
	}

	diff := snapshot.Diff(&oldSnap, newSnap)
	hasDiffs := len(diff.Files) > 0 || len(diff.Variables) > 0 || len(diff.Ports) > 0 || len(diff.Containers) > 0

	if !quiet {
		if format == "json" {
			_ = json.NewEncoder(stdout).Encode(diff)
		} else {
			snapshot.RenderText(stdout, diff, oldSnap.CreatedAt)
		}
	}

	if hasDiffs {
		return 2
	}
	return 0
}
