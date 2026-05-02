// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/session"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure endpoint, auth, and skills",
	Long: `Manage CLI configuration, authentication, and agent skills.

Subcommands:
  dx setup init     Run the setup wizard (endpoint + auth)
  dx setup auth     Re-authenticate
  dx setup skill    Manage agent skill files
  dx setup reset    Reset all configuration
  dx setup config   View current configuration

Examples:
  dx setup init
  dx setup auth
  dx setup skill install
  dx setup reset`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var setupInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Run the setup wizard (endpoint + auth)",
	Long: `Run the interactive setup wizard to configure endpoint and authentication.

If a configuration already exists, it will be replaced.

Examples:
  dx setup init`,
	Run: runSetupInit,
}

var setupResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset configuration",
	Long: `Delete all configuration and session data.

Run "dx setup init" to set up again.`,
	Run: runConfigReset,
}

func init() {
	rootCmd.AddCommand(setupCmd)
	setupCmd.AddCommand(setupInitCmd)
	setupCmd.AddCommand(setupResetCmd)
}

func runSetupInit(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	if config.ProfileExists(profile) {
		_ = session.DeleteForProfile(profile)
		_ = config.DeleteProfile(profile)
	}

	runBootstrap(profile)
	fmt.Printf("\n%s Configuration saved.\n", color.Success("✓"))
}

