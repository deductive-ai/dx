// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"

	"github.com/deductive-ai/dx/internal/session"
	"github.com/spf13/cobra"
)

var investigateCmd = &cobra.Command{
	Use:   "investigate [question]",
	Short: "Deep root cause analysis with Deductive AI",
	Long: `Investigate an issue with Deductive AI using deep root cause analysis.

Use investigate when you need a comprehensive first answer. The AI explores
more paths, uses more tools, and produces a thorough analysis upfront.

For quick answers where you expect follow-ups, use "dx ask" instead.

Each invocation starts a fresh investigation. Sessions are not auto-resumed
across runs, but you can continue a previous investigation with --session.

Interactive mode (no arguments):
  dx investigate
  # Starts an interactive shell for multi-turn investigation

Non-interactive mode (question as argument):
  dx investigate "why is the payments service returning 500s?"
  # Investigates and exits after producing the analysis

Piped input:
  kubectl logs deploy/api --tail=200 | dx investigate "root cause this error spike"
  # The piped data is included as context for the investigation

Examples:
  dx investigate "why did the API start returning 503s at 2am?"
  kubectl logs deploy/payments --since=1h | dx investigate "root cause the payment failures"
  docker inspect $(docker ps -q) | dx investigate "which containers are misconfigured?"
  dx investigate --session abc123 "what about the database connection pool?"`,
	Example: `  # Investigate an incident
  dx investigate "why is the payments service returning 500s?"

  # Pipe logs for deep analysis
  kubectl logs deploy/api --tail=200 | dx investigate "root cause this error spike"

  # Continue a previous investigation
  dx investigate --session abc123 "follow up on the memory leak"

  # Interactive investigation
  dx investigate`,
	Run: runInvestigate,
}

var investigateSessionFlag string
var investigateTimeoutFlag int

func init() {
	rootCmd.AddCommand(investigateCmd)
	investigateCmd.Flags().StringVarP(&investigateSessionFlag, "session", "s", "", "Resume a previous investigation by session ID")
	investigateCmd.Flags().IntVar(&investigateTimeoutFlag, "timeout", 0, "Maximum total seconds to wait for complete response (0 = no limit)")
}

func runInvestigate(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	cfg := LoadOrBootstrap(profile)

	var err error
	cfg, err = EnsureAuth(cfg, profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	mode := &SessionMode{
		APIMode: "investigate",
		Persist: false,
	}

	var explicitState *session.State
	if investigateSessionFlag != "" {
		explicitState = resolveExplicitSession(cfg, profile, investigateSessionFlag)
	}

	question := buildQuestion(args)

	if question != "" {
		runNonInteractive(cfg, profile, question, explicitState, mode, investigateTimeoutFlag)
	} else {
		runInteractive(cfg, profile, explicitState, mode, investigateTimeoutFlag)
	}
}
