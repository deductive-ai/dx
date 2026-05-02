// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/logging"
	"github.com/deductive-ai/dx/internal/session"
	"github.com/deductive-ai/dx/internal/stream"
	"github.com/deductive-ai/dx/internal/telemetry"

	"github.com/peterh/liner"
	"go.opentelemetry.io/otel/attribute"
)

// SessionMode controls session lifecycle behavior for different commands.
type SessionMode struct {
	// APIMode is sent to the server as the "mode" field (e.g. "ask", "investigate").
	APIMode string
	// Persist controls whether sessions are saved to disk and auto-resumed.
	// When false, sessions are ephemeral (created fresh each run, not saved).
	Persist bool
	// ForceNew skips session resume even when Persist is true.
	ForceNew bool
}

// createNewSession creates a fresh session via the API and optionally persists it.
func createNewSession(client *api.Client, profile string, mode *SessionMode, sw io.Writer) *session.State {
	_, _ = fmt.Fprintln(sw, "Creating session...")
	resp, err := client.CreateSession(&api.SessionRequest{Mode: mode.APIMode})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error creating session: %v\n", err)
		os.Exit(1)
	}
	state := &session.State{
		SessionID: resp.SessionID,
		Profile:   profile,
		URL:       resp.URL,
		CreatedAt: time.Now(),
	}
	if mode.Persist {
		_ = session.Save(state)
		_ = session.SetCurrentSessionID(state.SessionID, profile)
	}
	logging.Debug("Session created", "id", resp.SessionID, "profile", profile, "mode", mode.APIMode)
	return state
}

// ensureSessionForMode loads an existing session or creates a new one based on mode.
// For persistent modes (ask), it auto-resumes. For ephemeral modes (investigate), it always creates fresh.
func ensureSessionForMode(client *api.Client, profile string, preState *session.State, mode *SessionMode, sw io.Writer) (*session.State, bool) {
	// If an explicit session was provided (--session flag), use it directly
	if preState != nil && preState.Profile == profile {
		return preState, false
	}

	// Ephemeral modes always create fresh
	if !mode.Persist {
		return createNewSession(client, profile, mode, sw), true
	}

	// Persistent mode: try to resume
	state := preState
	if state == nil || state.Profile != profile {
		var err error
		state, err = session.LoadCurrent(profile)
		if err != nil {
			state = nil
		}
	}

	reusing := state != nil && state.Profile == profile && !state.IsExpired() && !mode.ForceNew

	if reusing {
		age := time.Since(state.LastMessageAt)
		if state.LastMessageAt.IsZero() {
			age = time.Since(state.CreatedAt)
		}
		if state.LastQuestion != "" {
			_, _ = fmt.Fprintf(sw, "Continuing session (%s ago) — %q\nUse --new for a fresh start\n", formatDuration(age), state.LastQuestion)
		} else {
			_, _ = fmt.Fprintf(sw, "Continuing session (%s ago) — use --new for a fresh start\n", formatDuration(age))
		}
		logging.Debug("Session loaded", "id", state.SessionID, "profile", profile)
		return state, false
	}

	return createNewSession(client, profile, mode, sw), true
}

// recoverSessionForMode creates a fresh session after detecting that the current one
// is unavailable (404/403/410). Returns the new state, or nil on failure.
func recoverSessionForMode(client *api.Client, profile string, mode *SessionMode, sw io.Writer) *session.State {
	if mode.Persist {
		_ = session.Clear(profile)
	}
	cfg, _ := config.Load(profile)
	if cfg != nil && cfg.TeamName != "" {
		_, _ = fmt.Fprintf(sw, "Session expired for %s, starting fresh...\n", cfg.TeamName)
	} else {
		_, _ = fmt.Fprintln(sw, "Session expired, starting fresh...")
	}
	return createNewSession(client, profile, mode, sw)
}

// connectAndSend establishes a stream connection and sends a message, handling
// session recovery on unavailable sessions. Returns the event/error channels,
// cancel func, and updated state (which may differ if recovery occurred).
// Returns connected=false if connection failed.
func connectAndSend(
	cfg *config.Config, client *api.Client, state *session.State,
	profile string, question string, mode *SessionMode, sw io.Writer,
	recoveryAttempted bool,
) (
	events <-chan stream.Event, streamErrors <-chan error, cancel func(),
	newState *session.State, connected bool, recovered bool,
) {
	newState = state
	for {
		events, streamErrors, cancel = stream.StreamResponse(cfg.Endpoint, newState.SessionID, cfg.GetAuthToken(), cfg.TeamID)
		connectTimeout := time.After(30 * time.Second)

	connectLoop:
		for {
			select {
			case event, ok := <-events:
				if !ok {
					return nil, nil, cancel, newState, false, recoveryAttempted
				}
				if event.Type == "connected" {
					// Send the message now that we're connected
					if err := client.SendMessage(newState.SessionID, question, "", ""); err != nil {
						_, _ = fmt.Fprintln(os.Stderr, color.Error(fmt.Sprintf("✗ Failed to send message: %v", err)))
						cancel()
						return nil, nil, nil, newState, false, recoveryAttempted
					}
					return events, streamErrors, cancel, newState, true, recoveryAttempted
				}
			case err := <-streamErrors:
				cancel()
				var sessionErr *stream.SessionUnavailableError
				if errors.As(err, &sessionErr) && !recoveryAttempted {
					recoveryAttempted = true
					recovered := recoverSessionForMode(client, profile, mode, sw)
					if recovered != nil {
						newState = recovered
						printSessionBanner(cfg, newState)
						_, _ = fmt.Fprintln(sw)
						break connectLoop
					}
				}
				_, _ = fmt.Fprintln(os.Stderr, color.Error(fmt.Sprintf("✗ Connection error: %v", err)))
				return nil, nil, nil, newState, false, recoveryAttempted
			case <-connectTimeout:
				_, _ = fmt.Fprintln(os.Stderr, color.Error("✗ Connection timed out after 30s"))
				cancel()
				return nil, nil, nil, newState, false, recoveryAttempted
			}
		}
	}
}

// saveSessionState persists session state if the mode allows it.
func saveSessionState(state *session.State, question string, mode *SessionMode) {
	state.LastMessageAt = time.Now()
	truncated := truncateQuestion(question, 80)
	if state.FirstQuestion == "" {
		state.FirstQuestion = truncated
	}
	state.LastQuestion = truncated
	if mode.Persist {
		_ = session.Save(state)
	}
}

// runNonInteractive handles the one-shot question flow for any mode.
func runNonInteractive(cfg *config.Config, profile string, question string, preState *session.State, mode *SessionMode, timeoutSecs int) {
	_, span := telemetry.StartSpan(context.Background(), "dx."+mode.APIMode,
		attribute.String("mode", "non-interactive"),
		attribute.String("profile", profile),
	)
	defer span.End()

	client := api.NewClient(cfg)
	sw := statusWriter()

	state, isNew := ensureSessionForMode(client, profile, preState, mode, sw)
	if isNew {
		printSessionBannerTo(cfg, state, sw)
		_, _ = fmt.Fprintln(sw)
	}

	events, streamErrors, cancel, state, connected, _ := connectAndSend(cfg, client, state, profile, question, mode, sw, false)
	if !connected {
		os.Exit(1)
	}

	if isTTYOutput() {
		fmt.Println()
	}
	streamResponseWithTimeout(events, streamErrors, cancel, timeoutSecs)

	saveSessionState(state, question, mode)

	_, _ = fmt.Fprintln(sw)
	_, _ = fmt.Fprintf(sw, "View: %s\n", color.URL(state.URL))
}

// runInteractive handles the multi-turn REPL for any mode.
func runInteractive(cfg *config.Config, profile string, preState *session.State, mode *SessionMode, timeoutSecs int) {
	_, span := telemetry.StartSpan(context.Background(), "dx."+mode.APIMode,
		attribute.String("mode", "interactive"),
		attribute.String("profile", profile),
	)
	defer span.End()

	client := api.NewClient(cfg)

	state, _ := ensureSessionForMode(client, profile, preState, mode, os.Stdout)

	printSessionBanner(cfg, state)
	fmt.Printf("%s\n", color.Muted("Type your questions. Use /help for commands. Press Ctrl+D to exit."))
	fmt.Println()

	line := liner.NewLiner()
	defer func() { _ = line.Close() }()

	line.SetCtrlCAborts(true)
	line.SetWordCompleter(slashCompleter)
	line.SetTabCompletionStyle(liner.TabPrints)

	historyPath := getHistoryFilePath()
	if historyPath != "" {
		if f, err := os.Open(historyPath); err == nil {
			_, _ = line.ReadHistory(f)
			_ = f.Close()
		}
	}
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

		line.AppendHistory(question)

		if question == "exit" || question == "quit" {
			break
		}

		if strings.HasPrefix(question, "/") {
			if newState := handleSlashCommandForMode(question, cfg, state, profile, line, mode); newState != nil {
				state = newState
				printSessionBanner(cfg, state)
				fmt.Println()
			}
			continue
		}

		events, streamErrors, cancel, newState, connected, _ := connectAndSend(cfg, client, state, profile, question, mode, os.Stdout, false)
		if !connected {
			continue
		}
		state = newState

		fmt.Println()
		streamResponseWithTimeout(events, streamErrors, cancel, timeoutSecs)
		fmt.Println()

		saveSessionState(state, question, mode)
	}
}

// handleSlashCommandForMode handles slash commands, using the mode for /new.
func handleSlashCommandForMode(input string, cfg *config.Config, state *session.State, profile string, line *liner.State, mode *SessionMode) *session.State {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/upload":
		fmt.Println(color.Muted("File upload is not yet available. Use piped input instead:"))
		fmt.Println(color.Muted("  cat <file> | dx ask \"analyze this\""))

	case "/new":
		client := api.NewClient(cfg)
		return createNewSession(client, profile, mode, os.Stdout)

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

// printSessionBannerTo writes the session banner to a specific writer.
func printSessionBannerTo(cfg *config.Config, state *session.State, w io.Writer) {
	if cfg.TeamName != "" {
		_, _ = fmt.Fprintf(w, "Endpoint: %s | Team: %s | Session: %s\n", cfg.Endpoint, cfg.TeamName, state.SessionID)
	} else {
		_, _ = fmt.Fprintf(w, "Endpoint: %s | Session: %s\n", cfg.Endpoint, state.SessionID)
	}
}
