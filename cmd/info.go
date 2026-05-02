// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/spf13/cobra"
)

var infoJSONFlag bool

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show status, version, and diagnostics",
	Long: `Display the current status of the DX CLI including:
- Current profile and endpoint
- Authentication status
- Team context
- Active session information
- Version information

JSON output (--json):
  Emits a single JSON object suitable for scripting:

    dx info --json | jq -r .url

Examples:
  dx info
  dx info --json
  dx info version`,
	Run: runInfo,
}

var infoVersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

func init() {
	rootCmd.AddCommand(infoCmd)
	infoCmd.AddCommand(infoVersionCmd)
	infoCmd.Flags().BoolVar(&infoJSONFlag, "json", false, "Output status as JSON")
}

func runInfo(cmd *cobra.Command, args []string) {
	if infoJSONFlag {
		statusJSONFlag = true
	}
	runStatus(cmd, args)
}
