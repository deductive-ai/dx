package cmd

import (
	"fmt"
	"os"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Re-authenticate with Deductive",
	Long: `Re-authenticate with your Deductive instance.

For OAuth profiles, this runs the browser-based device flow.
For API key profiles, this prints instructions to update your key.

Examples:
  dx login
  dx login --profile=staging`,
	Example: `  dx login
  dx login --profile=staging`,
	GroupID: "getting-started",
	Run:     runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)
}

func runLogin(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	cfg, err := config.Load(profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s No configuration found.\n", color.Error("✗"))
		fmt.Fprintf(os.Stderr, "Run %s to set up the CLI first.\n", color.Command("dx init"))
		os.Exit(1)
	}

	if cfg.Endpoint == "" {
		fmt.Fprintf(os.Stderr, "%s No endpoint configured.\n", color.Error("✗"))
		fmt.Fprintf(os.Stderr, "Run %s to set up the CLI first.\n", color.Command("dx init"))
		os.Exit(1)
	}

	client := api.NewClientWithEndpoint(cfg.Endpoint)

	switch cfg.AuthMethod {
	case "oauth":
		if err := authenticateWithOAuth(client, profile); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "apikey":
		fmt.Println("This profile uses API key authentication.")
		fmt.Println("To update your key:")
		fmt.Println("  1) Generate a new key in Settings > API Keys")
		if profile == config.DefaultProfile {
			fmt.Printf("  2) Run: %s\n", color.Command("dx config --api-key=<new_key>"))
		} else {
			fmt.Printf("  2) Run: %s\n", color.Command(fmt.Sprintf("dx config --profile=%s --api-key=<new_key>", profile)))
		}
	default:
		fmt.Fprintf(os.Stderr, "%s No auth method configured.\n", color.Error("✗"))
		fmt.Fprintf(os.Stderr, "Run %s to set up the CLI.\n", color.Command("dx init"))
		os.Exit(1)
	}
}
