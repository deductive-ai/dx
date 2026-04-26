// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/session"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View or change settings",
	Long: `Display the current configuration.

Use subcommands to change settings:
  dx config setup    Re-run the setup wizard (endpoint + auth)
  dx config reset    Reset all configuration

Examples:
  dx config
  dx config setup`,
	Run: runConfigShow,
}

var configSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Re-run the setup wizard",
	Long: `Re-run the interactive setup wizard to change endpoint and authentication.

This clears any existing configuration and walks you through setup again.

Examples:
  dx config setup`,
	Run: runConfigSetup,
}

var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset configuration",
	Long: `Delete all configuration and session data.

On the next run of "dx ask", the setup wizard will run again.`,
	Run: runConfigReset,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetupCmd)
	configCmd.AddCommand(configResetCmd)
}

func runConfigShow(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	cfg, err := config.Load(profile)
	if err != nil {
		fmt.Println("Not configured yet.")
		fmt.Printf("Run %s to get started.\n", color.Command("dx ask"))
		return
	}

	fmt.Printf("  Endpoint:  %s\n", cfg.Endpoint)

	authStatus := "not authenticated"
	if cfg.AuthMethod == "apikey" && cfg.APIKey != "" {
		authStatus = "authenticated (api-key)"
	} else if cfg.AuthMethod == "oauth" && cfg.OAuthAccessToken != "" {
		if time.Now().Before(cfg.OAuthExpiresAt) {
			remaining := time.Until(cfg.OAuthExpiresAt).Truncate(time.Minute)
			authStatus = fmt.Sprintf("authenticated (oauth, expires in %s)", remaining)
		} else {
			authStatus = "expired (run: dx auth)"
		}
	}
	fmt.Printf("  Auth:      %s\n", authStatus)
}

func runConfigSetup(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	if config.ProfileExists(profile) {
		_ = session.DeleteForProfile(profile)
		_ = config.DeleteProfile(profile)
	}

	runBootstrap(profile)
	fmt.Printf("\n%s Configuration saved.\n", color.Success("✓"))
}

func runConfigReset(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	deleted := false

	if config.ProfileExists(profile) {
		_ = session.DeleteForProfile(profile)
		if err := config.DeleteProfile(profile); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		deleted = true
	}

	if profile != config.DefaultProfile && config.ProfileExists(config.DefaultProfile) {
		_ = session.DeleteForProfile(config.DefaultProfile)
		_ = config.DeleteProfile(config.DefaultProfile)
		deleted = true
	}

	if !deleted {
		fmt.Println("Nothing to reset — no configuration found.")
		return
	}

	_ = config.WriteActiveProfile(config.DefaultProfile)

	fmt.Printf("%s Configuration reset.\n", color.Success("✓"))
	fmt.Printf("Run %s to set up again.\n", color.Command("dx ask"))
}
