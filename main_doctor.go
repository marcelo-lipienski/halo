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

	var comp *config.ComposeConfig
	composePathYml := filepath.Join(configDir, "docker-compose.yml")
	composePathYaml := filepath.Join(configDir, "docker-compose.yaml")
	composePath := composePathYaml
	if _, err := os.Stat(composePathYml); err == nil {
		composePath = composePathYml
	}
	if _, err := os.Stat(composePath); err == nil {
		parsedComp, err := config.ParseCompose(composePath)
		if err == nil {
			comp = parsedComp
		}
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
