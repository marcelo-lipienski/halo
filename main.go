package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
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

	osExit           = os.Exit
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
)

func printVersion() {
	version := Version
	commit := CommitSHA

	if info, ok := debug.ReadBuildInfo(); ok {
		if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
			// If it's a Go pseudo-version (e.g. v0.2.3-0.20260719154649-fb27bb3b90df),
			// extract the 12-character commit hash suffix.
			parts := strings.Split(version, "-")
			if len(parts) >= 3 && commit == "unknown" {
				lastPart := parts[len(parts)-1]
				if len(lastPart) == 12 {
					isHex := true
					for i := 0; i < len(lastPart); i++ {
						c := lastPart[i]
						if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
							isHex = false
							break
						}
					}
					if isHex {
						commit = lastPart[:7]
					}
				}
			}
		}
		if commit == "unknown" {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					commit = setting.Value
					if len(commit) > 7 {
						commit = commit[:7]
					}
					break
				}
			}
		}
	}

	if commit != "unknown" {
		fmt.Fprintf(stdout, "halo version %s (%s)\n", version, commit)
	} else {
		fmt.Fprintf(stdout, "halo version %s\n", version)
	}
	fmt.Fprintf(stdout, "Go runtime:  %s (%s/%s)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
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
	interactive  bool
	watch        bool
)

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "halo",
		Short: "halo diagnoses local development environments",
		Long:  `halo is a lightweight CLI tool to diagnose and validate local development environments by analyzing environment configurations and active Docker state.`,
		Run: func(cmd *cobra.Command, args []string) {
			if watch {
				runWatch(context.Background())
			} else {
				osExit(executeCheck())
			}
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
	rootCmd.PersistentFlags().BoolVarP(&interactive, "interactive", "i", false, "Confirm mitigation steps interactively before applying them")
	rootCmd.PersistentFlags().BoolVarP(&watch, "watch", "w", false, "Watch configuration files for changes and automatically re-run checks")

	// check subcommand
	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Run the diagnostic suite",
		Run: func(cmd *cobra.Command, args []string) {
			if watch {
				runWatch(context.Background())
			} else {
				osExit(executeCheck())
			}
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

	return rootCmd
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		osExit(1)
	}
}

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

	// Merge all parsed configs according to docker-compose overrides rules
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

func renderSystemFailure(format string, verbose bool, errStr, mitigationStr string) {
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
		fmt.Fprintf(stderr, "Error: %s\nMitigation: %s\n", errStr, mitigationStr)
	} else {
		if format == "json" {
			_ = output.RenderJSON(stdout, report)
		} else {
			output.RenderText(stdout, report, verbose)
		}
	}
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

	// Dynamic detection of service-level env_files
	for _, file := range filesToLoad {
		if comp, err := config.ParseCompose(file); err == nil {
			for _, svc := range comp.Services {
				for _, ef := range svc.EnvFiles {
					resolvedPath := ef.File
					if strings.Contains(resolvedPath, "$") {
						resolvedPath = os.ExpandEnv(resolvedPath)
					}
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

	// De-duplicate files using absolute paths
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

	// Initial run
	_ = executeCheck()

	lastMods := make(map[string]time.Time)
	for _, f := range files {
		if stat, err := os.Stat(f); err == nil {
			lastMods[f] = stat.ModTime()
		} else {
			lastMods[f] = time.Time{}
		}
	}

	fmt.Fprintln(stdout, "\nWatching for configuration changes... (Press Ctrl+C to stop)")

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			changed := false
			files = getWatchFiles()

			// 1. Detect deleted files from lastMods
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

			// 2. Detect modified or new files
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
				fmt.Fprint(stdout, "\033[H\033[2J")
				fmt.Fprintln(stdout, "Change detected! Re-running diagnostics...")
				_ = executeCheck()
				fmt.Fprintln(stdout, "\nWatching for configuration changes... (Press Ctrl+C to stop)")
			}
		}
	}
}
