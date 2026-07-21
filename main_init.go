package main

import (
	"fmt"
	"path/filepath"

	init_cmd "github.com/marcelo-lipienski/halo/init"
)

func executeInit() int {
	envPath := envFile
	if envPath == "" {
		envPath = filepath.Join(configDir, ".env")
	}
	examplePath := filepath.Join(filepath.Dir(envPath), ".env.example")

	res, err := init_cmd.MergeEnvFiles(examplePath, envPath, dryRun)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	if dryRun {
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
	placeholders := 0
	for _, entry := range res.Added {
		if entry.IsPlaceholder {
			fmt.Fprintf(stdout, "  %s=%-30s ← needs value\n", entry.Key, entry.Value)
			placeholders++
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
