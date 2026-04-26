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
	"path/filepath"
	"strconv"

	"github.com/deductive-ai/dx/internal/config"
	"github.com/spf13/cobra"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage message hooks",
	Long: `Manage hooks that run on every message send.

WHAT ARE HOOKS?

Hooks are shell scripts that run before sending each message with 'dx ask'.
The stdout of hooks is included in the message wrapped in <appendix> tags,
but only when the output differs from the previous run in the same session.

This is useful for gathering dynamic system information (e.g., resource usage,
container status, recent logs) that may change between questions.

HOOK SCRIPT FORMAT:

Hooks must be executable shell scripts. The script's stdout will be captured
and included with your messages. Example hook script:

  #!/bin/bash
  # gather-info.sh - System status hook
  
  echo "=== System Status ==="
  echo "Load: $(uptime | awk -F'load average:' '{print $2}')"
  echo "Memory: $(free -h | grep Mem | awk '{print $3 "/" $2}')"
  echo "Disk: $(df -h / | tail -1 | awk '{print $5 " used"}')"
  
  echo ""
  echo "=== Recent Errors ==="
  tail -5 /var/log/app/error.log 2>/dev/null || echo "No recent errors"

HOW HOOKS WORK:

1. When you run 'dx ask', all configured hooks execute
2. Hook output is combined and compared to the last run
3. If output changed, it's appended to your message in <appendix> tags
4. Deductive AI sees this context with your question

Examples:
  dx hook add /path/to/script.sh
  dx hook add ./gather-info.sh --profile=staging
  dx hook list
  dx hook remove 0`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var hookAddCmd = &cobra.Command{
	Use:   "add <script-path>",
	Short: "Add a hook script",
	Long: `Add a hook script to run on every message send.

The script must be executable and will be run with bash.
Its stdout will be appended to messages in <appendix> tags.

SCRIPT REQUIREMENTS:
- Must be an executable file (chmod +x script.sh)
- Should output useful context to stdout
- Should exit quickly (delays message sending)
- stderr is ignored (use for logging/debugging)

Examples:
  dx hook add /path/to/gather-info.sh
  dx hook add ./my-hook.sh --profile=staging`,
	Args: cobra.ExactArgs(1),
	Run:  runHookAdd,
}

var hookListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered hooks",
	Long: `List all registered hooks for the current profile.

Shows hook index, status (✓ = exists, ✗ = missing), and path.
Use the index with 'dx hook remove' to remove a hook.

Examples:
  dx hook list
  dx hook list --profile=staging`,
	Run: runHookList,
}

var hookRemoveCmd = &cobra.Command{
	Use:   "remove <index>",
	Short: "Remove a hook by index",
	Long: `Remove a hook by its index (from 'dx hook list').

The index is shown in square brackets when listing hooks.

Examples:
  dx hook remove 0
  dx hook remove 1 --profile=staging`,
	Args: cobra.ExactArgs(1),
	Run:  runHookRemove,
}

func init() {
	hookCmd.Hidden = true
	rootCmd.AddCommand(hookCmd)
	hookCmd.AddCommand(hookAddCmd)
	hookCmd.AddCommand(hookListCmd)
	hookCmd.AddCommand(hookRemoveCmd)
}

func runHookAdd(cmd *cobra.Command, args []string) {
	profile := GetProfile()
	scriptPath := args[0]

	// Load config
	cfg, err := config.Load(profile)
	if err != nil {
		if profile == config.DefaultProfile {
			fmt.Fprintln(os.Stderr, "Error: No configuration found. Run 'dx config' first.")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Profile '%s' not found. Run 'dx config --profile=%s' first.\n", profile, profile)
		}
		os.Exit(1)
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(scriptPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// Check if file exists
	info, err := os.Stat(absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Script not found: %s\n", absPath)
		os.Exit(1)
	}

	// Check if it's a file (not directory)
	if info.IsDir() {
		fmt.Fprintln(os.Stderr, "Error: Path is a directory, not a script file")
		os.Exit(1)
	}

	if info.Mode()&0111 == 0 {
		fmt.Fprintf(os.Stderr, "Warning: %s is not executable. Run: chmod +x %s\n", absPath, absPath)
	}

	// Check if already registered
	for _, hook := range cfg.Hooks {
		if hook == absPath {
			fmt.Fprintf(os.Stderr, "Error: Hook already registered: %s\n", absPath)
			os.Exit(1)
		}
	}

	// Add to hooks
	cfg.Hooks = append(cfg.Hooks, absPath)

	// Save config
	if err := config.Save(cfg, profile); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Hook added: %s\n", absPath)
	if profile != config.DefaultProfile {
		fmt.Printf("  Profile: %s\n", profile)
	}
	fmt.Println("  The hook will run on every 'dx ask' message.")
}

func runHookList(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	// Load config
	cfg, err := config.Load(profile)
	if err != nil {
		if profile == config.DefaultProfile {
			fmt.Fprintln(os.Stderr, "Error: No configuration found. Run 'dx config' first.")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Profile '%s' not found. Run 'dx config --profile=%s' first.\n", profile, profile)
		}
		os.Exit(1)
	}

	if len(cfg.Hooks) == 0 {
		fmt.Printf("No hooks configured for profile '%s'\n", profile)
		fmt.Println("Use 'dx hook add <script>' to add a hook.")
		return
	}

	fmt.Printf("Hooks for profile '%s':\n", profile)
	for i, hook := range cfg.Hooks {
		status := "✓"
		// Check if file still exists
		if _, err := os.Stat(hook); err != nil {
			status = "✗ (not found)"
		}
		fmt.Printf("  [%d] %s %s\n", i, hook, status)
	}
}

func runHookRemove(cmd *cobra.Command, args []string) {
	profile := GetProfile()
	indexStr := args[0]

	// Parse index
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid index: %s\n", indexStr)
		os.Exit(1)
	}

	// Load config
	cfg, err := config.Load(profile)
	if err != nil {
		if profile == config.DefaultProfile {
			fmt.Fprintln(os.Stderr, "Error: No configuration found. Run 'dx config' first.")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Profile '%s' not found. Run 'dx config --profile=%s' first.\n", profile, profile)
		}
		os.Exit(1)
	}

	// Validate index
	if len(cfg.Hooks) == 0 {
		fmt.Fprintln(os.Stderr, "Error: No hooks configured. Use 'dx hook list' to see available hooks.")
		os.Exit(1)
	}
	if index < 0 || index >= len(cfg.Hooks) {
		if len(cfg.Hooks) == 1 {
			fmt.Fprintf(os.Stderr, "Error: Invalid index %d (only valid index is 0)\n", index)
		} else {
			fmt.Fprintf(os.Stderr, "Error: Invalid index %d (valid range: 0-%d)\n", index, len(cfg.Hooks)-1)
		}
		os.Exit(1)
	}

	// Remove hook
	removedHook := cfg.Hooks[index]
	cfg.Hooks = append(cfg.Hooks[:index], cfg.Hooks[index+1:]...)

	// Save config
	if err := config.Save(cfg, profile); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Hook removed: %s\n", removedHook)
	if profile != config.DefaultProfile {
		fmt.Printf("  Profile: %s\n", profile)
	}
}
