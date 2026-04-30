// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/session"
	"github.com/spf13/cobra"
)

var teamCmd = &cobra.Command{
	Use:   "team",
	Short: "Manage team context",
	Long: `List available teams and switch the active team.

Switching teams changes which team context is used for all
subsequent API requests. Your authentication stays the same.

Examples:
  dx team                     # List teams (same as dx team list)
  dx team list                # List all teams you belong to
  dx team switch "Acme Corp"  # Switch to a different team
  dx team switch abc123       # Switch by team ID`,
	Run: runTeamList,
}

var teamListCmd = &cobra.Command{
	Use:   "list",
	Short: "List teams you belong to",
	Run:   runTeamList,
}

var teamSwitchCmd = &cobra.Command{
	Use:   "switch <name-or-id>",
	Short: "Switch to a different team",
	Args:  cobra.ExactArgs(1),
	Run:   runTeamSwitch,
}

func init() {
	rootCmd.AddCommand(teamCmd)
	teamCmd.AddCommand(teamListCmd)
	teamCmd.AddCommand(teamSwitchCmd)
}

func runTeamList(cmd *cobra.Command, args []string) {
	profile := GetProfile()
	cfg, err := config.Load(profile)
	if err != nil || !cfg.IsAuthenticated() {
		_, _ = fmt.Fprintf(os.Stderr, "%s Not authenticated. Run %s first.\n",
			color.Error("✗"), color.Command("dx auth"))
		os.Exit(1)
	}

	client := api.NewClient(cfg)
	teamsResp, err := client.ListTeams()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s Failed to list teams: %v\n", color.Error("✗"), err)
		os.Exit(1)
	}

	if len(teamsResp.Teams) == 0 {
		fmt.Println("You don't belong to any teams.")
		return
	}

	fmt.Println("Your teams:")
	for _, t := range teamsResp.Teams {
		marker := "  "
		if t.ID == cfg.TeamID {
			marker = "* "
		}
		fmt.Printf("  %s%s (%s)\n", marker, t.Name, t.ID)
	}

	if cfg.TeamID != "" {
		fmt.Printf("\n  * = active team\n")
	}
}

func runTeamSwitch(cmd *cobra.Command, args []string) {
	target := args[0]
	profile := GetProfile()

	cfg, err := config.Load(profile)
	if err != nil || !cfg.IsAuthenticated() {
		_, _ = fmt.Fprintf(os.Stderr, "%s Not authenticated. Run %s first.\n",
			color.Error("✗"), color.Command("dx auth"))
		os.Exit(1)
	}

	if cfg.AuthMethod == "apikey" {
		_, _ = fmt.Fprintf(os.Stderr, "%s Team switching requires OAuth authentication.\n", color.Error("✗"))
		_, _ = fmt.Fprintf(os.Stderr, "API keys are scoped to a single team. Run %s to use OAuth instead.\n",
			color.Command("dx auth"))
		os.Exit(1)
	}

	client := api.NewClient(cfg)
	teamsResp, err := client.ListTeams()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s Failed to list teams: %v\n", color.Error("✗"), err)
		os.Exit(1)
	}

	var matched *api.Team
	targetLower := strings.ToLower(target)
	for i, t := range teamsResp.Teams {
		if t.ID == target || strings.ToLower(t.Name) == targetLower {
			matched = &teamsResp.Teams[i]
			break
		}
	}

	if matched == nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s No team matching %q found.\n", color.Error("✗"), target)
		_, _ = fmt.Fprintln(os.Stderr, "Run 'dx team list' to see available teams.")
		os.Exit(1)
	}

	if matched.ID == cfg.TeamID {
		fmt.Printf("Already on team: %s (%s)\n", matched.Name, matched.ID)
		return
	}

	cfg.TeamID = matched.ID
	cfg.TeamName = matched.Name
	if err := config.Save(cfg, profile); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s Failed to save config: %v\n", color.Error("✗"), err)
		os.Exit(1)
	}

	_ = session.Clear(profile)

	fmt.Printf("%s Switched to team: %s (%s)\n", color.Success("✓"), matched.Name, matched.ID)
	fmt.Println("Session cleared — next request will use the new team context.")
}
