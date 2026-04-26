package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Set up DX CLI with your Deductive instance",
	Long: `Interactive wizard that configures the DX CLI in one step.

Walks you through:
  1. Your Deductive URL (e.g. https://acme.deductive.ai)
  2. Authentication (API key or browser login)
  3. Connection test
  4. Shell completions (optional)

If you're already configured, dx init offers to reconfigure.

Examples:
  dx init
  dx init --profile=staging`,
	Example: `  # First-time setup
  dx init

  # Set up a separate profile for staging
  dx init --profile=staging`,
	GroupID: "getting-started",
	Run:     runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) {
	profile := GetProfile()
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Printf("  %s\n", color.Title("Welcome to Deductive! Let's get you set up."))
	fmt.Println()

	cfg, err := config.Load(profile)
	if err == nil && cfg.Endpoint != "" {
		fmt.Printf("  You already have a configuration for profile %s.\n", color.Info(profile))
		fmt.Printf("  Endpoint: %s\n", color.URL(cfg.Endpoint))
		fmt.Println()
		fmt.Print("  Reconfigure? [y/N] ")
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) != "y" {
			fmt.Println()
			fmt.Printf("  Keeping existing configuration. Run %s to check status.\n", color.Command("dx status"))
			return
		}
		fmt.Println()
	} else {
		cfg = &config.Config{}
	}

	// Step 1: Endpoint
	fmt.Printf("  Your Deductive URL (e.g. %s): ", color.Muted("https://acme.deductive.ai"))
	endpointInput, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  Error reading input: %v\n", err)
		os.Exit(1)
	}
	endpoint := strings.TrimSpace(endpointInput)
	if endpoint == "" {
		fmt.Fprintln(os.Stderr, "\n  Error: URL is required.")
		os.Exit(1)
	}

	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	// Validate endpoint
	fmt.Print("  Checking endpoint... ")
	if err := api.Ping(endpoint); err != nil {
		fmt.Println(color.Error("✗"))
		fmt.Fprintf(os.Stderr, "  Cannot reach %s: %v\n", endpoint, err)
		fmt.Fprintln(os.Stderr, "  Please check the URL and try again.")
		os.Exit(1)
	}
	fmt.Println(color.Success("OK"))
	cfg.Endpoint = endpoint

	// Step 2: Authentication
	fmt.Println()
	fmt.Println("  Authentication method:")
	fmt.Printf("    %s API Key %s\n", color.Info("[1]"), color.Muted("(recommended — get one from Settings > API Keys)"))
	fmt.Printf("    %s Browser login %s\n", color.Info("[2]"), color.Muted("(OAuth)"))
	fmt.Println()
	fmt.Print("  > ")
	authChoice, _ := reader.ReadString('\n')
	authChoice = strings.TrimSpace(authChoice)

	// Save config before auth so the profile directory exists
	if err := config.Save(cfg, profile); err != nil {
		fmt.Fprintf(os.Stderr, "\n  Error saving configuration: %v\n", err)
		os.Exit(1)
	}

	switch authChoice {
	case "1", "":
		cfg.AuthMethod = "apikey"
		config.Save(cfg, profile)

		fmt.Print("  API Key: ")
		keyInput, _ := reader.ReadString('\n')
		apiKey := strings.TrimSpace(keyInput)
		if apiKey == "" {
			fmt.Fprintln(os.Stderr, "\n  Error: API key is required.")
			os.Exit(1)
		}

		client := api.NewClientWithEndpoint(cfg.Endpoint)
		fmt.Print("  Verifying... ")
		if err := authenticateWithAPIKey(client, profile, apiKey); err != nil {
			fmt.Println(color.Error("✗"))
			fmt.Fprintf(os.Stderr, "  %v\n", err)
			os.Exit(1)
		}

	case "2":
		cfg.AuthMethod = "oauth"
		config.Save(cfg, profile)

		client := api.NewClientWithEndpoint(cfg.Endpoint)
		if err := authenticateWithOAuth(client, profile); err != nil {
			fmt.Fprintf(os.Stderr, "\n  Error: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintln(os.Stderr, "\n  Invalid choice. Use 1 or 2.")
		os.Exit(1)
	}

	// Step 3: Offer shell completions
	fmt.Println()
	shell := detectShell()
	if shell != "" {
		fmt.Printf("  Install shell completions for %s? [Y/n] ", color.Info(shell))
		compChoice, _ := reader.ReadString('\n')
		compChoice = strings.TrimSpace(strings.ToLower(compChoice))
		if compChoice == "" || compChoice == "y" || compChoice == "yes" {
			if err := InstallCompletions(rootCmd, shell); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: Could not install completions: %v\n", err)
			}
		}
	}

	// Step 4: Success
	fmt.Println()
	fmt.Printf("  %s Setup complete!\n", color.Success("✓"))
	fmt.Println()
	fmt.Println("  Try it:")
	fmt.Printf("    %s\n", color.Command(`dx ask "what's using the most memory right now?"`))
	fmt.Println()
}

func detectShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return ""
	}
	if strings.Contains(shell, "zsh") {
		return "zsh"
	}
	if strings.Contains(shell, "bash") {
		return "bash"
	}
	if strings.Contains(shell, "fish") {
		return "fish"
	}
	return ""
}
