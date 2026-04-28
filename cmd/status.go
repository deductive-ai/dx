// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/session"
	"github.com/spf13/cobra"
)

// StatusOutput is the machine-readable form of dx status --json.
type StatusOutput struct {
	Profile       string         `json:"profile"`
	Endpoint      string         `json:"endpoint,omitempty"`
	Configured    bool           `json:"configured"`
	Authenticated bool           `json:"authenticated"`
	AuthMethod    string         `json:"auth_method,omitempty"`
	Session       *SessionStatus `json:"session"`
	// URL is a convenience top-level alias for session.url so that
	// SESSION_URL=$(dx status --json | jq -r .url) works directly.
	URL string `json:"url,omitempty"`
}

// SessionStatus is the session sub-object inside StatusOutput.
type SessionStatus struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

var statusJSONFlag bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current CLI status",
	Long: `Display the current status of the DX CLI including:
- Current profile and endpoint
- Authentication status
- Active session information

JSON output (--json):
  Emits a single JSON object suitable for scripting. The session URL is
  available at both .url (top-level shortcut) and .session.url:

    dx status --json | jq -r .url
    SESSION_URL=$(dx status --json | jq -r .url)

  Full shape:
    {
      "profile":        "default",
      "endpoint":       "https://app.deductive.ai",
      "configured":     true,
      "authenticated":  true,
      "auth_method":    "apikey",
      "url":            "https://app.deductive.ai/threads/<id>",
      "session": {
        "id":                "<uuid>",
        "url":               "https://app.deductive.ai/threads/<id>",
        "created_at":        "2024-01-01T00:00:00Z",
        "uploads_available": 8,
        "uploads_total":     10
      }
    }
  When no active session exists, "session" and "url" are null/omitted.

Examples:
  dx status
  dx status --json
  dx status --json | jq -r .url
  dx status --json --profile=staging`,
	Example: `  # Check connectivity and session
  dx status

  # Get session URL for scripting
  dx status --json | jq -r .url

  # Check status of a different profile
  dx status --profile=staging`,
	Hidden: true,
	Run:    runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVar(&statusJSONFlag, "json", false, "Output status as JSON")
}

func runStatus(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	if statusJSONFlag {
		runStatusJSON(profile)
		return
	}

	fmt.Printf("%s dx %s\n", color.Title("DX Status"), Version)
	fmt.Println()

	// Profile
	fmt.Printf("%s\n", color.Title("Profile"))
	fmt.Printf("  Name: %s\n", color.Info(profile))

	// Check if profile exists
	cfg, err := config.Load(profile)
	if err != nil {
		fmt.Printf("  Status: %s\n", color.Error("Not configured"))
		fmt.Printf("\n  Run '%s' to get started.\n", color.Command("dx ask"))
		return
	}

	fmt.Printf("  Endpoint: %s\n", color.URL(cfg.Endpoint))

	// Connectivity check (absorbs dx ping)
	fmt.Print("  Connectivity: ")
	if err := api.Ping(cfg.Endpoint); err != nil {
		fmt.Printf("%s (%s)\n", color.Error("✗ unreachable"), color.Muted(err.Error()))
	} else {
		fmt.Println(color.Success("✓ connected"))
	}
	fmt.Println()

	// Authentication
	fmt.Printf("%s\n", color.Title("Authentication"))
	if cfg.IsAuthenticated() {
		fmt.Printf("  Status: %s\n", color.Success("✓ Authenticated"))
		switch cfg.AuthMethod {
		case "oauth":
			fmt.Printf("  Method: OAuth\n")
			if !cfg.OAuthExpiresAt.IsZero() {
				remaining := time.Until(cfg.OAuthExpiresAt)
				if remaining > 0 {
					fmt.Printf("  Expires: %s (in %s)\n",
						cfg.OAuthExpiresAt.Format("2006-01-02 15:04:05"),
						formatDuration(remaining))
				} else {
					fmt.Printf("  Expires: %s\n", color.Error("Expired"))
				}
			}
		case "apikey":
			fmt.Printf("  Method: API Key\n")
			if len(cfg.APIKey) > 12 {
				fmt.Printf("  Key: %s...%s\n", cfg.APIKey[:8], cfg.APIKey[len(cfg.APIKey)-4:])
			}
		}
	} else {
		fmt.Printf("  Status: %s\n", color.Error("✗ Not authenticated"))
		fmt.Printf("  Run '%s' to re-authenticate.\n", color.Command("dx auth"))
	}
	fmt.Println()

	// Session
	fmt.Printf("%s\n", color.Title("Session"))
	state, _ := session.LoadCurrent(profile)
	if state != nil {
		fmt.Printf("  Status: %s\n", color.Success("✓ Active"))
		fmt.Printf("  URL: %s\n", color.URL(state.URL))
	} else {
		fmt.Printf("  Status: %s\n", color.Muted("No active session"))
		fmt.Printf("  Run '%s' to start one.\n", color.Command("dx ask"))
	}
}

func runStatusJSON(profile string) {
	out := StatusOutput{
		Profile: profile,
	}

	cfg, err := config.Load(profile)
	if err != nil {
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return
	}

	out.Configured = true
	out.Endpoint = cfg.Endpoint
	out.Authenticated = cfg.IsAuthenticated()
	out.AuthMethod = cfg.AuthMethod

	state, _ := session.LoadCurrent(profile)
	if state != nil {
		out.Session = &SessionStatus{
			ID:        state.SessionID,
			URL:       state.URL,
			CreatedAt: state.CreatedAt,
		}
		out.URL = state.URL
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshalling status: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}
