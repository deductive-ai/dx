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
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/session"
	"github.com/spf13/cobra"
)

var (
	endpointFlag      string
	editorFlag        string
	configApiKeyFlag  string
	authModeFlag      string
	noValidateFlag    bool
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure the DX CLI",
	Long: `Configure the DX CLI with the Deductive AI endpoint, auth mode, and preferences.

Profiles:
  Use --profile to create or modify different configurations.
  The default profile is "default".

Interactive mode (prompts for profile name, endpoint, auth mode, token if apikey, editor):
  dx config
  dx config --profile=staging

Non-interactive mode (settings provided via flags):
  dx config --endpoint=https://app.deductive.ai --auth-mode=apikey --api-key=dak_xxxxx
  dx config --endpoint=https://app.deductive.ai --auth-mode=oauth
  dx config --endpoint=http://localhost:8081 --profile=staging --auth-mode=apikey --api-key=dak_xxxxx

Set preferred text editor:
  dx config --editor=vim
  dx config --editor=nano

Role is set only via 'dx set-role', not in config.`,
	Run: runConfig,
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured profiles",
	Run:   runConfigList,
}

var configDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a profile",
	Long: `Delete a configuration profile and all its data.

The "default" profile cannot be deleted.

Examples:
  dx config delete --profile=staging`,
	Run: runConfigDelete,
}

func init() {
	configCmd.Hidden = true
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configDeleteCmd)

	configCmd.Flags().StringVarP(&endpointFlag, "endpoint", "e", "", "Deductive AI endpoint URL")
	configCmd.Flags().StringVar(&editorFlag, "editor", "", "Preferred text editor (e.g., vim, nano, code)")
	configCmd.Flags().StringVar(&configApiKeyFlag, "api-key", "", "API key for authentication (starts with dak_). Use with --auth-mode=apikey.")
	configCmd.Flags().StringVar(&authModeFlag, "auth-mode", "", "Authentication method: oauth or apikey")
	configCmd.Flags().BoolVar(&noValidateFlag, "no-validate", false, "Skip endpoint connectivity check")
}

func runConfig(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	// Load existing config or create new one
	cfg, err := config.Load(profile)
	if err != nil {
		cfg = &config.Config{}
	}

	// Track if any changes were made
	hasChanges := false
	var apiKeyToApply string
	reader := bufio.NewReader(os.Stdin)

	// Determine if we're in interactive mode (no flags provided)
	isInteractive := endpointFlag == "" && editorFlag == "" && configApiKeyFlag == "" && authModeFlag == ""

	// Interactive: prompt for profile name first and use it for this run
	if isInteractive {
		defaultProfile := GetProfile()
		if defaultProfile == "" {
			defaultProfile = config.DefaultProfile
		}
		fmt.Printf("Profile name [%s]: ", defaultProfile)
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			os.Exit(1)
		}
		name := strings.TrimSpace(input)
		if name != "" {
			profile = name
			// Reload config for the chosen profile
			cfg, err = config.Load(profile)
			if err != nil {
				cfg = &config.Config{}
			}
		}
	}

	// Handle endpoint
	endpoint := endpointFlag
	if isInteractive && cfg.Endpoint == "" {
		fmt.Print("Deductive AI endpoint (e.g., https://app.deductive.ai or http://localhost:8081): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			os.Exit(1)
		}
		endpoint = strings.TrimSpace(input)
	}

	// Process endpoint if provided
	if endpoint != "" {
		// Normalize endpoint (add https:// if no protocol specified)
		if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			endpoint = "https://" + endpoint
		}

		// Remove trailing slash
		endpoint = strings.TrimSuffix(endpoint, "/")

		if !noValidateFlag {
			fmt.Printf("Testing connection to %s...\n", endpoint)
			if err := api.Ping(endpoint); err != nil {
				fmt.Fprintf(os.Stderr, "Error: Cannot reach endpoint: %v\n", err)
				fmt.Fprintln(os.Stderr, "Please check the endpoint URL and try again.")
				os.Exit(1)
			}
		}

		cfg.Endpoint = endpoint
		hasChanges = true
		fmt.Printf("✓ Endpoint set to: %s\n", endpoint)
	}

	// If --api-key provided without --auth-mode, imply apikey
	if configApiKeyFlag != "" && authModeFlag == "" {
		cfg.AuthMethod = "apikey"
		hasChanges = true
	}

	// Handle auth mode (--auth-mode or interactive choice)
	authMode := strings.ToLower(strings.TrimSpace(authModeFlag))
	if authMode != "" && authMode != "oauth" && authMode != "apikey" {
		fmt.Fprintf(os.Stderr, "Error: --auth-mode must be oauth or apikey (got %q)\n", authModeFlag)
		os.Exit(1)
	}
	if authMode != "" {
		cfg.AuthMethod = authMode
		hasChanges = true
		fmt.Printf("✓ Auth mode set to: %s\n", authMode)
	} else if isInteractive {
		fmt.Println()
		fmt.Println("Authentication method:")
		fmt.Println("  1) OAuth (login via browser)")
		fmt.Println("  2) API key")
		fmt.Print("Enter choice [1-2]: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		switch choice {
		case "1":
			cfg.AuthMethod = "oauth"
			hasChanges = true
			fmt.Println("✓ Auth mode set to: oauth")
		case "2":
			cfg.AuthMethod = "apikey"
			hasChanges = true
			fmt.Println("✓ Auth mode set to: apikey")
		default:
			if choice != "" {
				fmt.Fprintln(os.Stderr, "Invalid choice; use 1 or 2")
				os.Exit(1)
			}
		}
	}

	// If auth mode is apikey: collect API key (flag or interactive); apply after save
	if cfg.AuthMethod == "apikey" {
		apiKeyToApply = configApiKeyFlag
		if apiKeyToApply == "" && isInteractive {
			fmt.Print("API key (starts with dak_; press Enter to skip): ")
			input, _ := reader.ReadString('\n')
			apiKeyToApply = strings.TrimSpace(input)
		}
		if apiKeyToApply != "" {
			if cfg.Endpoint == "" {
				fmt.Fprintln(os.Stderr, "Error: Endpoint is required before setting API key. Configure endpoint first.")
				os.Exit(1)
			}
			hasChanges = true
		}
	}

	// Handle editor flag or interactive prompt
	if editorFlag != "" {
		cfg.Editor = editorFlag
		hasChanges = true
		fmt.Printf("✓ Editor set to: %s\n", editorFlag)
	} else if isInteractive {
		fmt.Printf("Preferred editor [%s]: ", cfg.GetEditor())
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			cfg.Editor = input
			hasChanges = true
			fmt.Printf("✓ Editor set to: %s\n", input)
		}
	}

	// If no changes and not interactive, show error
	if !hasChanges && !isInteractive {
		fmt.Fprintln(os.Stderr, "Error: No configuration changes specified")
		fmt.Fprintln(os.Stderr, "Use --endpoint, --auth-mode, --api-key, or --editor to set configuration")
		os.Exit(1)
	}

	// If interactive but no changes (everything skipped), still save to create profile
	if isInteractive && !hasChanges && cfg.Endpoint == "" {
		fmt.Fprintln(os.Stderr, "Error: Endpoint is required for a new profile")
		os.Exit(1)
	}

	// Save configuration
	if err := config.Save(cfg, profile); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
		os.Exit(1)
	}

	configPath, _ := config.GetProfileConfigPath(profile)
	fmt.Printf("✓ Configuration saved to %s\n", configPath)

	if profile != config.DefaultProfile {
		fmt.Printf("  Profile: %s\n", profile)
	}

	// Apply API key if we collected one (interactive or flag)
	keyToApply := apiKeyToApply
	if keyToApply == "" {
		keyToApply = configApiKeyFlag
	}
	if keyToApply != "" {
		client := api.NewClientWithEndpoint(cfg.Endpoint)
		if err := authenticateWithAPIKey(client, profile, keyToApply); err != nil {
			fmt.Fprintf(os.Stderr, "Error authenticating: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if cfg.Endpoint != "" && !cfg.IsAuthenticated() {
		fmt.Println()
		switch cfg.AuthMethod {
		case "oauth":
			if profile == config.DefaultProfile {
				fmt.Println("Run 'dx auth' to sign in with OAuth.")
			} else {
				fmt.Printf("Run 'dx auth --profile=%s' to sign in with OAuth.\n", profile)
			}
		case "apikey":
			if profile == config.DefaultProfile {
				fmt.Println("Run 'dx config --api-key=<key>' to set your API key.")
			} else {
				fmt.Printf("Run 'dx config --profile=%s --api-key=<key>' to set your API key.\n", profile)
			}
		default:
			if profile == config.DefaultProfile {
				fmt.Println("Run 'dx auth' to authenticate (OAuth) or set auth mode and API key via 'dx config'.")
			} else {
				fmt.Printf("Run 'dx auth --profile=%s' or 'dx config --profile=%s --api-key=<key>'.\n", profile, profile)
			}
		}
	}
}

func runConfigList(cmd *cobra.Command, args []string) {
	profiles, err := config.ListProfiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing profiles: %v\n", err)
		os.Exit(1)
	}

	if len(profiles) == 0 {
		fmt.Println("No profiles configured.")
		fmt.Println("Run 'dx config' to create the default profile.")
		return
	}

	fmt.Println("Configured profiles:")
	for _, profile := range profiles {
		cfg, err := config.Load(profile)
		if err != nil {
			fmt.Printf("  %s (error loading)\n", profile)
			continue
		}

		authStatus := "not authenticated"
		if cfg.IsAuthenticated() {
			authStatus = "authenticated"
		}

		defaultMark := ""
		if profile == config.DefaultProfile {
			defaultMark = " (default)"
		}

		fmt.Printf("  %s%s\n", profile, defaultMark)
		fmt.Printf("    Endpoint: %s\n", cfg.Endpoint)
		fmt.Printf("    Auth: %s\n", authStatus)
		if cfg.Role != "" {
			fmt.Printf("    Role: %s (set via dx set-role)\n", truncateString(cfg.Role, 40))
		}
		if len(cfg.Hooks) > 0 {
			fmt.Printf("    Hooks: %d configured\n", len(cfg.Hooks))
		}
	}
}

func runConfigDelete(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	if profile == config.DefaultProfile {
		fmt.Fprintln(os.Stderr, "Error: Cannot delete the default profile.")
		os.Exit(1)
	}

	// Clean up sessions first, then the profile directory
	_ = session.DeleteForProfile(profile)

	if err := config.DeleteProfile(profile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Deleted profile '%s'\n", profile)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
