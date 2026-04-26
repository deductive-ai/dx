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

The skill file teaches AI agents (Cursor, Claude Code, etc.) how to use dx.

Examples:
  dx skill install          # Auto-detect agent and install SKILL.md
  dx skill install --force  # Overwrite existing installation
  dx skill print            # Print SKILL.md to stdout`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var skillInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install SKILL.md to the agent skill directory",
	Long: `Write the embedded SKILL.md to the appropriate agent skill directory.

Detection order:
  1. .claude/ exists in cwd  → ~/.claude/skills/dx/SKILL.md
  2. .cursor/ exists in cwd  → .cursor/skills/dx/SKILL.md
  3. Default                 → .cursor/skills/dx/SKILL.md

The install is idempotent: if the file already exists and matches,
it prints "already installed" and exits. Use --force to overwrite.`,
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

func detectSkillPath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not determine working directory: %w", err)
	}

	// .claude/ in cwd → user-level Claude Code skills
	if info, err := os.Stat(filepath.Join(cwd, ".claude")); err == nil && info.IsDir() {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not determine home directory: %w", err)
		}
		return filepath.Join(home, ".claude", "skills", "dx", "SKILL.md"), nil
	}

	// .cursor/ in cwd → project-level Cursor skills
	if info, err := os.Stat(filepath.Join(cwd, ".cursor")); err == nil && info.IsDir() {
		return filepath.Join(cwd, ".cursor", "skills", "dx", "SKILL.md"), nil
	}

	// Default → project-level Cursor skills
	return filepath.Join(cwd, ".cursor", "skills", "dx", "SKILL.md"), nil
}

func runSkillInstall(cmd *cobra.Command, args []string) {
	targetPath, err := detectSkillPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !skillForceFlag {
		if existing, err := os.ReadFile(targetPath); err == nil {
			if string(existing) == skillContent {
				fmt.Printf("%s Skill already installed at %s\n", color.Success("✓"), targetPath)
				return
			}
			fmt.Fprintf(os.Stderr, "Error: %s already exists (content differs).\n", targetPath)
			fmt.Fprintf(os.Stderr, "Use --force to overwrite.\n")
			os.Exit(1)
		}
	}

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not create directory %s: %v\n", dir, err)
		os.Exit(1)
	}

	if err := os.WriteFile(targetPath, []byte(skillContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not write %s: %v\n", targetPath, err)
		os.Exit(1)
	}

	fmt.Printf("%s Skill installed to %s\n", color.Success("✓"), targetPath)
}
