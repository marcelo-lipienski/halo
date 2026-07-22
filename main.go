package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/marcelo-lipienski/halo/output"
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
			// Extract commit suffix from Go pseudo-version.
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

	rootCmd.PersistentFlags().StringVarP(&configDir, "config-dir", "c", ".", "Path to the directory containing local configuration files")
	rootCmd.PersistentFlags().StringVarP(&envFile, "env-file", "e", "", "Explicit path to the .env file")
	rootCmd.PersistentFlags().StringArrayVar(&composeFiles, "compose-file", []string{}, "Explicit path(s) to the docker-compose.yml file (can specify multiple times)")
	rootCmd.PersistentFlags().StringVarP(&format, "format", "f", "text", "Output format for results (text|json)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enables debug logging")
	rootCmd.PersistentFlags().BoolVar(&fix, "fix", false, "Automatically attempt to mitigate file permission and missing directory issues")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppresses all standard output")
	rootCmd.PersistentFlags().BoolVarP(&dryRun, "dry-run", "d", false, "Preview changes when running with --fix or fix command without modifying the filesystem")
	rootCmd.PersistentFlags().BoolVarP(&interactive, "interactive", "i", false, "Confirm mitigation steps interactively before applying them")
	rootCmd.PersistentFlags().BoolVarP(&watch, "watch", "w", false, "Watch configuration files for changes and automatically re-run checks")

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

	fixCmd := &cobra.Command{
		Use:   "fix",
		Short: "Automatically mitigate configuration, file permission, and missing directory/file issues",
		Run: func(cmd *cobra.Command, args []string) {
			if watch {
				runWatch(context.Background())
			} else {
				osExit.Exit(executeFix())
			}
		},
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			printVersion()
		},
	}

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize or update .env from .env.example",
		Run: func(cmd *cobra.Command, args []string) {
			osExit.Exit(executeInit())
		},
	}

	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Inspect host system-level prerequisites",
		Run: func(cmd *cobra.Command, args []string) {
			osExit.Exit(executeDoctor())
		},
	}

	snapshotCmd := &cobra.Command{
		Use:   "snapshot [file]",
		Short: "Capture a state snapshot of the local environment",
		Run: func(cmd *cobra.Command, args []string) {
			osExit.Exit(executeSnapshot(args))
		},
	}

	diffCmd := &cobra.Command{
		Use:   "diff [file]",
		Short: "Compare current environment state against a snapshot",
		Run: func(cmd *cobra.Command, args []string) {
			osExit.Exit(executeDiff(args))
		},
	}

	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(fixCmd)
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
