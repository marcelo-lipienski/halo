package main

import (
	"context"
	"fmt"
	"os"
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

	filesToLoad := config.FindComposeFiles(configDir, composeFiles)

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
