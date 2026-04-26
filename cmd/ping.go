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

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/spf13/cobra"
)

var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Test connection to Deductive AI",
	Long: `Test connectivity and authentication with Deductive AI.

This command verifies that:
  - The endpoint is reachable
  - The authentication is valid
  - The team is accessible

Examples:
  dx ping
  dx ping --profile=staging`,
	Run: runPing,
}

func init() {
	pingCmd.Hidden = true
	rootCmd.AddCommand(pingCmd)
}

func runPing(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	// Check if config exists
	cfg, err := config.Load(profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, color.Error("✗ No configuration found"))
		if profile == config.DefaultProfile {
			fmt.Fprintf(os.Stderr, "  Run '%s' to configure the CLI\n", color.Command("dx config"))
		} else {
			fmt.Fprintf(os.Stderr, "  Run '%s' to configure this profile\n", color.Command("dx config --profile="+profile))
		}
		os.Exit(1)
	}

	if profile != config.DefaultProfile {
		fmt.Printf("Profile: %s\n", color.Info(profile))
	}
	fmt.Printf("Endpoint: %s\n", color.URL(cfg.Endpoint))

	// Test endpoint connectivity
	fmt.Print("Testing connectivity... ")
	if err := api.Ping(cfg.Endpoint); err != nil {
		fmt.Println(color.Error("✗"))
		fmt.Fprintf(os.Stderr, "  %s\n", color.Error("Error: "+err.Error()))
		os.Exit(1)
	}
	fmt.Println(color.Success("✓"))

	// Check authentication
	if cfg.AuthMethod == "" {
		fmt.Printf("Authentication: %s\n", color.Warning("Not configured"))
		if profile == config.DefaultProfile {
			fmt.Printf("  Run '%s' to authenticate\n", color.Command("dx auth"))
		} else {
			fmt.Printf("  Run '%s' to authenticate\n", color.Command("dx auth --profile="+profile))
		}
		os.Exit(0)
	}

	fmt.Printf("Auth method: %s\n", color.Info(cfg.AuthMethod))

	if !cfg.IsAuthenticated() {
		fmt.Printf("Authentication: %s\n", color.Error("✗ Invalid or expired"))
		if cfg.AuthMethod == "apikey" {
			if profile == config.DefaultProfile {
				fmt.Printf("  Run '%s' to set a new API key\n", color.Command("dx config --api-key=<key>"))
			} else {
				fmt.Printf("  Run '%s' to set a new API key\n", color.Command("dx config --profile="+profile+" --api-key=<key>"))
			}
		} else {
			if profile == config.DefaultProfile {
				fmt.Printf("  Run '%s' to re-authenticate\n", color.Command("dx auth"))
			} else {
				fmt.Printf("  Run '%s' to re-authenticate\n", color.Command("dx auth --profile="+profile))
			}
		}
		os.Exit(1)
	}

	// Verify authentication with server
	fmt.Print("Verifying authentication... ")
	client := api.NewClient(cfg)
	resp, err := client.Verify(cfg.GetAuthToken())
	if err != nil {
		fmt.Println(color.Error("✗"))
		fmt.Fprintf(os.Stderr, "  %s\n", color.Error("Error: "+err.Error()))
		os.Exit(1)
	}
	fmt.Println(color.Success("✓"))

	fmt.Printf("Team ID: %s\n", color.Info(resp.TeamID))

	// Show role status
	if cfg.Role != "" {
		fmt.Printf("Role: %s\n", color.Success("configured"))
	}

	// Show hook status
	if len(cfg.Hooks) > 0 {
		fmt.Printf("Hooks: %s configured\n", color.Info(fmt.Sprintf("%d", len(cfg.Hooks))))
	}

	fmt.Println()
	fmt.Println(color.Success("All systems operational."))
}
