/*
 * Copyright (c) 2023, Deductive AI, Inc. All rights reserved.
 *
 * This software is the confidential and proprietary information of
 * Deductive AI, Inc. You shall not disclose such confidential
 * information and shall use it only in accordance with the terms of
 * the license agreement you entered into with Deductive AI, Inc.
 */

package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/deductive-ai/dx/internal/crypto"
)

func validateProfileName(profile string) error {
	if profile == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	if profile == "." || profile == ".." {
		return fmt.Errorf("invalid profile name: %s", profile)
	}
	if strings.ContainsAny(profile, "/\\") || filepath.IsAbs(profile) {
		return fmt.Errorf("invalid profile name: %s", profile)
	}
	return nil
}

const (
	ConfigDirName      = ".dx"
	ProfilesDirName    = "profiles"
	SessionsDirName    = "sessions"
	ConfigFileName     = "config"
	CurrentSessionFile = "current_session"
	ActiveProfileFile  = "active_profile"
	DefaultProfile     = "default"
)

// Config represents a profile's configuration stored in ~/.dx/profiles/<profile>/config
type Config struct {
	Endpoint          string    `toml:"endpoint"`
	AuthMethod        string    `toml:"auth_method,omitempty"`
	OAuthAccessToken  string    `toml:"oauth_access_token,omitempty"`
	OAuthRefreshToken string    `toml:"oauth_refresh_token,omitempty"`
	OAuthExpiresAt    time.Time `toml:"oauth_expires_at,omitempty"`
	APIKey            string    `toml:"api_key,omitempty"`
	EncryptedAPIKey   string    `toml:"encrypted_api_key,omitempty"`
	TeamID            string    `toml:"team_id,omitempty"`
	Editor            string    `toml:"editor,omitempty"`
}

// Auth represents authentication configuration
type Auth struct {
	Method       string    `toml:"method"`
	AccessToken  string    `toml:"access_token,omitempty"`
	RefreshToken string    `toml:"refresh_token,omitempty"`
	ExpiresAt    time.Time `toml:"expires_at,omitempty"`
	APIKey       string    `toml:"api_key,omitempty"`
	TeamID       string    `toml:"team_id,omitempty"`
}

// GetConfigDir returns the base config directory path (~/.dx/)
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ConfigDirName), nil
}

// GetProfilesDir returns the profiles directory path (~/.dx/profiles/)
func GetProfilesDir() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, ProfilesDirName), nil
}

// GetProfileDir returns the directory path for a specific profile (~/.dx/profiles/<profile>/)
func GetProfileDir(profile string) (string, error) {
	profilesDir, err := GetProfilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(profilesDir, profile), nil
}

// GetProfileConfigPath returns the config file path for a specific profile
func GetProfileConfigPath(profile string) (string, error) {
	profileDir, err := GetProfileDir(profile)
	if err != nil {
		return "", err
	}
	return filepath.Join(profileDir, ConfigFileName), nil
}

// GetSessionsDir returns the sessions directory path (~/.dx/sessions/)
func GetSessionsDir() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, SessionsDirName), nil
}

// GetSessionPath returns the path for a specific session file
func GetSessionPath(sessionID string) (string, error) {
	sessionsDir, err := GetSessionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(sessionsDir, sessionID), nil
}

// GetCurrentSessionPath returns the path to the current session pointer file (deprecated, use GetProfileCurrentSessionPath)
func GetCurrentSessionPath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, CurrentSessionFile), nil
}

// GetProfileCurrentSessionPath returns the per-profile current session pointer path (~/.dx/profiles/<profile>/current_session)
func GetProfileCurrentSessionPath(profile string) (string, error) {
	profileDir, err := GetProfileDir(profile)
	if err != nil {
		return "", err
	}
	return filepath.Join(profileDir, CurrentSessionFile), nil
}

// ReadActiveProfile reads the active profile name from ~/.dx/active_profile.
func ReadActiveProfile() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(configDir, ActiveProfileFile))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteActiveProfile writes the active profile name to ~/.dx/active_profile.
func WriteActiveProfile(profile string) error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, ActiveProfileFile), []byte(profile+"\n"), 0644)
}

// EnsureConfigDir creates the config directory structure if it doesn't exist
func EnsureConfigDir() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}

	profilesDir, err := GetProfilesDir()
	if err != nil {
		return err
	}

	sessionsDir, err := GetSessionsDir()
	if err != nil {
		return err
	}

	// Create directories
	for _, dir := range []string{configDir, profilesDir, sessionsDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// EnsureProfileDir creates the profile directory if it doesn't exist
func EnsureProfileDir(profile string) error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}

	profileDir, err := GetProfileDir(profile)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(profileDir, 0700); err != nil {
		return fmt.Errorf("failed to create profile directory: %w", err)
	}

	return nil
}

// Load reads the configuration for a specific profile
func Load(profile string) (*Config, error) {
	configPath, err := GetProfileConfigPath(profile)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("profile '%s' not found. Run 'dx config --profile=%s' first", profile, profile)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if cfg.EncryptedAPIKey != "" {
		decrypted, err := crypto.Decrypt(cfg.EncryptedAPIKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt API key: %w", err)
		}
		cfg.APIKey = decrypted
	}

	return &cfg, nil
}

// Save writes the configuration for a specific profile
func Save(cfg *Config, profile string) error {
	if err := EnsureProfileDir(profile); err != nil {
		return err
	}

	configPath, err := GetProfileConfigPath(profile)
	if err != nil {
		return err
	}

	originalAPIKey := cfg.APIKey
	if cfg.APIKey != "" {
		encrypted, err := crypto.Encrypt(cfg.APIKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt API key: %w", err)
		}
		cfg.EncryptedAPIKey = encrypted
		cfg.APIKey = ""
	} else {
		cfg.EncryptedAPIKey = ""
	}
	defer func() { cfg.APIKey = originalAPIKey }()

	var buf bytes.Buffer
	encoder := toml.NewEncoder(&buf)
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// SaveAuth updates only the authentication fields in the config for a profile
func SaveAuth(auth *Auth, profile string) error {
	cfg := &Config{}
	if ProfileExists(profile) {
		loaded, err := Load(profile)
		if err != nil {
			return fmt.Errorf("failed to load existing config: %w", err)
		}
		cfg = loaded
	}

	cfg.AuthMethod = auth.Method
	cfg.TeamID = auth.TeamID

	if auth.Method == "oauth" {
		cfg.OAuthAccessToken = auth.AccessToken
		cfg.OAuthRefreshToken = auth.RefreshToken
		cfg.OAuthExpiresAt = auth.ExpiresAt
		cfg.APIKey = ""
	} else if auth.Method == "apikey" {
		cfg.APIKey = auth.APIKey
		cfg.OAuthAccessToken = ""
		cfg.OAuthRefreshToken = ""
		cfg.OAuthExpiresAt = time.Time{}
	}

	return Save(cfg, profile)
}

// GetAuthToken returns the appropriate auth token based on auth method
func (c *Config) GetAuthToken() string {
	if c.AuthMethod == "apikey" {
		return c.APIKey
	}
	return c.OAuthAccessToken
}

// IsAuthenticated checks if the config has valid authentication
func (c *Config) IsAuthenticated() bool {
	if c.AuthMethod == "apikey" {
		return c.APIKey != ""
	}
	if c.AuthMethod == "oauth" {
		return c.OAuthAccessToken != "" && time.Now().Before(c.OAuthExpiresAt)
	}
	return false
}

// CanRefresh returns true if the config has a refresh token that can be used
// to obtain a new access token when the current one has expired.
func (c *Config) CanRefresh() bool {
	return c.AuthMethod == "oauth" && c.OAuthRefreshToken != "" && c.Endpoint != ""
}

// ProfileExists checks if a profile exists
func ProfileExists(profile string) bool {
	configPath, err := GetProfileConfigPath(profile)
	if err != nil {
		return false
	}
	_, err = os.Stat(configPath)
	return err == nil
}

// ListProfiles returns all available profile names
func ListProfiles() ([]string, error) {
	profilesDir, err := GetProfilesDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read profiles directory: %w", err)
	}

	var profiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if it has a config file
			configPath := filepath.Join(profilesDir, entry.Name(), ConfigFileName)
			if _, err := os.Stat(configPath); err == nil {
				profiles = append(profiles, entry.Name())
			}
		}
	}

	return profiles, nil
}

// GetEditor returns the configured editor or falls back to environment/defaults
func (c *Config) GetEditor() string {
	if c.Editor != "" {
		return c.Editor
	}
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if visual := os.Getenv("VISUAL"); visual != "" {
		return visual
	}
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	return "vim"
}

// Deprecated compatibility functions - these are kept for backward compatibility
// but delegate to the new profile-based system using the default profile

// GetConfigPath returns the config path for the default profile (deprecated)
func GetConfigPath() (string, error) {
	return GetProfileConfigPath(DefaultProfile)
}

// DeleteProfile removes the profile directory
func DeleteProfile(profile string) error {
	if err := validateProfileName(profile); err != nil {
		return err
	}

	profileDir, err := GetProfileDir(profile)
	if err != nil {
		return err
	}

	if _, err := os.Stat(profileDir); os.IsNotExist(err) {
		return fmt.Errorf("profile '%s' not found", profile)
	}

	if err := os.RemoveAll(profileDir); err != nil {
		return fmt.Errorf("failed to delete profile directory: %w", err)
	}

	return nil
}

// Exists checks if the default profile exists (deprecated)
func Exists() bool {
	return ProfileExists(DefaultProfile)
}
