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
	"context"
	"fmt"
	"os"
	"time"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/logging"
	"github.com/deductive-ai/dx/internal/session"
	"github.com/deductive-ai/dx/internal/telemetry"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"
)

var (
	sessionIDFlag string
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
		cmd.Help()
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

var createSessionCmd = &cobra.Command{
	Use:   "create-session",
	Short: "Create a new chat session",
	Long: `Create a new chat session with Deductive AI.

This creates a new session and provides presigned URLs for file uploads.
The session ID is saved locally for subsequent commands (ask, upload).

If you have configured a role (via 'dx set-role'), it will be sent
automatically as the first message when you use 'dx ask'.

Examples:
  dx create-session
  dx create-session --profile=staging`,
	Run: runCreateSession,
}

var resumeSessionCmd = &cobra.Command{
	Use:   "resume-session",
	Short: "Resume an existing chat session",
	Long: `Resume an existing chat session by its ID.

This retrieves the session and provides fresh presigned URLs for file uploads.
Use this to continue a session from another terminal or after the CLI restarts.

Examples:
  dx resume-session --session-id=abc123
  dx resume-session -r abc123
  dx resume-session -r abc123 --profile=staging`,
	Run: runResumeSession,
}

func init() {
	sessionCmd.Hidden = true
	rootCmd.AddCommand(sessionCmd)

	// Hidden: create-session and resume-session are power-user commands
	// dx ask auto-creates sessions; dx ask --session covers resume
	createSessionCmd.Hidden = true
	resumeSessionCmd.Hidden = true
	rootCmd.AddCommand(createSessionCmd)
	rootCmd.AddCommand(resumeSessionCmd)

	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionClearCmd)
	sessionCmd.AddCommand(sessionDeleteCmd)

	resumeSessionCmd.Flags().StringVarP(&sessionIDFlag, "session-id", "r", "", "Session ID to resume (required)")
	resumeSessionCmd.MarkFlagRequired("session-id")
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
		fmt.Println("Use 'dx create-session' to create a new session.")
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
	fmt.Println("Use 'dx create-session' to start a new session.")
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

func runCreateSession(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	_, span := telemetry.StartSpan(context.Background(), "dx.create_session",
		attribute.String("profile", profile),
	)
	defer span.End()

	cfg, err := config.Load(profile)
	if err != nil {
		if profile == config.DefaultProfile {
			fmt.Fprintln(os.Stderr, "Error: No configuration found. Run 'dx ask' to get started.")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Profile '%s' not found. Run 'dx ask --profile=%s' to set it up.\n", profile, profile)
		}
		os.Exit(1)
	}

	cfg, err = EnsureAuth(cfg, profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	client := api.NewClient(cfg)

	fmt.Println("Creating session...")

	resp, err := client.CreateSession(&api.SessionRequest{
		Mode: "ask",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating session: %v\n", err)
		os.Exit(1)
	}

	// Save session state
	state := &session.State{
		SessionID:      resp.SessionID,
		Profile:        profile,
		URL:            resp.URL,
		PresignedURLs:  resp.PresignedURLs,
		CreatedAt:      time.Now(),
		URLsUsed:       0,
		RoleSent:       false,
		LastHookOutput: "",
	}

	if err := session.Save(state); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not save session state: %v\n", err)
	}

	// Set as current session
	if err := session.SetCurrentSessionID(state.SessionID, profile); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not set current session: %v\n", err)
	}

	logging.Debug("Session created", "session_id", resp.SessionID, "profile", profile)

	fmt.Printf("%s Session created: %s\n", color.Success("✓"), color.SessionID(resp.SessionID))
	fmt.Printf("  View in browser: %s\n", color.URL(resp.URL))
	fmt.Printf("  Resume command:  %s\n", color.Command("dx resume-session -r "+resp.SessionID))
	fmt.Printf("  Upload slots:    %s available\n", color.Info(fmt.Sprintf("%d", len(resp.PresignedURLs))))
	if profile != config.DefaultProfile {
		fmt.Printf("  Profile:         %s\n", color.Info(profile))
	}
	if cfg.Role != "" {
		fmt.Printf("  Role:            %s\n", color.Muted("will be sent with first message"))
	}
	fmt.Println()
	fmt.Printf("You can now use '%s' to upload files or '%s' to ask questions.\n",
		color.Command("dx upload"), color.Command("dx ask"))
}

func runResumeSession(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	_, span := telemetry.StartSpan(context.Background(), "dx.resume_session",
		attribute.String("profile", profile),
	)
	defer span.End()

	cfg, err := config.Load(profile)
	if err != nil {
		if profile == config.DefaultProfile {
			fmt.Fprintln(os.Stderr, "Error: No configuration found. Run 'dx ask' to get started.")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Profile '%s' not found. Run 'dx ask --profile=%s' to set it up.\n", profile, profile)
		}
		os.Exit(1)
	}

	cfg, err = EnsureAuth(cfg, profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if sessionIDFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: session-id is required")
		os.Exit(1)
	}

	// Support short ID prefix matching
	if resolved, err := session.ResolveShortID(sessionIDFlag, profile); err == nil {
		sessionIDFlag = resolved
	}

	client := api.NewClient(cfg)

	fmt.Printf("Resuming session %s...\n", sessionIDFlag)

	resp, err := client.GetSession(sessionIDFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resuming session: %v\n", err)
		os.Exit(1)
	}

	// Try to load existing session state to preserve role_sent status
	existingState, _ := session.Load(sessionIDFlag)
	roleSent := false
	lastHookOutput := ""
	createdAt := time.Now()
	if existingState != nil {
		roleSent = existingState.RoleSent
		lastHookOutput = existingState.LastHookOutput
		if !existingState.CreatedAt.IsZero() {
			createdAt = existingState.CreatedAt
		}
	}

	// Save session state
	state := &session.State{
		SessionID:      resp.SessionID,
		Profile:        profile,
		URL:            resp.URL,
		PresignedURLs:  resp.PresignedURLs,
		CreatedAt:      createdAt,
		URLsUsed:       0,
		RoleSent:       roleSent,
		LastHookOutput: lastHookOutput,
	}

	if err := session.Save(state); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not save session state: %v\n", err)
	}

	// Set as current session
	if err := session.SetCurrentSessionID(state.SessionID, profile); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not set current session: %v\n", err)
	}

	fmt.Printf("%s Session resumed: %s\n", color.Success("✓"), color.SessionID(resp.SessionID))
	fmt.Printf("  View in browser: %s\n", color.URL(resp.URL))
	fmt.Printf("  Upload slots:    %s available\n", color.Info(fmt.Sprintf("%d", len(resp.PresignedURLs))))
	if profile != config.DefaultProfile {
		fmt.Printf("  Profile:         %s\n", color.Info(profile))
	}
	fmt.Println()
	fmt.Printf("You can now use '%s' to upload files or '%s' to ask questions.\n",
		color.Command("dx upload"), color.Command("dx ask"))
}


