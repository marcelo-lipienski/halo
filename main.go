package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/diagnostics"
	"github.com/marcelo-lipienski/halo/output"
	"github.com/moby/moby/client"
	"github.com/spf13/cobra"
)

// Version information injected during build via ldflags
var (
	Version   = "dev"
	CommitSHA = "unknown"
)

func printVersion() {
	fmt.Printf("halo version %s (%s)\n", Version, CommitSHA)
	fmt.Printf("Go runtime:  %s (%s/%s)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

var (
	configDir    string
	envFile      string
	composeFiles []string
	format       string
	verbose      bool
	fix          bool
	quiet        bool
	dryRun       bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "halo",
		Short: "halo diagnoses local development environments",
		Long:  `halo is a lightweight CLI tool to diagnose and validate local development environments by analyzing environment configurations and active Docker state.`,
		Run: func(cmd *cobra.Command, args []string) {
			// By default, running the root command executes the diagnostics checks (equivalent to "check" subcommand)
			runCheck()
		},
	}

	// Define persistent flags
	rootCmd.PersistentFlags().StringVarP(&configDir, "config-dir", "c", ".", "Path to the directory containing local configuration files")
	rootCmd.PersistentFlags().StringVarP(&envFile, "env-file", "e", "", "Explicit path to the .env file")
	rootCmd.PersistentFlags().StringArrayVar(&composeFiles, "compose-file", []string{}, "Explicit path(s) to the docker-compose.yml file (can specify multiple times)")
	rootCmd.PersistentFlags().StringVarP(&format, "format", "f", "text", "Output format for results (text|json)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enables debug logging")
	rootCmd.PersistentFlags().BoolVar(&fix, "fix", false, "Automatically attempt to mitigate file permission and missing directory issues")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppresses all standard output")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Preview changes when running with --fix without modifying the filesystem")

	// check subcommand
	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Run the diagnostic suite",
		Run: func(cmd *cobra.Command, args []string) {
			runCheck()
		},
	}

	// version subcommand
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			printVersion()
		},
	}

	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runCheck() {
	format = strings.ToLower(format)
	if format != "text" && format != "json" {
		fmt.Fprintf(os.Stderr, "Invalid format: %s. Supported formats: text, json\n", format)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	envPath := envFile
	if envPath == "" {
		envPath = filepath.Join(configDir, ".env")
	}

	// Determine compose files to load
	var filesToLoad []string
	if len(composeFiles) > 0 {
		filesToLoad = composeFiles
	} else {
		// Automatic detection: prefer docker-compose.yml, fall back to docker-compose.yaml
		// (matches Docker Compose's own precedence behaviour)
		composePathYml := filepath.Join(configDir, "docker-compose.yml")
		composePathYaml := filepath.Join(configDir, "docker-compose.yaml")
		composePath := composePathYaml
		if _, err := os.Stat(composePathYml); err == nil {
			composePath = composePathYml
		}
		filesToLoad = append(filesToLoad, composePath)

		// Check for automatic override file: docker-compose.override.yml or docker-compose.override.yaml
		overridePathYml := filepath.Join(configDir, "docker-compose.override.yml")
		overridePathYaml := filepath.Join(configDir, "docker-compose.override.yaml")
		if _, err := os.Stat(overridePathYml); err == nil {
			filesToLoad = append(filesToLoad, overridePathYml)
		} else if _, err := os.Stat(overridePathYaml); err == nil {
			filesToLoad = append(filesToLoad, overridePathYaml)
		}
	}

	// Stat check for all configuration files
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
		exitWithSystemFailure(format, verbose, errStr, mitigationStr)
	}

	if len(accessErrors) > 0 {
		errStr := fmt.Sprintf("Unable to access configuration files: %s.", strings.Join(accessErrors, " and "))
		mitigationStr := "Check file permissions and user privileges for the reported configuration files."
		exitWithSystemFailure(format, verbose, errStr, mitigationStr)
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
		exitWithSystemFailure(format, verbose, joinedErr.Error(), mitigation)
	}

	// Merge all parsed configs according to docker-compose overrides rules
	mergedComp := config.MergeComposeConfigs(parsedConfigs...)

	dockerCli, err := client.New(client.FromEnv)
	if err != nil {
		exitWithSystemFailure(format, verbose, fmt.Sprintf("Failed to create Docker client: %v", err), "Verify your Docker environment variables are set correctly.")
	}
	defer func() { _ = dockerCli.Close() }()

	pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
	defer pingCancel()
	_, err = dockerCli.Ping(pingCtx, client.PingOptions{})
	if err != nil {
		exitWithSystemFailure(format, verbose, fmt.Sprintf("Docker daemon is unreachable: %v", err), "Ensure Docker daemon/service is running and socket is accessible.")
	}

	engineConfigDir := filepath.Dir(filesToLoad[0])
	engine := diagnostics.NewEngine(engineConfigDir, filesToLoad[0], env, mergedComp, dockerCli)
	engine.AutoFix = fix
	engine.DryRun = dryRun
	report := engine.Run(ctx)

	if !quiet {
		if format == "json" {
			_ = output.RenderJSON(os.Stdout, report)
		} else {
			output.RenderText(os.Stdout, report, verbose)
		}
	}

	if report.Status == output.StatusEnvironmentBroken {
		os.Exit(2)
	}
	os.Exit(0)
}

func exitWithSystemFailure(format string, verbose bool, errStr, mitigationStr string) {
	report := &output.DiagnosticsReport{
		Status:     output.StatusSystemFailure,
		DurationMs: 0,
		Checks: []output.CheckResult{
			{
				Group:      "System Discovery",
				Name:       "Environment Check",
				Status:     output.CheckFailed,
				Error:      errStr,
				Mitigation: mitigationStr,
			},
		},
	}
	if quiet {
		fmt.Fprintf(os.Stderr, "Error: %s\nMitigation: %s\n", errStr, mitigationStr)
	} else {
		if format == "json" {
			_ = output.RenderJSON(os.Stdout, report)
		} else {
			output.RenderText(os.Stdout, report, verbose)
		}
	}
	os.Exit(1)
}
