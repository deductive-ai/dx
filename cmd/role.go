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
	"os/exec"
	"strings"

	"github.com/deductive-ai/dx/internal/config"
	"github.com/spf13/cobra"
)

var setRoleCmd = &cobra.Command{
	Use:   "set-role",
	Short: "Set your role for a profile",
	Long: `Set your role/persona for the current profile by opening a text editor.

The role is sent as the first message in new sessions, helping Deductive AI
understand your context and provide more relevant responses.

The editor used is determined by (in order of priority):
1. The editor configured via 'dx config --editor=<name>'
2. The $EDITOR environment variable
3. The $VISUAL environment variable
4. vim (Linux/macOS) or notepad (Windows)

Examples:
  dx set-role                    # Edit role for default profile
  dx set-role --profile=staging  # Edit role for staging profile`,
	Run: runSetRole,
}

var getRoleCmd = &cobra.Command{
	Use:   "get-role",
	Short: "Display the role for a profile",
	Long: `Display the configured role for a profile.

Examples:
  dx get-role                    # Show role for default profile
  dx get-role --profile=staging  # Show role for staging profile`,
	Run: runGetRole,
}

func init() {
	setRoleCmd.Hidden = true
	getRoleCmd.Hidden = true
	rootCmd.AddCommand(setRoleCmd)
	rootCmd.AddCommand(getRoleCmd)
}

func runSetRole(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	// Load config
	cfg, err := config.Load(profile)
	if err != nil {
		if profile == config.DefaultProfile {
			fmt.Fprintln(os.Stderr, "Error: No configuration found. Run 'dx ask' to get started.")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Profile '%s' not found. Run 'dx ask --profile=%s' to set it up.\n", profile, profile)
		}
		os.Exit(1)
	}

	// Create temp file with existing role
	tmpFile, err := os.CreateTemp("", "dx-role-*.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating temp file: %v\n", err)
		os.Exit(1)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Write existing role to temp file
	if cfg.Role != "" {
		if _, err := tmpFile.WriteString(cfg.Role); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to temp file: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Write a helpful comment
		helpText := `# Enter your role/persona below.
# This will be sent as the first message in new sessions.
# Lines starting with # will be removed.
#
# Example:
# I am a DevOps engineer debugging production issues.
# I primarily work with Kubernetes, AWS, and PostgreSQL.
# I prefer concise, actionable responses.

`
		if _, err := tmpFile.WriteString(helpText); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to temp file: %v\n", err)
			os.Exit(1)
		}
	}
	tmpFile.Close()

	// Get editor
	editor := cfg.GetEditor()

	// Open editor
	editorCmd := exec.Command(editor, tmpPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running editor '%s': %v\n", editor, err)
		fmt.Fprintln(os.Stderr, "Set $EDITOR or run: dx config --editor=<name>")
		os.Exit(1)
	}

	// Read content from temp file
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading temp file: %v\n", err)
		os.Exit(1)
	}

	// Remove comment lines
	role := removeCommentLines(string(content))

	// Save role to config
	cfg.Role = role
	if err := config.Save(cfg, profile); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
		os.Exit(1)
	}

	if role == "" {
		fmt.Println("✓ Role cleared")
	} else {
		fmt.Println("✓ Role saved")
		if profile != config.DefaultProfile {
			fmt.Printf("  Profile: %s\n", profile)
		}
		fmt.Println("  The role will be sent as the first message in new sessions.")
	}
}

func runGetRole(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	// Load config
	cfg, err := config.Load(profile)
	if err != nil {
		if profile == config.DefaultProfile {
			fmt.Fprintln(os.Stderr, "Error: No configuration found. Run 'dx ask' to get started.")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Profile '%s' not found. Run 'dx ask --profile=%s' to set it up.\n", profile, profile)
		}
		os.Exit(1)
	}

	if cfg.Role == "" {
		fmt.Printf("No role configured for profile '%s'\n", profile)
		fmt.Println("Use 'dx set-role' to configure your role.")
		return
	}

	fmt.Printf("Role for profile '%s':\n", profile)
	fmt.Println("---")
	fmt.Println(cfg.Role)
	fmt.Println("---")
}

// removeCommentLines removes lines starting with # from the text
func removeCommentLines(text string) string {
	var result []byte
	var lineStart = 0
	var inComment = false

	for i := 0; i < len(text); i++ {
		if i == lineStart && text[i] == '#' {
			inComment = true
		}
		if text[i] == '\n' {
			if !inComment {
				result = append(result, text[lineStart:i+1]...)
			}
			lineStart = i + 1
			inComment = false
		}
	}

	// Handle last line without newline
	if lineStart < len(text) && !inComment {
		result = append(result, text[lineStart:]...)
	}

	// Trim leading/trailing whitespace
	return strings.TrimSpace(string(result))
}
