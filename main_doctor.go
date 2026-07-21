package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/doctor"
	"github.com/marcelo-lipienski/halo/output"
)

func executeDoctor() int {
	format = strings.ToLower(format)
	if format != "text" && format != "json" {
		fmt.Fprintf(stderr, "Invalid format: %s. Supported formats: text, json\n", format)
		return 1
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

	var parsedConfigs []*config.ComposeConfig
	for _, file := range filesToLoad {
		if _, err := os.Stat(file); err == nil {
			if parsed, err := config.ParseCompose(file); err == nil {
				parsedConfigs = append(parsedConfigs, parsed)
			}
		}
	}
	var comp *config.ComposeConfig
	if len(parsedConfigs) > 0 {
		comp = config.MergeComposeConfigs(parsedConfigs...)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	report := doctor.RunDoctor(ctx, configDir, comp)

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
