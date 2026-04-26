// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/logging"
	"github.com/deductive-ai/dx/internal/version"
	"github.com/spf13/cobra"
)

// Version info - set via ldflags at build time
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

var profileFlag string
var profileExplicit bool
var versionFlag bool
var noColorFlag bool
var debugFlag bool

var rootCmd = &cobra.Command{
	Use:   "dx",
	Short: "CLI for Deductive AI — ask questions about your infrastructure",
	Long: `DX is the command-line interface for Deductive AI.
Ask questions about your infrastructure, pipe in data, get answers.

  dx ask "what's using the most memory?"
  ps aux | dx ask "which process needs attention?"`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		profileExplicit = cmd.Flags().Changed("profile")
		if noColorFlag {
			color.SetEnabled(false)
		}
		if debugFlag || os.Getenv("DX_DEBUG") != "" {
			logging.Init(true)
		}
		version.Check(Version, GetProfile())
	},
	Run: func(cmd *cobra.Command, args []string) {
		if versionFlag {
			printVersion()
			return
		}
		defaultToAsk(cmd, args)
	},
}

var versionCmd = &cobra.Command{
	Use:    "version",
	Short:  "Print the CLI version",
	Long:   `Print the version, git commit, and build date of the DX CLI.`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

func printVersion() {
	fmt.Printf("dx version %s\n", Version)
	fmt.Printf("  Git commit: %s\n", GitCommit)
	fmt.Printf("  Built:      %s\n", BuildDate)
	fmt.Printf("  Go version: %s\n", runtime.Version())
	fmt.Printf("  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

// defaultToAsk delegates to the "ask" subcommand when no subcommand is given.
// Defined here (not inline) to break the init-cycle between rootCmd and runAsk.
func defaultToAsk(cmd *cobra.Command, args []string) {
	askCmd, _, _ := cmd.Find([]string{"ask"})
	if askCmd != nil && askCmd.Run != nil {
		askCmd.Run(askCmd, args)
		return
	}
	_ = cmd.Help()
}

// Execute runs the root command
func Execute() {
	unhideAdvanced()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&profileFlag, "profile", config.DefaultProfile,
		"Configuration profile to use")
	_ = rootCmd.PersistentFlags().MarkHidden("profile")
	rootCmd.PersistentFlags().BoolVar(&noColorFlag, "no-color", false,
		"Disable colored output (also respects NO_COLOR and DX_NO_COLOR env vars)")
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false,
		"Enable debug logging (also respects DX_DEBUG env var)")
	rootCmd.Flags().BoolVarP(&versionFlag, "version", "V", false, "Print version information")

	rootCmd.AddCommand(versionCmd)

	rootCmd.CompletionOptions.DisableDefaultCmd = true
}

// unhideAdvanced reveals internal commands and flags when DX_ADVANCED is set.
func unhideAdvanced() {
	if os.Getenv("DX_ADVANCED") == "" {
		return
	}
	profileCmd.Hidden = false
	statusCmd.Hidden = false
	_ = rootCmd.PersistentFlags().SetAnnotation("profile", cobra.BashCompOneRequiredFlag, []string{""})
	rootCmd.PersistentFlags().Lookup("profile").Hidden = false
}

// GetProfile returns the active profile using the precedence chain:
// --profile flag > DX_PROFILE env var > ~/.dx/active_profile > "default"
func GetProfile() string {
	if profileExplicit {
		return profileFlag
	}
	if env := os.Getenv("DX_PROFILE"); env != "" {
		return env
	}
	if active, err := config.ReadActiveProfile(); err == nil && active != "" {
		return active
	}
	return config.DefaultProfile
}
