// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
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

	// API keys are single-team scoped, so no team picker needed
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

	if err := promptTeamSelection(profile); err != nil {
		logging.Debug("Team selection skipped", "error", err)
		if token.TeamName != "" {
			fmt.Printf("✓ Team: %s (%s)\n", token.TeamName, token.TeamID)
		} else if token.TeamID != "" {
			fmt.Printf("✓ Team ID: %s\n", token.TeamID)
		}
	}

	if profile != config.DefaultProfile {
		fmt.Printf("✓ Profile: %s\n", profile)
	}
	return nil
}

// promptTeamSelection lists the user's teams after auth. If there are
// multiple teams, it prompts the user to pick one. For a single team
// it just confirms the selection. Requires an interactive terminal.
func promptTeamSelection(profile string) error {
	cfg, err := config.Load(profile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	authedClient := api.NewClient(cfg)
	teamsResp, err := authedClient.ListTeams()
	if err != nil {
		return fmt.Errorf("failed to list teams: %w", err)
	}

	if len(teamsResp.Teams) <= 1 {
		if len(teamsResp.Teams) == 1 {
			t := teamsResp.Teams[0]
			cfg.TeamID = t.ID
			cfg.TeamName = t.Name
			_ = config.Save(cfg, profile)
			fmt.Printf("✓ Team: %s\n", color.Info(t.Name))
		}
		return nil
	}

	if !isInteractiveTerminal() {
		fmt.Printf("✓ Team: %s (use %s to change)\n",
			color.Info(cfg.TeamName), color.Command("dx team switch"))
		return nil
	}

	fmt.Println()
	fmt.Println("  You belong to multiple teams:")
	defaultIdx := 1
	for i, t := range teamsResp.Teams {
		fmt.Printf("    %d. %s\n", i+1, t.Name)
		if t.ID == cfg.TeamID {
			defaultIdx = i + 1
		}
	}
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("  Select a team [%d]: ", defaultIdx)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	selectedIdx := defaultIdx
	if input != "" {
		n, err := strconv.Atoi(input)
		if err != nil || n < 1 || n > len(teamsResp.Teams) {
			fmt.Printf("  Invalid selection, using default (%d)\n", defaultIdx)
			n = defaultIdx
		}
		selectedIdx = n
	}

	selected := teamsResp.Teams[selectedIdx-1]
	cfg.TeamID = selected.ID
	cfg.TeamName = selected.Name
	if err := config.Save(cfg, profile); err != nil {
		return fmt.Errorf("failed to save team selection: %w", err)
	}

	fmt.Printf("✓ Team: %s\n", color.Info(selected.Name))
	return nil
}
