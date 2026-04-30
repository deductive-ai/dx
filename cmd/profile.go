package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/session"
	"github.com/spf13/cobra"
)

var (
	createEndpointFlag string
	createApiKeyFlag   string
	createNoValidate   bool
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage configuration profiles",
	Long: `List and manage configuration profiles.

Each profile stores an endpoint, auth credentials, and preferences.
Use --profile on any command to target a specific profile,
or switch the default with dx profile use.

Examples:
  dx profile                          # List all profiles
  dx profile create staging --endpoint=https://staging.deductive.ai --api-key=dak_xxx
  dx profile use staging              # Switch active profile
  dx profile delete --profile=staging # Delete a profile`,
	Run: runProfileList,
}

var profileCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create or update a profile",
	Long: `Create a new profile with the given endpoint and authentication.

If the profile already exists, its settings are updated.

Examples:
  dx profile create staging --endpoint=https://staging.deductive.ai
  dx profile create staging --endpoint=https://staging.deductive.ai --api-key=dak_xxxxx
  dx profile create local --endpoint=http://localhost:8081 --api-key=dak_xxxxx --no-validate`,
	Args: cobra.ExactArgs(1),
	Run:  runProfileCreate,
}

var profileDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a profile",
	Long: `Delete a configuration profile and all its data.

The "default" profile cannot be deleted.

Examples:
  dx profile delete --profile=staging`,
	Run: runProfileDelete,
}

var profileUseCmd = &cobra.Command{
	Use:   "use <profile>",
	Short: "Set the active profile",
	Long: `Set the active profile for all dx commands.

The active profile is used when --profile is not specified.
Override per-command with --profile or the DX_PROFILE env var.

Examples:
  dx profile use staging
  dx profile use default`,
	Args: cobra.ExactArgs(1),
	Run:  runProfileUse,
}

func init() {
	profileCmd.Hidden = true
	setupCmd.AddCommand(profileCmd)
	profileCmd.AddCommand(profileCreateCmd)
	profileCmd.AddCommand(profileDeleteCmd)
	profileCmd.AddCommand(profileUseCmd)

	profileCreateCmd.Flags().StringVarP(&createEndpointFlag, "endpoint", "e", "", "Deductive AI endpoint URL (required)")
	profileCreateCmd.Flags().StringVar(&createApiKeyFlag, "api-key", "", "API key (starts with dak_)")
	profileCreateCmd.Flags().BoolVar(&createNoValidate, "no-validate", false, "Skip endpoint connectivity check")
	_ = profileCreateCmd.MarkFlagRequired("endpoint")
}

func runProfileList(cmd *cobra.Command, args []string) {
	profiles, err := config.ListProfiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing profiles: %v\n", err)
		os.Exit(1)
	}

	if len(profiles) == 0 {
		fmt.Println("No profiles configured.")
		fmt.Println("Run 'dx setup init' to get started.")
		return
	}

	activeProfile := GetProfile()

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

		marker := " "
		if profile == activeProfile {
			marker = "*"
		}

		fmt.Printf("\n  %s %s\n", marker, profile)
		fmt.Printf("      Endpoint: %s\n", cfg.Endpoint)
		fmt.Printf("      Auth: %s\n", authStatus)
	}
}

func runProfileDelete(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	if profile == config.DefaultProfile {
		fmt.Fprintln(os.Stderr, "Error: Cannot delete the default profile.")
		os.Exit(1)
	}

	_ = session.DeleteForProfile(profile)

	if err := config.DeleteProfile(profile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Deleted profile '%s'\n", profile)
}

func runProfileUse(cmd *cobra.Command, args []string) {
	profile := args[0]

	if _, err := config.Load(profile); err != nil {
		fmt.Fprintf(os.Stderr, "%s Profile '%s' not found.\n", color.Error("✗"), profile)
		fmt.Fprintf(os.Stderr, "Run %s to set it up.\n", color.Command("dx profile create "+profile+" --endpoint=..."))
		os.Exit(1)
	}

	if err := config.WriteActiveProfile(profile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s Active profile set to '%s'\n", color.Success("✓"), profile)
}

func runProfileCreate(cmd *cobra.Command, args []string) {
	profile := args[0]

	cfg, err := config.Load(profile)
	if err != nil {
		cfg = &config.Config{}
	}

	endpoint := createEndpointFlag
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	if !createNoValidate {
		fmt.Printf("Testing connection to %s... ", endpoint)
		if err := api.Ping(endpoint); err != nil {
			fmt.Println(color.Error("✗"))
			fmt.Fprintf(os.Stderr, "Cannot reach endpoint: %v\n", err)
			fmt.Fprintln(os.Stderr, "Use --no-validate to skip this check.")
			os.Exit(1)
		}
		fmt.Println(color.Success("OK"))
	}

	cfg.Endpoint = endpoint

	if createApiKeyFlag != "" {
		cfg.AuthMethod = "apikey"
	} else {
		cfg.AuthMethod = "oauth"
	}

	if err := config.Save(cfg, profile); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving profile: %v\n", err)
		os.Exit(1)
	}

	if err := config.WriteActiveProfile(profile); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not set active profile: %v\n", err)
	}

	fmt.Printf("%s Profile '%s' saved and set as active (endpoint: %s)\n", color.Success("✓"), profile, endpoint)

	if createApiKeyFlag != "" {
		client := api.NewClientWithEndpoint(cfg.Endpoint)
		if err := authenticateWithAPIKey(client, profile, createApiKeyFlag); err != nil {
			fmt.Fprintf(os.Stderr, "Error authenticating: %v\n", err)
			os.Exit(1)
		}
		return
	}

	client := api.NewClientWithEndpoint(cfg.Endpoint)
	if err := authenticateWithOAuth(client, profile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
