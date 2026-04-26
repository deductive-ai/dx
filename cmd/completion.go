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
	Args:                  cobra.ExactValidArgs(1),
	Run:                   runCompletion,
}

var manCmd = &cobra.Command{
	Use:    "man",
	Short:  "Generate man page",
	Long:   `Generate a man page for dx and print to stdout.`,
	Hidden: true, // Hidden since most users won't need this
	Run:    runMan,
}

func init() {
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(manCmd)
	
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
		cmd.Root().GenBashCompletion(os.Stdout)
	case "zsh":
		cmd.Root().GenZshCompletion(os.Stdout)
	case "fish":
		cmd.Root().GenFishCompletion(os.Stdout, true)
	case "powershell":
		cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
	}
}

// InstallCompletions installs shell completions for the given shell.
// Called by `dx init` and `dx completion --install`.
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

func runMan(cmd *cobra.Command, args []string) {
	// Generate a simple man page format
	fmt.Println(".TH DX 1 \"2026\" \"dx\" \"Deductive AI CLI\"")
	fmt.Println(".SH NAME")
	fmt.Println("dx \\- Deductive AI CLI for server operations")
	fmt.Println(".SH SYNOPSIS")
	fmt.Println(".B dx")
	fmt.Println("[\\fIcommand\\fR] [\\fIoptions\\fR]")
	fmt.Println(".SH DESCRIPTION")
	fmt.Println("DX CLI tool for interacting with Deductive AI to analyze logs,")
	fmt.Println("investigate issues, and get AI-powered insights from your server.")
	fmt.Println(".SH COMMANDS")
	fmt.Println(".TP")
	fmt.Println(".B ask \\fI[question]\\fR")
	fmt.Println("Ask a question to Deductive AI and receive a streaming response.")
	fmt.Println(".TP")
	fmt.Println(".B auth")
	fmt.Println("Re-authenticate (OAuth) or get API key instructions; auth is set via dx config.")
	fmt.Println(".TP")
	fmt.Println(".B config")
	fmt.Println("Configure endpoint, auth mode (oauth/apikey), and editor.")
	fmt.Println(".TP")
	fmt.Println(".B create-session")
	fmt.Println("Create a new chat session with Deductive AI.")
	fmt.Println(".TP")
	fmt.Println(".B hook")
	fmt.Println("Manage message hooks that run on every message send.")
	fmt.Println(".TP")
	fmt.Println(".B ping")
	fmt.Println("Test connectivity to the Deductive AI server.")
	fmt.Println(".TP")
	fmt.Println(".B resume-session")
	fmt.Println("Resume an existing chat session by its ID.")
	fmt.Println(".TP")
	fmt.Println(".B session")
	fmt.Println("Manage sessions (list, clear).")
	fmt.Println(".TP")
	fmt.Println(".B set-role")
	fmt.Println("Set your role/persona for a profile.")
	fmt.Println(".TP")
	fmt.Println(".B status")
	fmt.Println("Show current CLI status.")
	fmt.Println(".TP")
	fmt.Println(".B upload")
	fmt.Println("Upload files to the current session.")
	fmt.Println(".TP")
	fmt.Println(".B version")
	fmt.Println("Print the CLI version.")
	fmt.Println(".TP")
	fmt.Println(".B completion \\fI[shell]\\fR")
	fmt.Println("Generate shell completion scripts.")
	fmt.Println(".SH OPTIONS")
	fmt.Println(".TP")
	fmt.Println(".B --profile=\\fIname\\fR")
	fmt.Println("Configuration profile to use (default: \"default\").")
	fmt.Println(".TP")
	fmt.Println(".B --version, -V")
	fmt.Println("Print version information.")
	fmt.Println(".TP")
	fmt.Println(".B --no-color")
	fmt.Println("Disable colored output.")
	fmt.Println(".TP")
	fmt.Println(".B --help, -h")
	fmt.Println("Show help for any command.")
	fmt.Println(".SH QUICK START")
	fmt.Println(".nf")
	fmt.Println("dx config --endpoint=https://app.deductive.ai --auth-mode=apikey --api-key=dak_xxxxx")
	fmt.Println("dx ping")
	fmt.Println("dx create-session")
	fmt.Println("dx ask \"what processes are using the most memory?\"")
	fmt.Println(".fi")
	fmt.Println(".SH FILES")
	fmt.Println(".TP")
	fmt.Println(".I ~/.dx/profiles/<profile>/config")
	fmt.Println("Profile configuration files (TOML format).")
	fmt.Println(".TP")
	fmt.Println(".I ~/.dx/sessions/")
	fmt.Println("Session state files.")
	fmt.Println(".TP")
	fmt.Println(".I ~/.dx/history")
	fmt.Println("Command history for interactive mode.")
	fmt.Println(".SH ENVIRONMENT")
	fmt.Println(".TP")
	fmt.Println(".B DX_API_KEY")
	fmt.Println("API key for authentication.")
	fmt.Println(".TP")
	fmt.Println(".B DX_NO_COLOR")
	fmt.Println("Disable colored output when set.")
	fmt.Println(".TP")
	fmt.Println(".B EDITOR, VISUAL")
	fmt.Println("Editor for 'set-role' command.")
	fmt.Println(".SH AUTHOR")
	fmt.Println("Deductive AI, Inc.")
	fmt.Println(".SH SEE ALSO")
	fmt.Println("Visit https://deductive.ai for documentation.")
}
