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
	"strings"
	"time"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/logging"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Re-authenticate or get API key instructions",
	Long: `Re-authenticate (OAuth) or see how to update your API key.

This command does not configure auth from scratch. Use 'dx config' to set
endpoint and auth mode (OAuth or API key). Then:

  - If the profile uses OAuth: 'dx auth' runs the device flow so you can
    sign in again in the browser and refresh tokens.
  - If the profile uses API key: 'dx auth' prints instructions to generate
    a new key in the Deductive app and set it with 'dx config --api-key=<key>'.

Examples:
  dx auth                    # Re-auth default profile (OAuth) or show API key instructions
  dx auth --profile=staging  # Same for staging profile`,
	Run: runAuth,
}

func init() {
	authCmd.Hidden = true
	rootCmd.AddCommand(authCmd)
}

func runAuth(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	cfg, err := config.Load(profile)
	if err != nil {
		if profile == config.DefaultProfile {
			fmt.Fprintln(os.Stderr, "Error: No configuration found. Run 'dx config' first.")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Profile '%s' not found. Run 'dx config --profile=%s' first.\n", profile, profile)
		}
		os.Exit(1)
	}

	if cfg.Endpoint == "" {
		fmt.Fprintln(os.Stderr, "Error: No endpoint configured. Run 'dx config' first.")
		os.Exit(1)
	}

	client := api.NewClientWithEndpoint(cfg.Endpoint)

	switch cfg.AuthMethod {
	case "oauth":
		if err := authenticateWithOAuth(client, profile); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "apikey":
		fmt.Println("This profile uses API key authentication.")
		fmt.Println("API keys cannot be re-issued. To use a new key:")
		fmt.Println("  1) Generate one in the Deductive app (e.g. Settings → API Keys).")
		if profile == config.DefaultProfile {
			fmt.Println("  2) Run: dx config --api-key=<new_key>")
		} else {
			fmt.Printf("  2) Run: dx config --profile=%s --api-key=<new_key>\n", profile)
		}
	default:
		fmt.Fprintln(os.Stderr, "Error: No auth mode configured for this profile.")
		fmt.Fprintln(os.Stderr, "Run 'dx config' to set endpoint and auth mode (oauth or apikey).")
		os.Exit(1)
	}
}

func authenticateWithAPIKey(client *api.Client, profile string, apiKey string) error {
	if !strings.HasPrefix(apiKey, "dak_") {
		return fmt.Errorf("invalid API key format (must start with dak_)")
	}

	fmt.Println("Verifying API key...")

	resp, err := client.Verify(apiKey)
	if err != nil {
		return fmt.Errorf("authentication failed: %v", err)
	}

	auth := &config.Auth{
		Method: "apikey",
		APIKey: apiKey,
		TeamID: resp.TeamID,
	}

	if err := config.SaveAuth(auth, profile); err != nil {
		return fmt.Errorf("failed to save authentication: %v", err)
	}

	logging.Debug("Auth verified", "team_id", resp.TeamID, "method", "apikey")

	fmt.Println("✓ Authentication successful")
	fmt.Printf("✓ Team ID: %s\n", resp.TeamID)
	if profile != config.DefaultProfile {
		fmt.Printf("✓ Profile: %s\n", profile)
	}
	return nil
}

func authenticateWithOAuth(client *api.Client, profile string) error {
	fmt.Println("Requesting authorization...")
	resp, err := client.RequestDeviceCode()
	if err != nil {
		return fmt.Errorf("failed to request device code: %v", err)
	}

	fmt.Println()
	fmt.Printf("To authorize the CLI, visit:\n")
	fmt.Printf("  %s\n\n", resp.VerificationURIComplete)
	fmt.Printf("Or go to %s and enter code: %s\n\n", resp.VerificationURI, resp.UserCode)
	fmt.Println("Waiting for authorization...")

	token, err := client.PollForToken(resp.DeviceCode, resp.Interval)
	if err != nil {
		return fmt.Errorf("authorization failed: %v", err)
	}

	auth := &config.Auth{
		Method:       "oauth",
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(token.ExpiresIn) * time.Second),
		TeamID:       token.TeamID,
	}

	if err := config.SaveAuth(auth, profile); err != nil {
		return fmt.Errorf("failed to save authentication: %v", err)
	}

	logging.Debug("Auth verified", "team_id", token.TeamID, "method", "oauth")

	fmt.Println()
	fmt.Println("✓ Authentication successful")
	fmt.Printf("✓ Team ID: %s\n", token.TeamID)
	if profile != config.DefaultProfile {
		fmt.Printf("✓ Profile: %s\n", profile)
	}
	return nil
}
