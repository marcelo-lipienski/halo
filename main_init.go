package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	init_cmd "github.com/marcelo-lipienski/halo/init"
)

func executeInit() int {
	format = strings.ToLower(format)
	if format != "text" && format != "json" {
		fmt.Fprintf(stderr, "Invalid format: %s. Supported formats: text, json\n", format)
		return 1
	}

	envPath := envFile
	if envPath == "" {
		envPath = filepath.Join(configDir, ".env")
	}
	examplePath := filepath.Join(filepath.Dir(envPath), ".env.example")

	res, err := init_cmd.MergeEnvFiles(examplePath, envPath, dryRun)
	if err != nil {
		if !quiet {
			if format == "json" {
				renderSystemFailure("json", verbose, err.Error(), "Ensure .env.example exists and paths are valid.")
			} else {
				fmt.Fprintf(stderr, "Error: %v\n", err)
			}
		}
		return 1
	}

	placeholders := 0
	for _, entry := range res.Added {
		if entry.IsPlaceholder {
			placeholders++
		}
	}

	if dryRun {
		if !quiet {
			if format == "json" {
				_ = json.NewEncoder(stdout).Encode(map[string]interface{}{
					"status":          "success",
					"dry_run":         true,
					"created":         res.AlreadyPresent == 0 && len(res.Added) > 0,
					"already_present": res.AlreadyPresent,
					"added_count":     len(res.Added),
					"added":           res.Added,
					"placeholders":    placeholders,
				})
			} else {
				fmt.Fprintf(stdout, "[dry-run] Would merge %d missing keys from %s into %s\n", len(res.Added), examplePath, envPath)
			}
		}
		return 0
	}

	if quiet {
		return 0
	}

	if format == "json" {
		_ = json.NewEncoder(stdout).Encode(map[string]interface{}{
			"status":          "success",
			"created":         res.AlreadyPresent == 0 && len(res.Added) > 0,
			"already_present": res.AlreadyPresent,
			"added_count":     len(res.Added),
			"added":           res.Added,
			"placeholders":    placeholders,
		})
		return 0
	}

	if res.AlreadyPresent == 0 && len(res.Added) > 0 {
		fmt.Fprintf(stdout, "✓ .env does not exist — created from .env.example\n\n")
	}

	if len(res.Added) == 0 {
		fmt.Fprintf(stdout, "✓ .env is up to date with .env.example — no keys to add.\n")
		return 0
	} else if res.AlreadyPresent > 0 {
		fmt.Fprintf(stdout, "✓ .env exists — merging missing keys from .env.example\n\n")
	}

	fmt.Fprintf(stdout, "Added %d keys:\n", len(res.Added))
	for _, entry := range res.Added {
		if entry.IsPlaceholder {
			fmt.Fprintf(stdout, "  %s=%-30s ← needs value\n", entry.Key, entry.Value)
		} else {
			fmt.Fprintf(stdout, "  %s=%-30s ✓ has default\n", entry.Key, entry.Value)
		}
	}

	if placeholders > 0 {
		if placeholders == 1 {
			fmt.Fprintf(stdout, "\n1 key needs a value before running. Open .env in your editor.\n")
		} else {
			fmt.Fprintf(stdout, "\n%d keys need values before running. Open .env in your editor.\n", placeholders)
		}
	}

	return 0
}
