package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/diagnostics"
	"github.com/marcelo-lipienski/halo/doctor"
	init_cmd "github.com/marcelo-lipienski/halo/init"
	"github.com/marcelo-lipienski/halo/output"
	"github.com/marcelo-lipienski/halo/snapshot"
	"github.com/moby/moby/client"
	"github.com/spf13/cobra"
)

type proxyWriter struct {
	mu sync.RWMutex
	w  io.Writer
}

func (p *proxyWriter) Write(b []byte) (n int, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.w.Write(b)
}

func (p *proxyWriter) Set(w io.Writer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.w = w
}

type safeExitFunc struct {
	mu sync.RWMutex
	fn func(code int)
}

func (s *safeExitFunc) Exit(code int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.fn(code)
}

func (s *safeExitFunc) Set(fn func(code int)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fn = fn
}

var (
	Version   = "dev"
	CommitSHA = "unknown"

	osExit = &safeExitFunc{fn: os.Exit}
	stdout = &proxyWriter{w: os.Stdout}
	stderr = &proxyWriter{w: os.Stderr}
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
				osExit.Exit(executeCheck())
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
				osExit.Exit(executeCheck())
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

	// init subcommand
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize or update .env from .env.example",
		Run: func(cmd *cobra.Command, args []string) {
			osExit.Exit(executeInit())
		},
	}

	// doctor subcommand
	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Inspect host system-level prerequisites",
		Run: func(cmd *cobra.Command, args []string) {
			osExit.Exit(executeDoctor())
		},
	}

	// snapshot subcommand
	snapshotCmd := &cobra.Command{
		Use:   "snapshot [file]",
		Short: "Capture a state snapshot of the local environment",
		Run: func(cmd *cobra.Command, args []string) {
			osExit.Exit(executeSnapshot(args))
		},
	}

	// diff subcommand
	diffCmd := &cobra.Command{
		Use:   "diff [file]",
		Short: "Compare current environment state against a snapshot",
		Run: func(cmd *cobra.Command, args []string) {
			osExit.Exit(executeDiff(args))
		},
	}

	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(snapshotCmd)
	rootCmd.AddCommand(diffCmd)

	return rootCmd
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		osExit.Exit(1)
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

	snap, warnings, err := snapshot.CreateSnapshot(configDir, envFile, composeFiles)
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

	newSnap, warnings, err := snapshot.CreateSnapshot(configDir, envFile, composeFiles)
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

