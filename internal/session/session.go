/*
 * Copyright (c) 2023, Deductive AI, Inc. All rights reserved.
 *
 * This software is the confidential and proprietary information of
 * Deductive AI, Inc. You shall not disclose such confidential
 * information and shall use it only in accordance with the terms of
 * the license agreement you entered into with Deductive AI, Inc.
 */

package session

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/deductive-ai/dx/internal/config"
)

// State represents the session state stored in ~/.dx/sessions/<session-id>
type State struct {
	SessionID     string    `json:"session_id"`
	Profile       string    `json:"profile"`
	URL           string    `json:"url"`
	CreatedAt     time.Time `json:"created_at"`
	LastMessageAt time.Time `json:"last_message_at"`
}

const DefaultSessionTTL = 30 * time.Minute

// IsExpired returns true if the session has been idle longer than DefaultSessionTTL.
func (s *State) IsExpired() bool {
	ref := s.LastMessageAt
	if ref.IsZero() {
		ref = s.CreatedAt
	}
	return ref.Add(DefaultSessionTTL).Before(time.Now())
}

// Load reads the session state for a specific session ID
func Load(sessionID string) (*State, error) {
	sessionPath, err := config.GetSessionPath(sessionID)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(sessionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse session file: %w", err)
	}

	return &state, nil
}

// Save writes the session state to ~/.dx/sessions/<session-id>
func Save(state *State) error {
	if err := config.EnsureConfigDir(); err != nil {
		return err
	}

	sessionPath, err := config.GetSessionPath(state.SessionID)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := os.WriteFile(sessionPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// GetCurrentSessionID reads the current session ID from the per-profile pointer file
func GetCurrentSessionID(profile string) (string, error) {
	currentPath, err := config.GetProfileCurrentSessionPath(profile)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(currentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // No current session
		}
		return "", fmt.Errorf("failed to read current session file: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

// SetCurrentSessionID writes the current session ID to the per-profile pointer file
func SetCurrentSessionID(sessionID string, profile string) error {
	if err := config.EnsureProfileDir(profile); err != nil {
		return err
	}

	currentPath, err := config.GetProfileCurrentSessionPath(profile)
	if err != nil {
		return err
	}

	if err := os.WriteFile(currentPath, []byte(sessionID), 0600); err != nil {
		return fmt.Errorf("failed to write current session file: %w", err)
	}

	return nil
}

// LoadCurrent loads the current session state for a profile
func LoadCurrent(profile string) (*State, error) {
	sessionID, err := GetCurrentSessionID(profile)
	if err != nil {
		return nil, err
	}

	if sessionID == "" {
		return nil, nil // No current session
	}

	return Load(sessionID)
}

// Clear removes the current session pointer for a profile (not the session file itself)
func Clear(profile string) error {
	currentPath, err := config.GetProfileCurrentSessionPath(profile)
	if err != nil {
		return err
	}

	if err := os.Remove(currentPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove current session file: %w", err)
	}

	return nil
}

// Exists checks if a current session exists for a profile
func Exists(profile string) bool {
	state, err := LoadCurrent(profile)
	return err == nil && state != nil
}

// ListForProfile returns all stored sessions for a specific profile
func ListForProfile(profile string) ([]*State, error) {
	all, err := ListAll()
	if err != nil {
		return nil, err
	}
	var filtered []*State
	for _, s := range all {
		if s.Profile == profile {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

// ResolveShortID resolves a short session ID prefix to a full ID,
// scoped to the given profile. Returns the full ID or an error.
func ResolveShortID(prefix string, profile string) (string, error) {
	if len(prefix) >= 36 {
		state, err := Load(prefix)
		if err != nil || state == nil || state.Profile != profile {
			return "", fmt.Errorf("no session found matching '%s'", prefix)
		}
		return prefix, nil
	}
	sessions, _ := ListForProfile(profile)
	var matches []string
	for _, s := range sessions {
		if strings.HasPrefix(s.SessionID, prefix) {
			matches = append(matches, s.SessionID)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no session found matching '%s'", prefix)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("ambiguous prefix '%s' matches %d sessions", prefix, len(matches))
	}
	return matches[0], nil
}

// ListAll returns all stored sessions
func ListAll() ([]*State, error) {
	sessionsDir, err := config.GetSessionsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*State{}, nil
		}
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []*State
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Skip the current_session pointer file
		if entry.Name() == "current_session" {
			continue
		}
		
		state, err := Load(entry.Name())
		if err != nil || state == nil {
			continue
		}
		sessions = append(sessions, state)
	}

	return sessions, nil
}

// ClearAll removes session files belonging to the given profile and its current session pointer
func ClearAll(profile string) (int, error) {
	sessions, err := ListForProfile(profile)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, s := range sessions {
		sessionPath, err := config.GetSessionPath(s.SessionID)
		if err != nil {
			continue
		}
		if err := os.Remove(sessionPath); err != nil {
			continue
		}
		count++
	}

	_ = Clear(profile)

	return count, nil
}

// Delete removes a specific session file
func Delete(sessionID string, profile string) error {
	sessionPath, err := config.GetSessionPath(sessionID)
	if err != nil {
		return err
	}

	if err := os.Remove(sessionPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("session '%s' not found", sessionID)
		}
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// If this was the current session, clear the pointer
	currentID, _ := GetCurrentSessionID(profile)
	if currentID == sessionID {
		_ = Clear(profile)
	}

	return nil
}

// DeleteForProfile removes all session files belonging to the given profile.
func DeleteForProfile(profile string) error {
	sessions, err := ListForProfile(profile)
	if err != nil {
		return err
	}
	for _, s := range sessions {
		sessionPath, err := config.GetSessionPath(s.SessionID)
		if err != nil {
			continue
		}
		os.Remove(sessionPath)
	}
	_ = Clear(profile)
	return nil
}
