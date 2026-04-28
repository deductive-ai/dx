// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
  # The piped data is included as context with the question.

  lsof -i | dx ask
  # Piped data is included as context and interactive mode starts,
  # so you can ask multiple follow-up questions about the data.

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
		_, _ = fmt.Fprintf(sw, "Continuing session (%s ago)\n", formatDuration(age))
		logging.Debug("Session loaded", "id", state.SessionID, "profile", profile)
		return state, false
	}

	_, _ = fmt.Fprintln(sw, "Creating session...")
	resp, err := client.CreateSession(&api.SessionRequest{Mode: "ask"})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error creating session: %v\n", err)
		os.Exit(1)
	}
	state = &session.State{
		SessionID: resp.SessionID,
		Profile:   profile,
		URL:       resp.URL,
		CreatedAt: time.Now(),
	}
	_ = session.Save(state)
	_ = session.SetCurrentSessionID(state.SessionID, profile)
	logging.Debug("Session created", "id", resp.SessionID, "profile", profile)
	return state, true
}

// recoverSession creates a fresh session after detecting that the current one
// is unavailable (404/403/410). Returns the new state, or nil on failure.
func recoverSession(client *api.Client, profile string, sw io.Writer) *session.State {
	_ = session.Clear(profile)
	_, _ = fmt.Fprintln(sw, "Session expired, starting fresh...")
	_, _ = fmt.Fprintln(sw, "Creating session...")
	resp, err := client.CreateSession(&api.SessionRequest{Mode: "ask"})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error creating session: %v\n", err)
		return nil
	}
	state := &session.State{
		SessionID: resp.SessionID,
		Profile:   profile,
		URL:       resp.URL,
		CreatedAt: time.Now(),
	}
	_ = session.Save(state)
	_ = session.SetCurrentSessionID(state.SessionID, profile)
	logging.Debug("Session recovered", "id", resp.SessionID, "profile", profile)
	return state
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
			SessionID: resp.SessionID,
			Profile:   profile,
			URL:       resp.URL,
			CreatedAt: createdAt,
		}
		_ = session.Save(explicitState)
		// Update the per-profile current_session pointer for the convenience
		// of subsequent invocations without -s. This is racy across parallel
		// `dx ask -s` runs, but is harmless because each running invocation
		// uses its own explicitState above and never reads the pointer.
		_ = session.SetCurrentSessionID(explicitState.SessionID, profile)
	}

	// Check for piped input
	var pipedQuestion string
	stat, _ := os.Stdin.Stat()
	if (stat.Mode()&os.ModeCharDevice) == 0 && (stat.Mode()&os.ModeNamedPipe) != 0 {
		data, err := io.ReadAll(os.Stdin)
		if err == nil && len(data) > 0 {
			pipedQuestion = strings.TrimSpace(string(data))
		}
	}

	// Build question from CLI args and/or piped input
	question := strings.Join(args, " ")
	if pipedQuestion != "" {
		if question != "" {
			question = question + "\n\n" + pipedQuestion
		} else {
			question = pipedQuestion
		}
	}

	if question != "" {
		runNonInteractiveAsk(cfg, profile, question, explicitState)
	} else {
		runInteractiveAsk(cfg, profile, explicitState)
	}
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
		_, _ = fmt.Fprintf(sw, "Endpoint: %s | Session: %s\n", cfg.Endpoint, state.SessionID)
		_, _ = fmt.Fprintln(sw)
	}

	// Start streaming BEFORE sending message to avoid race condition.
	// If the session is unavailable (404/403/410), auto-recover once.
	var events <-chan stream.Event
	var streamErrors <-chan error
	var cancel func()
	connected := false
	recoveryAttempted := false

	for !connected {
		events, streamErrors, cancel = stream.StreamResponse(cfg.Endpoint, state.SessionID, cfg.GetAuthToken())
		connectTimeout := time.After(30 * time.Second)

	nonInteractiveConnect:
		for {
			select {
			case event, ok := <-events:
				if !ok {
					fmt.Fprintln(os.Stderr, color.Error("✗ Connection closed unexpectedly"))
					cancel()
					os.Exit(1)
				}
				if event.Type == "connected" {
					connected = true
					break nonInteractiveConnect
				}
			case err := <-streamErrors:
				cancel()
				var sessionErr *stream.SessionUnavailableError
				if errors.As(err, &sessionErr) && !recoveryAttempted {
					recoveryAttempted = true
					newState := recoverSession(client, profile, sw)
					if newState != nil {
						state = newState
					_, _ = fmt.Fprintf(sw, "Endpoint: %s | Session: %s\n", cfg.Endpoint, state.SessionID)
					_, _ = fmt.Fprintln(sw)
						break nonInteractiveConnect
					}
				}
				fmt.Fprintln(os.Stderr, color.Error(fmt.Sprintf("✗ Connection error: %v", err)))
				os.Exit(1)
			case <-connectTimeout:
				fmt.Fprintln(os.Stderr, color.Error("✗ Connection timed out after 30s"))
				cancel()
				os.Exit(1)
			}
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
	streamResponseWithTimeout(events, streamErrors, cancel, askTimeoutFlag)

	state.LastMessageAt = time.Now()
	truncated := truncateQuestion(question, 80)
	if state.FirstQuestion == "" {
		state.FirstQuestion = truncated
	}
	state.LastQuestion = truncated
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

	fmt.Printf("Endpoint: %s | Session: %s\n", color.Info(cfg.Endpoint), color.SessionID(state.SessionID))
	fmt.Printf("%s\n", color.Muted("Type your questions. Use /help for commands. Press Ctrl+D to exit."))
	fmt.Println()

	// Set up liner for line editing and history
	line := liner.NewLiner()
	defer func() { _ = line.Close() }()

	line.SetCtrlCAborts(true)
	line.SetWordCompleter(slashCompleter)
	line.SetTabCompletionStyle(liner.TabPrints)

	// Load history
	historyPath := getHistoryFilePath()
	if historyPath != "" {
		if f, err := os.Open(historyPath); err == nil {
			_, _ = line.ReadHistory(f)
			_ = f.Close()
		}
	}

	// Save history on exit
	defer func() {
		if historyPath != "" {
			if f, err := os.Create(historyPath); err == nil {
				_, _ = line.WriteHistory(f)
				_ = f.Close()
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

		if strings.HasPrefix(question, "/") {
			if newState := handleSlashCommand(question, cfg, state, profile, line); newState != nil {
				state = newState
				fmt.Printf("Endpoint: %s | Session: %s\n", color.Info(cfg.Endpoint), color.SessionID(state.SessionID))
				fmt.Println()
			}
			continue
		}

		// Start streaming BEFORE sending message to avoid race condition.
		// If the session is unavailable (404/403/410), auto-recover once.
		var events <-chan stream.Event
		var streamErrors <-chan error
		var cancel func()
		connected := false
		recoveryAttempted := false

	connectAttempt:
		for !connected {
			events, streamErrors, cancel = stream.StreamResponse(cfg.Endpoint, state.SessionID, cfg.GetAuthToken())
			connectTimeout := time.After(30 * time.Second)

		connectLoop:
			for {
				select {
				case event, ok := <-events:
					if !ok {
						fmt.Fprintln(os.Stderr, color.Error("✗ Connection lost"))
						cancel()
						break connectAttempt
					}
					if event.Type == "connected" {
						connected = true
						break connectLoop
					}
				case err := <-streamErrors:
					cancel()
					var sessionErr *stream.SessionUnavailableError
					if errors.As(err, &sessionErr) && !recoveryAttempted {
						recoveryAttempted = true
						newState := recoverSession(client, profile, os.Stdout)
						if newState != nil {
							state = newState
							fmt.Printf("Endpoint: %s | Session: %s\n", color.Info(cfg.Endpoint), color.SessionID(state.SessionID))
							continue connectAttempt
						}
					}
					fmt.Fprintln(os.Stderr, color.Error(fmt.Sprintf("✗ Error: %v", err)))
					break connectAttempt
				case <-connectTimeout:
					fmt.Fprintln(os.Stderr, color.Error("✗ Connection timed out"))
					cancel()
					break connectAttempt
				}
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
		streamResponseWithTimeout(events, streamErrors, cancel, askTimeoutFlag)
		fmt.Println()

		state.LastMessageAt = time.Now()
		truncated := truncateQuestion(question, 80)
		if state.FirstQuestion == "" {
			state.FirstQuestion = truncated
		}
		state.LastQuestion = truncated
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

func slashCompleter(line string, pos int) (string, []string, string) {
	if line[:pos] == "/" || (strings.HasPrefix(line[:pos], "/") && !strings.Contains(line[:pos], " ")) {
		partial := line[:pos]
		commands := []string{"/new", "/resume", "/help"}
		var matches []string
		for _, c := range commands {
			if strings.HasPrefix(c, partial) {
				matches = append(matches, c)
			}
		}
		return "", matches, line[pos:]
	}

	cmdPrefix := "/upload "
	if !strings.HasPrefix(line[:pos], cmdPrefix) {
		return line[:pos], nil, line[pos:]
	}

	partial := line[len(cmdPrefix):pos]

	var dir, namePrefix string
	if idx := strings.LastIndex(partial, "/"); idx >= 0 {
		dir = partial[:idx+1]
		namePrefix = partial[idx+1:]
	} else {
		dir = ""
		namePrefix = partial
	}

	readDir := dir
	if readDir == "" {
		readDir = "."
	}

	entries, err := os.ReadDir(readDir)
	if err != nil {
		return line[:pos], nil, line[pos:]
	}

	var candidates []string
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), namePrefix) {
			continue
		}
		name := dir + e.Name()
		if e.IsDir() {
			name += "/"
		}
		candidates = append(candidates, name)
	}

	return cmdPrefix, candidates, line[pos:]
}

func handleSlashCommand(input string, cfg *config.Config, state *session.State, profile string, line *liner.State) *session.State {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/upload":
		fmt.Println(color.Muted("File upload is not yet available. Use piped input instead:"))
		fmt.Println(color.Muted("  cat <file> | dx ask \"analyze this\""))

	case "/new":
		fmt.Println("Creating session...")
		client := api.NewClient(cfg)
		resp, err := client.CreateSession(&api.SessionRequest{Mode: "ask"})
		if err != nil {
			fmt.Printf("%s Failed to create session: %v\n", color.Error("✗"), err)
			return nil
		}
		newState := &session.State{
			SessionID: resp.SessionID,
			Profile:   profile,
			URL:       resp.URL,
			CreatedAt: time.Now(),
		}
		_ = session.Save(newState)
		_ = session.SetCurrentSessionID(newState.SessionID, profile)
		return newState

	case "/resume":
		return handleResume(cfg, state, profile, line)

	case "/help":
		fmt.Println()
		fmt.Println("  Available commands:")
		helpItems := []struct{ cmd, desc string }{
			{"/new", "Start a fresh session"},
			{"/resume", "Switch to a previous session"},
			{"/help", "Show this help"},
			{"exit", "End the session"},
		}
		const colWidth = 18
		for _, h := range helpItems {
			pad := colWidth - len(h.cmd)
			if pad < 2 {
				pad = 2
			}
			fmt.Printf("    %s%s%s\n", color.Command(h.cmd), strings.Repeat(" ", pad), h.desc)
		}
		fmt.Println()

	default:
		fmt.Printf("%s Unknown command: %s (type /help for available commands)\n", color.Error("✗"), cmd)
	}
	return nil
}

func handleResume(cfg *config.Config, currentState *session.State, profile string, line *liner.State) *session.State {
	sessions, err := session.ListForProfile(profile)
	if err != nil || len(sessions) == 0 {
		fmt.Println(color.Muted("  No previous sessions found."))
		return nil
	}

	// Sort by last activity, most recent first
	sort.Slice(sessions, func(i, j int) bool {
		ti := sessions[i].LastMessageAt
		if ti.IsZero() {
			ti = sessions[i].CreatedAt
		}
		tj := sessions[j].LastMessageAt
		if tj.IsZero() {
			tj = sessions[j].CreatedAt
		}
		return ti.After(tj)
	})

	// Filter out the current session and limit to 10
	var candidates []*session.State
	for _, s := range sessions {
		if s.SessionID == currentState.SessionID {
			continue
		}
		candidates = append(candidates, s)
		if len(candidates) >= 10 {
			break
		}
	}

	if len(candidates) == 0 {
		fmt.Println(color.Muted("  No other sessions found."))
		return nil
	}

	fmt.Println()
	fmt.Println("  Recent sessions:")
	for i, s := range candidates {
		ref := s.LastMessageAt
		if ref.IsZero() {
			ref = s.CreatedAt
		}
		age := formatDuration(time.Since(ref))

		label := s.SessionID[:8]
		if s.LastQuestion != "" {
			label = fmt.Sprintf("%q", s.LastQuestion)
		} else if s.FirstQuestion != "" {
			label = fmt.Sprintf("%q", s.FirstQuestion)
		}
		fmt.Printf("  %d. %s ago — %s\n", i+1, age, label)
	}
	fmt.Println()

	choice, err := line.Prompt(fmt.Sprintf("Pick a session (1-%d), or Enter to cancel: ", len(candidates)))
	if err != nil {
		return nil
	}
	choice = strings.TrimSpace(choice)
	if choice == "" {
		return nil
	}

	idx, err := strconv.Atoi(choice)
	if err != nil || idx < 1 || idx > len(candidates) {
		fmt.Println(color.Error("  Invalid choice."))
		return nil
	}

	picked := candidates[idx-1]

	client := api.NewClient(cfg)
	resp, err := client.GetSession(picked.SessionID)
	if err != nil {
		fmt.Printf("%s Session no longer exists on server.\n", color.Error("✗"))
		return nil
	}

	newState := &session.State{
		SessionID:     resp.SessionID,
		Profile:       profile,
		URL:           resp.URL,
		CreatedAt:     picked.CreatedAt,
		LastMessageAt: picked.LastMessageAt,
		FirstQuestion: picked.FirstQuestion,
	}
	_ = session.Save(newState)
	_ = session.SetCurrentSessionID(newState.SessionID, profile)

	fmt.Printf("%s Switched to session %s\n", color.Success("✓"), color.SessionID(newState.SessionID))
	return newState
}

func truncateQuestion(q string, maxLen int) string {
	q = strings.Split(q, "\n")[0]
	q = strings.TrimSpace(q)
	if len(q) > maxLen {
		return q[:maxLen-3] + "..."
	}
	return q
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
