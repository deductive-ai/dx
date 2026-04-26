// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/session"
	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage sessions",
	Long: `Manage Deductive AI sessions.

Sessions store conversation context and presigned URLs for file uploads.
Use subcommands to list, clear, or manage sessions.

Examples:
  dx session list      # List all stored sessions
  dx session clear     # Clear all stored sessions`,
	Example: `  dx session list
  dx session clear`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stored sessions",
	Long: `List all sessions stored locally.

Shows session ID, creation time, profile, and available upload slots.
The current active session is marked with an asterisk (*).

Examples:
  dx session list`,
	Run: runSessionList,
}

var sessionDeleteCmd = &cobra.Command{
	Use:   "delete <session-id>",
	Short: "Delete a stored session",
	Long: `Delete a specific session by ID. This removes the local session file.
Server-side data is not affected.

Supports short ID prefix matching — you don't need to type the full UUID.

Examples:
  dx session delete abc123-def456-...
  dx session delete abc123`,
	Args: cobra.ExactArgs(1),
	Run:  runSessionDelete,
}

var sessionClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all stored sessions",
	Long: `Clear all locally stored session data.

This removes all sessions for the current profile and clears the
current session pointer. Other profiles' sessions are not affected.
Server-side session data is not affected.

Examples:
  dx session clear`,
	Run: runSessionClear,
}

func init() {
	sessionCmd.Hidden = true
	rootCmd.AddCommand(sessionCmd)

	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionClearCmd)
	sessionCmd.AddCommand(sessionDeleteCmd)
}

func runSessionList(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	sessions, err := session.ListForProfile(profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing sessions: %v\n", err)
		os.Exit(1)
	}

	if len(sessions) == 0 {
		fmt.Printf("No sessions found for profile '%s'.\n", profile)
		fmt.Println("Use 'dx ask' to start a new session.")
		return
	}

	// Get current session ID
	currentID, _ := session.GetCurrentSessionID(profile)

	fmt.Printf("Found %d session(s):\n\n", len(sessions))
	for _, s := range sessions {
		marker := " "
		if s.SessionID == currentID {
			marker = "*"
		}
		age := formatAge(s.CreatedAt)
		available := len(s.PresignedURLs) - s.URLsUsed
		
		fmt.Printf("%s %s\n", marker, color.SessionID(s.SessionID))
		fmt.Printf("    Created:  %s (%s)\n", s.CreatedAt.Format("2006-01-02 15:04:05"), age)
		fmt.Printf("    Profile:  %s\n", color.Info(s.Profile))
		fmt.Printf("    Uploads:  %d/%d available\n", available, len(s.PresignedURLs))
		if s.URL != "" {
			fmt.Printf("    URL:      %s\n", color.URL(s.URL))
		}
		fmt.Println()
	}

	if currentID != "" {
		fmt.Println("* = current active session")
	}
}

func runSessionClear(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	count, err := session.ClearAll(profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error clearing sessions: %v\n", err)
		os.Exit(1)
	}

	if count == 0 {
		fmt.Println("No sessions to clear.")
		return
	}

	fmt.Printf("%s Cleared %d session(s)\n", color.Success("✓"), count)
	fmt.Println("Use 'dx ask' to start a new session.")
}

func runSessionDelete(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	sessionID, err := session.ResolveShortID(args[0], profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := session.Delete(sessionID, profile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s Deleted session %s\n", color.Success("✓"), color.SessionID(sessionID))
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}


