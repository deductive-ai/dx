// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deductive-ai/dx/internal/color"
	"github.com/spf13/cobra"
)

var installFlag bool

var completionCmd = &cobra.Command{
	Use:    "completion [bash|zsh|fish|powershell]",
	Short:  "Generate shell completion scripts",
	Hidden: true,
	Long: `Generate shell completion scripts for dx.

OUTPUT TO STDOUT (default):
  dx completion bash    # Print bash completions
  dx completion zsh     # Print zsh completions

INSTALL DIRECTLY (--install flag):
  dx completion bash --install   # Add to ~/.bashrc
  dx completion zsh --install    # Add to ~/.zshrc
  dx completion fish --install   # Add to ~/.config/fish/completions/

MANUAL INSTALLATION:

Bash:
  # Add to ~/.bashrc or ~/.bash_profile:
  source <(dx completion bash)
  
  # Or save to file:
  dx completion bash > /etc/bash_completion.d/dx

Zsh:
  # Add to ~/.zshrc:
  source <(dx completion zsh)
  
  # Or enable completions directory and save:
  dx completion zsh > "${fpath[1]}/_dx"

Fish:
  dx completion fish | source
  
  # Or save to completions directory:
  dx completion fish > ~/.config/fish/completions/dx.fish

PowerShell:
  # Add to your PowerShell profile:
  dx completion powershell | Out-String | Invoke-Expression

After installation, restart your shell or source the configuration file.`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run:                   runCompletion,
}

func init() {
	rootCmd.AddCommand(completionCmd)

	completionCmd.Flags().BoolVar(&installFlag, "install", false, "Install completions to shell config file")
}

func runCompletion(cmd *cobra.Command, args []string) {
	shell := args[0]
	
	if installFlag {
		if err := installCompletion(cmd, shell); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	
	// Output to stdout
	switch shell {
	case "bash":
		_ = cmd.Root().GenBashCompletion(os.Stdout)
	case "zsh":
		_ = cmd.Root().GenZshCompletion(os.Stdout)
	case "fish":
		_ = cmd.Root().GenFishCompletion(os.Stdout, true)
	case "powershell":
		_ = cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
	}
}

// InstallCompletions installs shell completions for the given shell.
// Called by `dx completion --install`.
func InstallCompletions(cmd *cobra.Command, shell string) error {
	return installCompletion(cmd, shell)
}

func installCompletion(cmd *cobra.Command, shell string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}
	
	var targetFile string
	var completionLine string
	var appendToFile bool
	
	switch shell {
	case "bash":
		// Check for .bashrc or .bash_profile
		bashrc := filepath.Join(home, ".bashrc")
		bashProfile := filepath.Join(home, ".bash_profile")
		
		if _, err := os.Stat(bashrc); err == nil {
			targetFile = bashrc
		} else if _, err := os.Stat(bashProfile); err == nil {
			targetFile = bashProfile
		} else {
			targetFile = bashrc // Create .bashrc if neither exists
		}
		completionLine = `source <(dx completion bash)`
		appendToFile = true
		
	case "zsh":
		targetFile = filepath.Join(home, ".zshrc")
		completionLine = `source <(dx completion zsh)`
		appendToFile = true
		
	case "fish":
		// Fish uses a completions directory
		fishDir := filepath.Join(home, ".config", "fish", "completions")
		if err := os.MkdirAll(fishDir, 0755); err != nil {
			return fmt.Errorf("could not create fish completions directory: %w", err)
		}
		targetFile = filepath.Join(fishDir, "dx.fish")
		appendToFile = false // Write the whole file
		
	case "powershell":
		// PowerShell profile location varies
		return fmt.Errorf("PowerShell auto-install not supported. Please add manually:\n  dx completion powershell | Out-String | Invoke-Expression")
		
	default:
		return fmt.Errorf("unknown shell: %s", shell)
	}
	
	if appendToFile {
		// Check if already installed
		if content, err := os.ReadFile(targetFile); err == nil {
			if strings.Contains(string(content), "dx completion "+shell) {
				fmt.Printf("%s Completions already installed in %s\n", color.Success("✓"), targetFile)
				return nil
			}
		}
		
		// Append to config file
		f, err := os.OpenFile(targetFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("could not open %s: %w", targetFile, err)
		}
		defer f.Close()
		
		// Add a newline and comment before the completion line
		installContent := fmt.Sprintf("\n# DX completions\n%s\n", completionLine)
		if _, err := f.WriteString(installContent); err != nil {
			return fmt.Errorf("could not write to %s: %w", targetFile, err)
		}
		
		fmt.Printf("%s Completions installed to %s\n", color.Success("✓"), targetFile)
		fmt.Printf("  Restart your shell or run: %s\n", color.Command("source "+targetFile))
		
	} else {
		// Write completion file directly (fish)
		f, err := os.Create(targetFile)
		if err != nil {
			return fmt.Errorf("could not create %s: %w", targetFile, err)
		}
		defer f.Close()
		
		if err := cmd.Root().GenFishCompletion(f, true); err != nil {
			return fmt.Errorf("could not generate fish completions: %w", err)
		}
		
		fmt.Printf("%s Completions installed to %s\n", color.Success("✓"), targetFile)
		fmt.Println("  Completions will be available in new fish shells.")
	}
	
	return nil
}
