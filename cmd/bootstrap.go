package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
)

// LoadOrBootstrap attempts to load config for the given profile.
// If no config exists, it checks environment variables first, then runs
// an inline interactive setup. Returns the config ready for use.
func LoadOrBootstrap(profile string) *config.Config {
	cfg, err := LoadConfigFromEnv()
	if err == nil {
		return cfg
	}

	cfg, err = config.Load(profile)
	if err == nil {
		return cfg
	}

	// No env vars, no config file -- interactive bootstrap
	if !isInteractiveTerminal() {
		fmt.Fprintln(os.Stderr, "Error: No configuration found and stdin is not a terminal.")
		fmt.Fprintf(os.Stderr, "Set DX_API_KEY and DX_ENDPOINT environment variables, or run %s interactively.\n", color.Command("dx setup init"))
		os.Exit(1)
	}

	return runBootstrap(profile)
}

// LoadConfigFromEnv constructs an in-memory Config from environment variables.
// Returns an error if the required variables are not set.
func LoadConfigFromEnv() (*config.Config, error) {
	apiKey := os.Getenv("DX_API_KEY")
	endpoint := os.Getenv("DX_ENDPOINT")

	if apiKey == "" || endpoint == "" {
		return nil, fmt.Errorf("DX_API_KEY and DX_ENDPOINT must both be set")
	}

	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	return &config.Config{
		Endpoint:   endpoint,
		AuthMethod: "apikey",
		APIKey:     apiKey,
	}, nil
}

// runBootstrap is the inline first-run setup triggered by `dx ask` when no config exists.
func runBootstrap(profile string) *config.Config {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Printf("  %s\n", color.Title("Welcome to Deductive! Let's get you set up."))
	fmt.Println()

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

	// If user pasted an API key instead of a URL, handle it
	if strings.HasPrefix(endpoint, "dak_") {
		return bootstrapWithAPIKey(endpoint, profile, reader)
	}

	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	// Validate endpoint
	fmt.Print("  Checking... ")
	if err := api.Ping(endpoint); err != nil {
		fmt.Println(color.Error("✗"))
		fmt.Fprintf(os.Stderr, "  Cannot reach %s: %v\n", endpoint, err)
		os.Exit(1)
	}
	fmt.Println(color.Success("OK"))

	cfg := &config.Config{Endpoint: endpoint}
	if err := config.Save(cfg, profile); err != nil {
		fmt.Fprintf(os.Stderr, "\n  Error saving configuration: %v\n", err)
		os.Exit(1)
	}

	// Step 2: Auth -- default to browser OAuth
	fmt.Println()
	fmt.Printf("  Authentication: paste an %s or press Enter for %s: ",
		color.Info("API key"),
		color.Info("browser login"))
	authInput, _ := reader.ReadString('\n')
	authInput = strings.TrimSpace(authInput)

	if authInput != "" && strings.HasPrefix(authInput, "dak_") {
		cfg.AuthMethod = "apikey"
		_ = config.Save(cfg, profile)

		client := api.NewClientWithEndpoint(cfg.Endpoint)
		fmt.Print("  Verifying... ")
		if err := authenticateWithAPIKey(client, profile, authInput); err != nil {
			fmt.Println(color.Error("✗"))
			fmt.Fprintf(os.Stderr, "  %v\n", err)
			os.Exit(1)
		}
	} else if authInput != "" {
		fmt.Fprintln(os.Stderr, "  Error: API keys start with dak_. Press Enter for browser login instead.")
		os.Exit(1)
	} else {
		cfg.AuthMethod = "oauth"
		_ = config.Save(cfg, profile)

		client := api.NewClientWithEndpoint(cfg.Endpoint)
		if err := authenticateWithOAuth(client, profile); err != nil {
			fmt.Fprintf(os.Stderr, "\n  Error: %v\n", err)
			os.Exit(1)
		}
	}

	// Reload config after auth
	cfg, err = config.Load(profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reloading config: %v\n", err)
		os.Exit(1)
	}

	tryInstallCompletions()

	fmt.Println()
	fmt.Printf("  %s Ready!\n", color.Success("✓"))
	fmt.Println()
	return cfg
}

// bootstrapWithAPIKey handles the case where user pastes an API key at the URL prompt.
func bootstrapWithAPIKey(apiKey string, profile string, reader *bufio.Reader) *config.Config {
	fmt.Println()
	fmt.Printf("  That looks like an API key. Enter your Deductive URL: ")
	urlInput, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  Error reading input: %v\n", err)
		os.Exit(1)
	}
	endpoint := strings.TrimSpace(urlInput)
	if endpoint == "" {
		fmt.Fprintln(os.Stderr, "\n  Error: URL is required.")
		os.Exit(1)
	}

	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	fmt.Print("  Checking... ")
	if err := api.Ping(endpoint); err != nil {
		fmt.Println(color.Error("✗"))
		fmt.Fprintf(os.Stderr, "  Cannot reach %s: %v\n", endpoint, err)
		os.Exit(1)
	}
	fmt.Println(color.Success("OK"))

	cfg := &config.Config{
		Endpoint:   endpoint,
		AuthMethod: "apikey",
	}
	if err := config.Save(cfg, profile); err != nil {
		fmt.Fprintf(os.Stderr, "\n  Error saving configuration: %v\n", err)
		os.Exit(1)
	}

	client := api.NewClientWithEndpoint(endpoint)
	fmt.Print("  Verifying key... ")
	if err := authenticateWithAPIKey(client, profile, apiKey); err != nil {
		fmt.Println(color.Error("✗"))
		fmt.Fprintf(os.Stderr, "  %v\n", err)
		os.Exit(1)
	}

	cfg, err = config.Load(profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reloading config: %v\n", err)
		os.Exit(1)
	}

	tryInstallCompletions()

	fmt.Println()
	fmt.Printf("  %s Ready!\n", color.Success("✓"))
	fmt.Println()
	return cfg
}

func tryInstallCompletions() {
	shell := filepath.Base(os.Getenv("SHELL"))
	switch shell {
	case "bash", "zsh", "fish":
		if err := InstallCompletions(rootCmd, shell); err != nil {
			fmt.Fprintf(os.Stderr, "  Note: could not install shell completions: %v\n", err)
		}
	}
}

func isInteractiveTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
