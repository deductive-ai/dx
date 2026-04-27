// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deductive-ai/dx/internal/color"
	"github.com/spf13/cobra"
)

//go:embed skill_content.md
var skillContent string

var skillForceFlag bool

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage the dx agent skill",
	Long: `Install or print the dx SKILL.md for AI agent integration.

The skill file teaches AI agents how to use dx. Install writes to
user-level skill directories for Claude Code, Cursor, GitHub Copilot,
and OpenAI Codex -- one install covers every agent in every project.

Examples:
  dx skill install          # Install to all agent skill directories
  dx skill install --force  # Overwrite existing installations
  dx skill print            # Print SKILL.md to stdout`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var skillInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install SKILL.md to the agent skill directory",
	Long: `Write the embedded SKILL.md to all user-level agent skill directories.

Targets:
  ~/.claude/skills/dx/SKILL.md    (Claude Code)
  ~/.cursor/skills/dx/SKILL.md    (Cursor)
  ~/.copilot/skills/dx/SKILL.md   (GitHub Copilot)
  ~/.agents/skills/dx/SKILL.md    (OpenAI Codex)

The install is idempotent: if a file already exists and matches,
it prints "already installed". Use --force to overwrite.`,
	Run: runSkillInstall,
}

var skillPrintCmd = &cobra.Command{
	Use:   "print",
	Short: "Print the embedded SKILL.md to stdout",
	Long: `Dump the embedded SKILL.md content to stdout.

Useful for piping into a custom location:
  dx skill print > /path/to/SKILL.md`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(skillContent)
	},
}

func init() {
	rootCmd.AddCommand(skillCmd)
	skillCmd.AddCommand(skillInstallCmd)
	skillCmd.AddCommand(skillPrintCmd)

	skillInstallCmd.Flags().BoolVar(&skillForceFlag, "force", false, "Overwrite existing SKILL.md")
}

func userSkillPaths() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not determine home directory: %w", err)
	}
	return []string{
		filepath.Join(home, ".claude", "skills", "dx", "SKILL.md"),
		filepath.Join(home, ".cursor", "skills", "dx", "SKILL.md"),
		filepath.Join(home, ".copilot", "skills", "dx", "SKILL.md"),
		filepath.Join(home, ".agents", "skills", "dx", "SKILL.md"),
	}, nil
}

func runSkillInstall(cmd *cobra.Command, args []string) {
	targets, err := userSkillPaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var hadError bool
	for _, targetPath := range targets {
		if !skillForceFlag {
			if existing, err := os.ReadFile(targetPath); err == nil {
				if string(existing) == skillContent {
					fmt.Printf("%s Skill already installed at %s\n", color.Success("✓"), targetPath)
					continue
				}
				fmt.Fprintf(os.Stderr, "Warning: %s already exists (content differs). Use --force to overwrite.\n", targetPath)
				hadError = true
				continue
			}
		}

		dir := filepath.Dir(targetPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not create directory %s: %v\n", dir, err)
			hadError = true
			continue
		}

		if err := os.WriteFile(targetPath, []byte(skillContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not write %s: %v\n", targetPath, err)
			hadError = true
			continue
		}

		fmt.Printf("%s Skill installed to %s\n", color.Success("✓"), targetPath)
	}

	if hadError {
		os.Exit(1)
	}
}
