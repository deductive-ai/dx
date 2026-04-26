// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/logging"
	"github.com/deductive-ai/dx/internal/render"
	"github.com/deductive-ai/dx/internal/session"
	"github.com/deductive-ai/dx/internal/stream"
	"github.com/deductive-ai/dx/internal/telemetry"
	"github.com/deductive-ai/dx/internal/upload"
	"github.com/peterh/liner"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"
)

var askCmd = &cobra.Command{
	Use:   "ask [question]",
	Short: "Ask a question to Deductive AI",
	Long: `Ask a question to Deductive AI and receive a streaming response.

Interactive mode (no arguments):
  dx ask
  # Starts an interactive shell where you can ask multiple questions
  # Supports command history (up/down arrows) and line editing

Non-interactive mode (question as argument):
  dx ask "What processes are using the most memory?"
  # Asks a single question and exits after receiving the answer

Piped input:
  ps aux | dx ask "summarize these processes"
  # The piped data is uploaded as a file (stdin.txt) to the session,
  # then the question is sent. The agent can read the file for analysis.

  lsof -i | dx ask
  # Piped data is uploaded and interactive mode starts, so you can
  # ask multiple follow-up questions about the data.

Piped output:
  When stdout is piped (e.g. to jq), status messages are automatically
  redirected to stderr so only the answer appears on stdout.

Examples:
  dx ask "analyze the uploaded logs"
  ps aux --sort=-%mem | head -20 | dx ask "which processes need attention?"
  netstat -an | dx ask "are there any unusual connections?"
  lsof -i | dx ask
  dx ask "test query" --profile=staging`,
	Example: `  # Ask a single question
  dx ask "what processes are using the most memory?"

  # Pipe data for analysis
  ps aux | dx ask "which processes need attention?"

  # Start a fresh session
  dx ask --new "ignore previous context"

  # Interactive mode
  dx ask`,
	Run: runAsk,
}

var askSessionFlag string
var askTimeoutFlag int
var askNewFlag bool

func init() {
	rootCmd.AddCommand(askCmd)
	askCmd.Flags().StringVarP(&askSessionFlag, "session", "s", "", "Session ID to use (creates one if not specified)")
	askCmd.Flags().IntVar(&askTimeoutFlag, "timeout", 0, "Maximum total seconds to wait for complete response (0 = no limit)")
	askCmd.Flags().BoolVar(&askNewFlag, "new", false, "Start a new session (ignore any existing session)")
}

// ensureSession loads an existing session or creates a new one.
// Returns the session state and whether a new session was created.
func ensureSession(client *api.Client, profile string, preState *session.State, sw io.Writer) (*session.State, bool) {
	state := preState
	if state == nil || state.Profile != profile {
		var err error
		state, err = session.LoadCurrent(profile)
		if err != nil {
			state = nil
		}
	}

	reusing := state != nil && state.Profile == profile && !state.IsExpired() && !askNewFlag

	if reusing {
		age := time.Since(state.LastMessageAt)
		if state.LastMessageAt.IsZero() {
			age = time.Since(state.CreatedAt)
		}
		fmt.Fprintf(sw, "Continuing session (%s ago)\n", formatDuration(age))
		logging.Debug("Session loaded", "id", state.SessionID, "profile", profile)
		return state, false
	}

	fmt.Fprintln(sw, "Creating session...")
	resp, err := client.CreateSession(&api.SessionRequest{Mode: "ask"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating session: %v\n", err)
		os.Exit(1)
	}
	state = &session.State{
		SessionID:     resp.SessionID,
		Profile:       profile,
		URL:           resp.URL,
		PresignedURLs: resp.PresignedURLs,
		CreatedAt:     time.Now(),
	}
	_ = session.Save(state)
	_ = session.SetCurrentSessionID(state.SessionID, profile)
	logging.Debug("Session created", "id", resp.SessionID, "profile", profile)
	return state, true
}

func runAsk(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	// Check config and auth (env vars → config file → interactive bootstrap)
	cfg := LoadOrBootstrap(profile)

	var err error
	cfg, err = EnsureAuth(cfg, profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// If --session flag provided, load/resume that session in-memory and pass
	// it through to downstream helpers. We deliberately do NOT depend on
	// session.LoadCurrent() (which reads ~/.dx/profiles/<p>/current_session)
	// for this invocation: that pointer file is process-global, so two
	// concurrent `dx ask -s SID_A` and `dx ask -s SID_B` invocations would
	// race on it and both could end up using whichever SID wrote last.
	// Threading state in-memory makes parallel `dx ask -s` invocations safe.
	var explicitState *session.State
	if askSessionFlag != "" {
		client := api.NewClient(cfg)

		// Support short ID prefix matching
		resolvedID := askSessionFlag
		if resolved, err := session.ResolveShortID(askSessionFlag, profile); err == nil {
			resolvedID = resolved
		}

		resp, err := client.GetSession(resolvedID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resuming session: %v\n", err)
			os.Exit(1)
		}

		existingState, _ := session.Load(resolvedID)
		createdAt := time.Now()
		if existingState != nil {
			if !existingState.CreatedAt.IsZero() {
				createdAt = existingState.CreatedAt
			}
		}

		explicitState = &session.State{
			SessionID:     resp.SessionID,
			Profile:       profile,
			URL:           resp.URL,
			PresignedURLs: resp.PresignedURLs,
			CreatedAt:     createdAt,
		}
		_ = session.Save(explicitState)
		// Update the per-profile current_session pointer for the convenience
		// of subsequent invocations without -s. This is racy across parallel
		// `dx ask -s` runs, but is harmless because each running invocation
		// uses its own explicitState above and never reads the pointer.
		_ = session.SetCurrentSessionID(explicitState.SessionID, profile)
	}

	// Check for piped input
	var pipedData []byte
	stat, _ := os.Stdin.Stat()
	if (stat.Mode()&os.ModeCharDevice) == 0 && (stat.Mode()&os.ModeNamedPipe) != 0 {
		data, err := io.ReadAll(os.Stdin)
		if err == nil && len(data) > 0 {
			pipedData = data
		}
	}

	// If data was piped, upload it as a file to the session before asking
	if len(pipedData) > 0 {
		explicitState = uploadPipedData(cfg, profile, pipedData, explicitState)

		// Reopen the controlling terminal so interactive mode can read input.
		// When stdin is a pipe, it's been consumed; /dev/tty gives us the real terminal.
		tty, err := os.Open("/dev/tty")
		if err == nil {
			os.Stdin = tty
			defer tty.Close()
		}
	}

	// Determine mode: interactive or non-interactive
	if len(args) > 0 {
		question := strings.Join(args, " ")
		runNonInteractiveAsk(cfg, profile, question, explicitState)
	} else {
		runInteractiveAsk(cfg, profile, explicitState)
	}
}

// uploadPipedData uploads piped stdin data as a file to the current session.
// It ensures a session exists (creating one if needed), uploads the data to S3,
// attaches it, and sends a notification so inference knows about the file.
//
// If preState is non-nil it is used directly (no read of the per-profile
// current_session pointer file); this is required for safe parallel
// `dx ask -s SID ...` invocations. Returns the (possibly updated) state so
// callers can keep threading it through.
func uploadPipedData(cfg *config.Config, profile string, data []byte, preState *session.State) *session.State {
	logging.Debug("Uploading piped data", "size", len(data), "profile", profile)
	client := api.NewClient(cfg)

	state, _ := ensureSession(client, profile, preState, os.Stderr)

	availableURLs := state.GetAvailableURLCount()
	if availableURLs <= 0 {
		fmt.Fprintln(os.Stderr, color.Warning("Warning: No upload slots available, piped data will not be uploaded."))
		return state
	}

	startIdx := state.URLsUsed
	if startIdx > len(state.PresignedURLs) {
		startIdx = len(state.PresignedURLs)
	}
	uploader := upload.NewS3Uploader(state.PresignedURLs[startIdx:])
	if err := uploader.UploadBytes(data, "stdin.txt"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not upload piped data: %v\n", err)
		return state
	}

	// Attach and notify
	uploadedKeys := uploader.UploadedKeys()
	resp, err := client.AttachFiles(state.SessionID, uploadedKeys)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not attach piped data to session: %v\n", err)
	} else if resp.Success {
		fmt.Fprintf(statusWriter(), "✓ Uploaded piped input as %s (%s)\n",
			color.Info("stdin.txt"),
			color.Muted(formatByteSize(len(data))))

		fileList := uploader.FormatFileNamesForNotification()
		notifyMsg := fmt.Sprintf(
			"[System] 1 file uploaded to this session (piped from stdin):\n\n%s\n"+
				"This file is now available in the workspace for analysis when needed.",
			fileList,
		)
		if err := client.SendMessage(state.SessionID, notifyMsg, "", ""); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not notify about uploaded file: %v\n", err)
		}
	}

	state.URLsUsed++
	_ = session.Save(state)
	return state
}

// formatByteSize formats a byte count for display.
func formatByteSize(bytes int) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	case bytes == 1:
		return "1 byte"
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}

func runNonInteractiveAsk(cfg *config.Config, profile string, question string, preState *session.State) {
	_, span := telemetry.StartSpan(context.Background(), "dx.ask",
		attribute.String("mode", "non-interactive"),
		attribute.String("profile", profile),
	)
	defer span.End()

	client := api.NewClient(cfg)

	sw := statusWriter()

	state, isNew := ensureSession(client, profile, preState, sw)
	if isNew {
		fmt.Fprintf(sw, "Session: %s\n", state.SessionID)
		fmt.Fprintln(sw)
	}

	// Start streaming BEFORE sending message to avoid race condition
	events, errors, cancel := stream.StreamResponse(cfg.Endpoint, state.SessionID, cfg.GetAuthToken())

	// Wait for connection to be established (with 30s timeout)
	connected := false
	connectTimeout := time.After(30 * time.Second)
	for !connected {
		select {
		case event, ok := <-events:
			if !ok {
				fmt.Fprintln(os.Stderr, color.Error("✗ Connection closed unexpectedly"))
				cancel()
				os.Exit(1)
			}
			if event.Type == "connected" {
				connected = true
			}
		case err := <-errors:
			fmt.Fprintln(os.Stderr, color.Error(fmt.Sprintf("✗ Connection error: %v", err)))
			cancel()
			os.Exit(1)
		case <-connectTimeout:
			fmt.Fprintln(os.Stderr, color.Error("✗ Connection timed out after 30s"))
			cancel()
			os.Exit(1)
		}
	}

	// Now send the message (after stream is connected)
	if err := client.SendMessage(state.SessionID, question, "", ""); err != nil {
		fmt.Fprintln(os.Stderr, color.Error(fmt.Sprintf("✗ Failed to send message: %v", err)))
		cancel()
		os.Exit(1)
	}

	// Continue reading stream for response
	if isTTYOutput() {
		fmt.Println()
	}
	streamResponseWithTimeout(events, errors, cancel, askTimeoutFlag)

	state.LastMessageAt = time.Now()
	_ = session.Save(state)

	// Show view URL — goes to stderr when stdout is captured so it doesn't
	// pollute the captured answer.
	fmt.Fprintln(sw)
	fmt.Fprintf(sw, "View: %s\n", color.URL(state.URL))
}

// getHistoryFilePath returns the path to the command history file
func getHistoryFilePath() string {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "history")
}

func runInteractiveAsk(cfg *config.Config, profile string, preState *session.State) {
	_, span := telemetry.StartSpan(context.Background(), "dx.ask",
		attribute.String("mode", "interactive"),
		attribute.String("profile", profile),
	)
	defer span.End()

	client := api.NewClient(cfg)

	state, _ := ensureSession(client, profile, preState, os.Stdout)

	fmt.Printf("Session: %s\n", color.SessionID(state.SessionID))
	fmt.Printf("%s\n", color.Muted("Type your questions. Use up/down arrows for history. Press Ctrl+D to exit."))
	fmt.Println()

	// Set up liner for line editing and history
	line := liner.NewLiner()
	defer line.Close()

	// Set up multiline mode and ctrl+c handling
	line.SetCtrlCAborts(true)

	// Load history
	historyPath := getHistoryFilePath()
	if historyPath != "" {
		if f, err := os.Open(historyPath); err == nil {
			_, _ = line.ReadHistory(f)
			f.Close()
		}
	}

	// Save history on exit
	defer func() {
		if historyPath != "" {
			if f, err := os.Create(historyPath); err == nil {
				_, _ = line.WriteHistory(f)
				f.Close()
			}
		}
	}()

	for {
		question, err := line.Prompt("dx> ")
		if err != nil {
			if err == liner.ErrPromptAborted {
				fmt.Println()
				fmt.Println("Interrupted. Type 'exit' or press Ctrl+D to quit.")
				continue
			}
			if err == io.EOF {
				fmt.Println()
				break
			}
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			continue
		}

		question = strings.TrimSpace(question)
		if question == "" {
			continue
		}

		// Add to history
		line.AppendHistory(question)

		// Handle special commands
		if question == "exit" || question == "quit" {
			break
		}

		// Start streaming BEFORE sending message to avoid race condition
		events, errors, cancel := stream.StreamResponse(cfg.Endpoint, state.SessionID, cfg.GetAuthToken())

		// Wait for connection to be established (with 30s timeout)
		connected := false
		connectTimeout := time.After(30 * time.Second)
	connectLoop:
		for !connected {
			select {
			case event, ok := <-events:
				if !ok {
					fmt.Fprintln(os.Stderr, color.Error("✗ Connection lost"))
					cancel()
					break connectLoop
				}
				if event.Type == "connected" {
					connected = true
				}
			case err := <-errors:
				fmt.Fprintln(os.Stderr, color.Error(fmt.Sprintf("✗ Error: %v", err)))
				cancel()
				break connectLoop
			case <-connectTimeout:
				fmt.Fprintln(os.Stderr, color.Error("✗ Connection timed out"))
				cancel()
				break connectLoop
			}
		}
		if !connected {
			continue
		}

		// Now send the message (after stream is connected)
		if err := client.SendMessage(state.SessionID, question, "", ""); err != nil {
			fmt.Fprintln(os.Stderr, color.Error(fmt.Sprintf("✗ Failed: %v", err)))
			cancel()
			continue
		}

		// Continue reading stream for response
		fmt.Println()
		streamResponseWithTimeout(events, errors, cancel, askTimeoutFlag)
		fmt.Println()

		state.LastMessageAt = time.Now()
		_ = session.Save(state)
	}
}

// isTTYOutput returns true if stdout is a terminal (not piped).
func isTTYOutput() bool {
	if fileInfo, err := os.Stdout.Stat(); err == nil {
		return (fileInfo.Mode() & os.ModeCharDevice) != 0
	}
	return false
}

// statusWriter returns os.Stderr when stdout is not a TTY so that infrastructure
// messages (session creation, role, view URL) do not pollute captured output.
// When stdout is a TTY, messages go to stdout as usual.
func statusWriter() *os.File {
	if isTTYOutput() {
		return os.Stdout
	}
	return os.Stderr
}

// streamResponseWithTimeout reads from stream channels with an optional timeout (seconds, 0 = no limit)
func streamResponseWithTimeout(events <-chan stream.Event, errors <-chan error, cancel func(), timeoutSecs int) {
	defer cancel()

	outputState := &OutputState{
		isTTY: isTTYOutput(),
	}

	if outputState.isTTY {
		outputState.spinner = render.Thinking()
	}

	var timer *time.Timer
	var timeoutCh <-chan time.Time
	if timeoutSecs > 0 {
		timer = time.NewTimer(time.Duration(timeoutSecs) * time.Second)
		timeoutCh = timer.C
		defer timer.Stop()
	}

	for {
		select {
		case <-timeoutCh:
			fmt.Fprintln(os.Stderr, color.Error(fmt.Sprintf("\n✗ Timeout: response not completed within %d seconds", timeoutSecs)))
			return
		case event, ok := <-events:
			if !ok {
				outputState.stopSpinner()
				return
			}

			switch event.Type {
			case stream.EventConnected:
				// Already handled
			case stream.EventProgress:
				if event.Message != "" && outputState.isTTY {
					outputState.stopSpinner()
					outputState.clearStatsLine()
					formatProgressMessage(event.Message, outputState)
				}
			case stream.EventStats:
				if len(event.Stats) > 0 {
					var stats stream.AgentStats
					if err := json.Unmarshal(event.Stats, &stats); err == nil {
						outputState.stopSpinner()
						outputState.lastStats = &stats
						outputState.hadProgress = true
						outputState.printStatsLine(&stats)
					}
				}
			case stream.EventProgressReport:
				if event.Content != "" && outputState.isTTY {
					outputState.stopSpinner()
					outputState.clearStatsLine()
					formatProgressReport(event.Content, outputState)
				}
			case stream.EventAnswer:
				if event.Content != "" {
					outputState.stopSpinner()
					if !outputState.answerStarted {
						if outputState.isTTY {
							outputState.clearStatsLine()
							outputState.printFinalStats()
							if outputState.hadProgress {
								fmt.Println()
							}
							fmt.Println(color.AnswerMarker())
							fmt.Println()
						}
						outputState.answerStarted = true
					}
					outputState.answerBuffer.WriteString(event.Content)
					fmt.Print(event.Content)
				}
			case stream.EventComplete:
				outputState.stopSpinner()
				if event.Content != "" {
					if !outputState.answerStarted {
						if outputState.isTTY {
							outputState.clearStatsLine()
							outputState.printFinalStats()
							if outputState.hadProgress {
								fmt.Println()
							}
							fmt.Println(color.AnswerMarker())
							fmt.Println()
						}
					}
					outputState.answerBuffer.WriteString(event.Content)
					fmt.Print(event.Content)
				}
				fmt.Println()
				return
			case stream.EventError:
				outputState.stopSpinner()
				outputState.clearStatsLine()
				fmt.Fprintln(os.Stderr, color.Error("✗ Error: "+event.Message))
				return
			}

		case err, ok := <-errors:
			if !ok {
				return
			}
			outputState.clearStatsLine()
			fmt.Fprintln(os.Stderr, color.Error(fmt.Sprintf("✗ Stream error: %v", err)))
			return
		}
	}
}

// OutputState tracks the state of output formatting
type OutputState struct {
	lastTaskTitle  string
	hadProgress    bool
	answerStarted  bool
	inToolBlock    bool
	lastToolCallID string
	isTTY          bool
	lastStats      *stream.AgentStats
	statsLineLen   int // length of the last printed stats line (for \r overwrite)
	answerBuffer   strings.Builder
	spinner        interface{ Stop() }
}

// stopSpinner stops the thinking spinner if active.
func (s *OutputState) stopSpinner() {
	if s.spinner != nil {
		s.spinner.Stop()
		s.spinner = nil
	}
}

// printStatsLine prints a compact stats line, overwriting the previous one on TTYs.
func (s *OutputState) printStatsLine(stats *stream.AgentStats) {
	if !s.isTTY {
		return
	}

	line := formatStatsBar(stats)
	// Overwrite previous stats line
	if s.statsLineLen > 0 {
		fmt.Printf("\r%s\r", strings.Repeat(" ", s.statsLineLen))
	}
	fmt.Printf("\r%s", line)
	s.statsLineLen = visibleLen(line)
}

// clearStatsLine clears the in-place stats line so other output can print cleanly.
func (s *OutputState) clearStatsLine() {
	if s.statsLineLen > 0 && s.isTTY {
		fmt.Printf("\r%s\r", strings.Repeat(" ", s.statsLineLen))
		s.statsLineLen = 0
	}
}

// printFinalStats prints a permanent stats summary line before the answer.
func (s *OutputState) printFinalStats() {
	if s.lastStats == nil {
		return
	}
	line := formatStatsBar(s.lastStats)
	fmt.Println(line)
}

// formatStatsBar builds the compact stats summary string.
func formatStatsBar(stats *stream.AgentStats) string {
	var parts []string

	elapsed := stats.ElapsedSeconds
	if elapsed > 0 {
		parts = append(parts, fmt.Sprintf("⏱ %s", stream.FormatElapsedTime(elapsed)))
	}

	inputTokens := stats.AggregateInputTokens()
	outputTokens := stats.AggregateOutputTokens()
	totalTokens := inputTokens + outputTokens
	if totalTokens > 0 {
		parts = append(parts, fmt.Sprintf("↓%s ↑%s tokens",
			stream.FormatTokenCount(inputTokens),
			stream.FormatTokenCount(outputTokens)))
	}

	toolCalls := stats.AggregateToolCalls()
	if toolCalls > 0 {
		parts = append(parts, fmt.Sprintf("%d tool calls", toolCalls))
	}

	line := "  " + color.StatsLine(strings.Join(parts, " · "))

	if activeTask := stats.ActiveTask(); activeTask != "" {
		line += "  " + color.Progress("●") + " " + color.Muted(activeTask)
	}

	return line
}

// visibleLen approximates the visible width of a string by stripping ANSI escapes.
func visibleLen(s string) int {
	inEscape := false
	n := 0
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		n++
	}
	return n
}

// ToolCall represents a tool call message from the AI
type ToolCall struct {
	Command     string `json:"command"`
	StepID      int    `json:"step_id"`
	ToolCallID  string `json:"tool_call_id"`
	ToolName    string `json:"tool_name"`
	MessageType string `json:"message_type"`
	AssistantID string `json:"assistant_id"`
}

// ToolOutput represents a tool output message
type ToolOutput struct {
	ToolCallID    string `json:"tool_call_id"`
	ToolStatus    bool   `json:"tool_status"`
	ToolOutput    string `json:"tool_output"`
	ExitCode      int    `json:"exit_code"`
	ExecutionTime string `json:"execution_time"`
	MessageType   string `json:"message_type"`
	AssistantID   string `json:"assistant_id"`
}

// formatProgressMessage parses and formats a progress message with colors
func formatProgressMessage(message string, state *OutputState) {
	message = strings.TrimSpace(message)
	state.hadProgress = true

	// Check if it's a JSON object
	if strings.HasPrefix(message, "{") {
		// Try parsing as tool call
		var toolCall ToolCall
		if err := json.Unmarshal([]byte(message), &toolCall); err == nil && toolCall.MessageType == "tool_call" {
			formatToolCall(&toolCall, state)
			return
		}

		// Try parsing as tool output
		var toolOutput ToolOutput
		if err := json.Unmarshal([]byte(message), &toolOutput); err == nil && toolOutput.MessageType == "tool_output" {
			formatToolOutput(&toolOutput, state)
			return
		}

		// Unknown JSON, skip
		return
	}

	// Plain text - treat as task/thinking indicator
	formatTaskTitle(message, state)
}

// formatTaskTitle formats a task title (thinking/planning step)
func formatTaskTitle(title string, state *OutputState) {
	// Skip duplicate titles
	if title == state.lastTaskTitle {
		return
	}
	state.lastTaskTitle = title

	// Close any open tool block
	if state.inToolBlock {
		state.inToolBlock = false
	}

	// Print the task title with a thinking indicator
	fmt.Printf("%s %s\n", color.Progress("●"), color.Title(title))
}

// formatToolCall formats a tool call (command execution)
func formatToolCall(tc *ToolCall, state *OutputState) {
	state.lastToolCallID = tc.ToolCallID
	state.inToolBlock = true

	// Format based on tool type
	switch tc.ToolName {
	case "bash":
		// Show bash command with shell prompt style
		fmt.Printf("  %s %s\n", color.Muted("$"), color.Command(tc.Command))
	default:
		// Generic tool call
		fmt.Printf("  %s %s\n", color.ToolName(tc.ToolName+":"), color.Command(tc.Command))
	}
}

// formatToolOutput formats tool output (command result)
func formatToolOutput(to *ToolOutput, state *OutputState) {
	output := strings.TrimSpace(to.ToolOutput)

	if output == "" {
		return
	}

	// Format the output with proper indentation
	lines := strings.Split(output, "\n")

	// Limit output lines for readability (show first 10 and last 5 if too long)
	maxLines := 15
	if len(lines) > maxLines {
		// Show first 8 lines
		for i := 0; i < 8; i++ {
			fmt.Printf("    %s\n", color.ToolOutput(lines[i]))
		}
		// Show truncation indicator
		fmt.Printf("    %s\n", color.Muted(fmt.Sprintf("... (%d lines hidden) ...", len(lines)-13)))
		// Show last 5 lines
		for i := len(lines) - 5; i < len(lines); i++ {
			fmt.Printf("    %s\n", color.ToolOutput(lines[i]))
		}
	} else {
		for _, line := range lines {
			fmt.Printf("    %s\n", color.ToolOutput(line))
		}
	}

	// Show execution metadata
	if to.ExecutionTime != "" || !to.ToolStatus {
		var meta []string
		if to.ExecutionTime != "" {
			meta = append(meta, to.ExecutionTime)
		}
		if !to.ToolStatus {
			meta = append(meta, fmt.Sprintf("exit code: %d", to.ExitCode))
		}
		if len(meta) > 0 {
			fmt.Printf("    %s\n", color.Muted("("+strings.Join(meta, ", ")+")"))
		}
	}

	state.inToolBlock = false
}

// formatProgressReport formats a progress report with a left border.
func formatProgressReport(report string, state *OutputState) {
	report = strings.TrimSpace(report)
	if report == "" {
		return
	}
	state.hadProgress = true

	fmt.Println()
	lines := strings.Split(report, "\n")
	for _, line := range lines {
		fmt.Printf("  %s %s\n", color.ProgressBorder(), color.ProgressReport(line))
	}
	fmt.Println()
}
