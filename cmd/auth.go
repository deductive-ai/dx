// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/logging"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with Deductive",
	Long: `Re-authenticate with your Deductive instance.

For OAuth, this runs the browser-based device flow.
For API key auth, this prints instructions to update your key.

Examples:
  dx auth`,
	Run: runAuth,
}

func init() {
	setupCmd.AddCommand(authCmd)
}

func runAuth(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	cfg, err := config.Load(profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s No configuration found.\n", color.Error("✗"))
		fmt.Fprintf(os.Stderr, "Run %s to set up the CLI first.\n", color.Command("dx setup init"))
		os.Exit(1)
	}

	if cfg.Endpoint == "" {
		fmt.Fprintf(os.Stderr, "%s No endpoint configured.\n", color.Error("✗"))
		fmt.Fprintf(os.Stderr, "Run %s to set up the CLI first.\n", color.Command("dx setup init"))
		os.Exit(1)
	}

	client := api.NewClientWithEndpoint(cfg.Endpoint)

	switch cfg.AuthMethod {
	case "apikey":
		fmt.Println("This profile uses API key authentication.")
		fmt.Println("To update your key:")
		fmt.Println("  1) Generate a new key in Settings > API Keys")
		fmt.Printf("  2) Run: %s\n", color.Command("dx setup init"))
	default:
		if err := authenticateWithOAuth(client, profile); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
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
		Method:   "apikey",
		APIKey:   apiKey,
		TeamID:   resp.TeamID,
		TeamName: resp.TeamName,
	}

	if err := config.SaveAuth(auth, profile); err != nil {
		return fmt.Errorf("failed to save authentication: %v", err)
	}

	logging.Debug("Auth verified", "team_id", resp.TeamID, "method", "apikey")

	fmt.Println("✓ Authentication successful")
	if resp.TeamName != "" {
		fmt.Printf("✓ Team: %s (%s)\n", resp.TeamName, resp.TeamID)
	} else {
		fmt.Printf("✓ Team ID: %s\n", resp.TeamID)
	}
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
	openBrowserOrPrint(resp.VerificationURIComplete)
	fmt.Printf("\n  Or go to %s and enter code: %s\n\n", resp.VerificationURI, resp.UserCode)
	fmt.Println("  Waiting for authorization...")

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
		TeamName:     token.TeamName,
	}

	if err := config.SaveAuth(auth, profile); err != nil {
		return fmt.Errorf("failed to save authentication: %v", err)
	}

	logging.Debug("Auth verified", "team_id", token.TeamID, "method", "oauth")

	fmt.Println()
	fmt.Println("✓ Authentication successful")
	if token.TeamName != "" {
		fmt.Printf("✓ Team: %s (%s)\n", token.TeamName, token.TeamID)
	} else {
		fmt.Printf("✓ Team ID: %s\n", token.TeamID)
	}
	if profile != config.DefaultProfile {
		fmt.Printf("✓ Profile: %s\n", profile)
	}
	return nil
}
