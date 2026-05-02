// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
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
	"github.com/deductive-ai/dx/internal/render"
	"github.com/deductive-ai/dx/internal/session"
	"github.com/deductive-ai/dx/internal/stream"

	"github.com/spf13/cobra"
)

var askCmd = &cobra.Command{
	Use:   "ask [question]",
	Short: "Ask a question to Deductive AI",
	Long: `Ask a question to Deductive AI and receive a streaming response.

Quick answers where you expect follow-ups. Good for iterative exploration:
ask a question, get a response, refine with follow-ups. Sessions auto-resume
so conversation flows naturally.

For deep root cause analysis, use "dx investigate" instead.

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

func runAsk(cmd *cobra.Command, args []string) {
	profile := GetProfile()

	cfg := LoadOrBootstrap(profile)

	var err error
	cfg, err = EnsureAuth(cfg, profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	mode := &SessionMode{
		APIMode:  "ask",
		Persist:  true,
		ForceNew: askNewFlag,
	}

	var explicitState *session.State
	if askSessionFlag != "" {
		explicitState = resolveExplicitSession(cfg, profile, askSessionFlag)
	}

	question := buildQuestion(args)

	if question != "" {
		runNonInteractive(cfg, profile, question, explicitState, mode, askTimeoutFlag)
	} else {
		runInteractive(cfg, profile, explicitState, mode, askTimeoutFlag)
	}
}

// resolveExplicitSession resolves a --session flag value to a session state.
func resolveExplicitSession(cfg *config.Config, profile string, sessionFlag string) *session.State {
	client := api.NewClient(cfg)

	resolvedID := sessionFlag
	if resolved, err := session.ResolveShortID(sessionFlag, profile); err == nil {
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

	state := &session.State{
		SessionID: resp.SessionID,
		Profile:   profile,
		URL:       resp.URL,
		CreatedAt: createdAt,
	}
	_ = session.Save(state)
	_ = session.SetCurrentSessionID(state.SessionID, profile)
	return state
}

// buildQuestion constructs the question from CLI args and/or piped stdin.
func buildQuestion(args []string) string {
	var pipedQuestion string
	stat, _ := os.Stdin.Stat()
	if (stat.Mode()&os.ModeCharDevice) == 0 && (stat.Mode()&os.ModeNamedPipe) != 0 {
		data, err := io.ReadAll(os.Stdin)
		if err == nil && len(data) > 0 {
			pipedQuestion = strings.TrimSpace(string(data))
		}
	}

	question := strings.Join(args, " ")
	if pipedQuestion != "" {
		if question != "" {
			question = question + "\n\n" + pipedQuestion
		} else {
			question = pipedQuestion
		}
	}
	return question
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

func printSessionBanner(cfg *config.Config, state *session.State) {
	if cfg.TeamName != "" {
		fmt.Printf("Endpoint: %s | Team: %s | Session: %s\n", color.Info(cfg.Endpoint), color.Info(cfg.TeamName), color.SessionID(state.SessionID))
	} else {
		fmt.Printf("Endpoint: %s | Session: %s\n", color.Info(cfg.Endpoint), color.SessionID(state.SessionID))
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

	const idleTimeoutSecs = 60
	idleTimer := time.NewTimer(time.Duration(idleTimeoutSecs) * time.Second)
	defer idleTimer.Stop()

	for {
		select {
		case <-timeoutCh:
			fmt.Fprintln(os.Stderr, color.Error(fmt.Sprintf("\n✗ Timeout: response not completed within %d seconds", timeoutSecs)))
			return
		case <-idleTimer.C:
			if outputState.answerStarted {
				outputState.stopSpinner()
				fmt.Fprintln(os.Stderr)
				return
			}
			idleTimer.Reset(time.Duration(idleTimeoutSecs) * time.Second)
		case event, ok := <-events:
			if !ok {
				outputState.stopSpinner()
				return
			}
			idleTimer.Reset(time.Duration(idleTimeoutSecs) * time.Second)

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
	statsLineLen   int
	answerBuffer   strings.Builder
	spinner        interface{ Stop() }
}

func (s *OutputState) stopSpinner() {
	if s.spinner != nil {
		s.spinner.Stop()
		s.spinner = nil
	}
}

func (s *OutputState) printStatsLine(stats *stream.AgentStats) {
	if !s.isTTY {
		return
	}

	line := formatStatsBar(stats)
	if s.statsLineLen > 0 {
		fmt.Printf("\r%s\r", strings.Repeat(" ", s.statsLineLen))
	}
	fmt.Printf("\r%s", line)
	s.statsLineLen = visibleLen(line)
}

func (s *OutputState) clearStatsLine() {
	if s.statsLineLen > 0 && s.isTTY {
		fmt.Printf("\r%s\r", strings.Repeat(" ", s.statsLineLen))
		s.statsLineLen = 0
	}
}

func (s *OutputState) printFinalStats() {
	if s.lastStats == nil {
		return
	}
	line := formatStatsBar(s.lastStats)
	fmt.Println(line)
}

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

type ToolCall struct {
	Command     string `json:"command"`
	StepID      int    `json:"step_id"`
	ToolCallID  string `json:"tool_call_id"`
	ToolName    string `json:"tool_name"`
	MessageType string `json:"message_type"`
	AssistantID string `json:"assistant_id"`
}

type ToolOutput struct {
	ToolCallID    string `json:"tool_call_id"`
	ToolStatus    bool   `json:"tool_status"`
	ToolOutput    string `json:"tool_output"`
	ExitCode      int    `json:"exit_code"`
	ExecutionTime string `json:"execution_time"`
	MessageType   string `json:"message_type"`
	AssistantID   string `json:"assistant_id"`
}

func formatProgressMessage(message string, state *OutputState) {
	message = strings.TrimSpace(message)
	state.hadProgress = true

	if strings.HasPrefix(message, "{") {
		var toolCall ToolCall
		if err := json.Unmarshal([]byte(message), &toolCall); err == nil && toolCall.MessageType == "tool_call" {
			formatToolCall(&toolCall, state)
			return
		}

		var toolOutput ToolOutput
		if err := json.Unmarshal([]byte(message), &toolOutput); err == nil && toolOutput.MessageType == "tool_output" {
			formatToolOutput(&toolOutput, state)
			return
		}

		return
	}

	formatTaskTitle(message, state)
}

func formatTaskTitle(title string, state *OutputState) {
	if title == state.lastTaskTitle {
		return
	}
	state.lastTaskTitle = title

	if state.inToolBlock {
		state.inToolBlock = false
	}

	fmt.Printf("%s %s\n", color.Progress("●"), color.Title(title))
}

func formatToolCall(tc *ToolCall, state *OutputState) {
	state.lastToolCallID = tc.ToolCallID
	state.inToolBlock = true

	switch tc.ToolName {
	case "bash":
		fmt.Printf("  %s %s\n", color.Muted("$"), color.Command(tc.Command))
	default:
		fmt.Printf("  %s %s\n", color.ToolName(tc.ToolName+":"), color.Command(tc.Command))
	}
}

func formatToolOutput(to *ToolOutput, state *OutputState) {
	output := strings.TrimSpace(to.ToolOutput)

	if output == "" {
		return
	}

	lines := strings.Split(output, "\n")

	maxLines := 15
	if len(lines) > maxLines {
		for i := 0; i < 8; i++ {
			fmt.Printf("    %s\n", color.ToolOutput(lines[i]))
		}
		fmt.Printf("    %s\n", color.Muted(fmt.Sprintf("... (%d lines hidden) ...", len(lines)-13)))
		for i := len(lines) - 5; i < len(lines); i++ {
			fmt.Printf("    %s\n", color.ToolOutput(lines[i]))
		}
	} else {
		for _, line := range lines {
			fmt.Printf("    %s\n", color.ToolOutput(line))
		}
	}

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

func handleResume(cfg *config.Config, currentState *session.State, profile string, line interface{ Prompt(string) (string, error) }) *session.State {
	sessions, err := session.ListForProfile(profile)
	if err != nil || len(sessions) == 0 {
		fmt.Println(color.Muted("  No previous sessions found."))
		return nil
	}

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

// getHistoryFilePath returns the path to the command history file
func getHistoryFilePath() string {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "history")
}
