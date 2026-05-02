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
	Short: "View current configuration",
	Long: `Display the current endpoint and authentication status.

Examples:
  dx setup config`,
	Run: runConfigShow,
}

func init() {
	setupCmd.AddCommand(configCmd)
}

func runConfigShow(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	cfg, err := config.Load(profile)
	if err != nil {
		fmt.Println("Not configured yet.")
		fmt.Printf("Run %s to get started.\n", color.Command("dx setup init"))
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
