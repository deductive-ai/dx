/*
 * Copyright (c) 2023, Deductive AI, Inc. All rights reserved.
 *
 * This software is the confidential and proprietary information of
 * Deductive AI, Inc. You shall not disclose such confidential
 * information and shall use it only in accordance with the terms of
 * the license agreement you entered into with Deductive AI, Inc.
 */

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
var versionFlag bool
var noColorFlag bool
var debugFlag bool

var rootCmd = &cobra.Command{
	Use:   "dx",
	Short: "CLI for Deductive AI — ask questions about your infrastructure",
	Long: `DX is the command-line interface for Deductive AI.
Ask questions about your infrastructure, investigate issues,
and get AI-powered insights — all from the terminal.

Get started:
  dx init                                       # One-command setup
  dx ask "what's using the most memory?"         # Ask a question
  ps aux | dx ask "which process needs attention?" # Pipe data in

Profiles:
  Use --profile to manage multiple Deductive instances.
  dx init --profile=staging
  dx ask "test query" --profile=staging`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
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
		cmd.Help()
	},
}

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Print the CLI version",
	Long:    `Print the version, git commit, and build date of the DX CLI.`,
	GroupID: "advanced",
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

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Command groups for organized help output
var (
	groupGettingStarted = cobra.Group{ID: "getting-started", Title: "Getting Started:"}
	groupUsage          = cobra.Group{ID: "usage", Title: "Usage:"}
	groupAdvanced       = cobra.Group{ID: "advanced", Title: "Advanced:"}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&profileFlag, "profile", config.DefaultProfile,
		"Configuration profile to use (default: \"default\")")
	rootCmd.PersistentFlags().BoolVar(&noColorFlag, "no-color", false,
		"Disable colored output (also respects NO_COLOR and DX_NO_COLOR env vars)")
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false,
		"Enable debug logging (also respects DX_DEBUG env var)")
	rootCmd.Flags().BoolVarP(&versionFlag, "version", "V", false, "Print version information")

	rootCmd.AddGroup(&groupGettingStarted, &groupUsage, &groupAdvanced)
	rootCmd.AddCommand(versionCmd)

	rootCmd.CompletionOptions.DisableDefaultCmd = true
}

// HideCommand marks a command as hidden (still functional, not shown in help).
func HideCommand(cmd *cobra.Command) {
	cmd.Hidden = true
}

// GetProfile returns the current profile from the global flag
func GetProfile() string {
	if profileFlag == "" {
		return config.DefaultProfile
	}
	return profileFlag
}
