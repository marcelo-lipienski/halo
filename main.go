package main

import (
	"context"
	"flag"
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

func main() {
	var configDir string
	var envFile string
	var composeFile string
	var format string
	var verbose bool

	fs := flag.NewFlagSet("halo", flag.ExitOnError)
	fs.StringVar(&configDir, "config-dir", ".", "Path to the directory containing local configuration files")
	fs.StringVar(&configDir, "c", ".", "Path to the directory containing local configuration files (shorthand)")
	fs.StringVar(&envFile, "env-file", "", "Explicit path to the .env file")
	fs.StringVar(&envFile, "e", "", "Explicit path to the .env file (shorthand)")
	fs.StringVar(&composeFile, "compose-file", "", "Explicit path to the docker-compose.yml file")
	fs.StringVar(&format, "format", "text", "Output format for results (text|json)")
	fs.StringVar(&format, "f", "text", "Output format for results (text|json) (shorthand)")
	fs.BoolVar(&verbose, "verbose", false, "Enables debug logging")
	fs.BoolVar(&verbose, "v", false, "Enables debug logging (shorthand)")

	args := os.Args[1:]

	command := ""
	var flagArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "check" || arg == "version" {
			if command == "" {
				command = arg
				continue
			}
		}
		// If it's a flag that takes an argument, consume the flag and its value
		if (arg == "-c" || arg == "--config-dir" || arg == "-e" || arg == "--env-file" || arg == "--compose-file" || arg == "-f" || arg == "--format") && i+1 < len(args) {
			flagArgs = append(flagArgs, arg, args[i+1])
			i++
			continue
		}
		flagArgs = append(flagArgs, arg)
	}

	if err := fs.Parse(flagArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	if command == "" {
		command = "check"
	}

	switch command {
	case "version":
		printVersion()
		os.Exit(0)
	case "check":
		runCheck(configDir, envFile, composeFile, format, verbose)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		fmt.Fprintln(os.Stderr, "Usage: halo [command] [flags]")
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  check     Run diagnostic suite (default)")
		fmt.Fprintln(os.Stderr, "  version   Show version information")
		os.Exit(1)
	}
}

func runCheck(configDir, envFile, composeFile, format string, verbose bool) {
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

	composePath := composeFile
	if composePath == "" {
		composePathYml := filepath.Join(configDir, "docker-compose.yml")
		composePathYaml := filepath.Join(configDir, "docker-compose.yaml")
		composePath = composePathYml
		if _, err := os.Stat(composePathYaml); err == nil {
			composePath = composePathYaml
		}
	}

	_, composeStatErr := os.Stat(composePath)
	_, envStatErr := os.Stat(envPath)

	if os.IsNotExist(composeStatErr) || os.IsNotExist(envStatErr) {
		var missing []string
		if os.IsNotExist(envStatErr) {
			missing = append(missing, filepath.Base(envPath))
		}
		if os.IsNotExist(composeStatErr) {
			missing = append(missing, filepath.Base(composePath))
		}
		errStr := fmt.Sprintf("Missing configuration files: %s must exist.", strings.Join(missing, " and "))
		mitigationStr := fmt.Sprintf("Ensure both your .env file and docker-compose.yml file are present at their specified paths (%s and %s).", envPath, composePath)
		exitWithSystemFailure(format, errStr, mitigationStr)
	}

	env, err := config.ParseEnv(envPath)
	if err != nil {
		exitWithSystemFailure(format, fmt.Sprintf("Failed to parse .env file: %v", err), "Check .env format for syntax errors.")
	}

	comp, err := config.ParseCompose(composePath)
	if err != nil {
		exitWithSystemFailure(format, fmt.Sprintf("Failed to parse docker-compose file: %v", err), "Verify docker-compose.yml syntax is valid YAML.")
	}

	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		exitWithSystemFailure(format, fmt.Sprintf("Failed to create Docker client: %v", err), "Verify your Docker environment variables are set correctly.")
	}
	defer dockerCli.Close()

	pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
	defer pingCancel()
	_, err = dockerCli.Ping(pingCtx, client.PingOptions{})
	if err != nil {
		exitWithSystemFailure(format, fmt.Sprintf("Docker daemon is unreachable: %v", err), "Ensure Docker daemon/service is running and socket is accessible.")
	}

	engineConfigDir := filepath.Dir(composePath)
	engine := diagnostics.NewEngine(engineConfigDir, composePath, env, comp, dockerCli)
	report := engine.Run(ctx)

	if format == "json" {
		output.RenderJSON(os.Stdout, report)
	} else {
		output.RenderText(os.Stdout, report, verbose)
	}

	if report.Status == output.StatusEnvironmentBroken {
		os.Exit(2)
	}
	os.Exit(0)
}

func exitWithSystemFailure(format, errStr, mitigationStr string) {
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
	if format == "json" {
		output.RenderJSON(os.Stdout, report)
	} else {
		output.RenderText(os.Stdout, report, true)
	}
	os.Exit(1)
}
