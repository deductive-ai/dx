package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deductive-ai/dx/internal/crypto"
)

// setTestHome overrides HOME so all config operations use a temp directory.
// Returns the temp dir path and a cleanup function.
func setTestHome(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	return tmpDir
}

func TestEnsureConfigDir_CreatesStructure(t *testing.T) {
	home := setTestHome(t)

	if err := EnsureConfigDir(); err != nil {
		t.Fatalf("EnsureConfigDir() error: %v", err)
	}

	for _, sub := range []string{ConfigDirName, filepath.Join(ConfigDirName, ProfilesDirName), filepath.Join(ConfigDirName, SessionsDirName)} {
		dir := filepath.Join(home, sub)
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", dir, err)
		} else if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
	}
}

func TestEnsureConfigDir_Idempotent(t *testing.T) {
	setTestHome(t)

	if err := EnsureConfigDir(); err != nil {
		t.Fatalf("first EnsureConfigDir() error: %v", err)
	}
	if err := EnsureConfigDir(); err != nil {
		t.Fatalf("second EnsureConfigDir() error: %v", err)
	}
}

func TestEnsureProfileDir_CreatesProfileDir(t *testing.T) {
	home := setTestHome(t)

	if err := EnsureProfileDir("staging"); err != nil {
		t.Fatalf("EnsureProfileDir() error: %v", err)
	}

	dir := filepath.Join(home, ConfigDirName, ProfilesDirName, "staging")
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("expected profile dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("profile dir is not a directory")
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	setTestHome(t)

	original := &Config{
		Endpoint:   "https://example.com",
		AuthMethod: "apikey",
		APIKey:     "sk-test-key-12345",
		TeamID:     "team_abc",
		Editor:     "nano",
	}

	if err := Save(original, "default"); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load("default")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.Endpoint != original.Endpoint {
		t.Errorf("Endpoint = %q, want %q", loaded.Endpoint, original.Endpoint)
	}
	if loaded.AuthMethod != original.AuthMethod {
		t.Errorf("AuthMethod = %q, want %q", loaded.AuthMethod, original.AuthMethod)
	}
	if loaded.APIKey != original.APIKey {
		t.Errorf("APIKey = %q, want %q", loaded.APIKey, original.APIKey)
	}
	if loaded.TeamID != original.TeamID {
		t.Errorf("TeamID = %q, want %q", loaded.TeamID, original.TeamID)
	}
	if loaded.Editor != original.Editor {
		t.Errorf("Editor = %q, want %q", loaded.Editor, original.Editor)
	}
}

func TestSaveLoad_OAuthRoundTrip(t *testing.T) {
	setTestHome(t)

	expires := time.Now().Add(1 * time.Hour).Truncate(time.Second)
	original := &Config{
		Endpoint:          "https://example.com",
		AuthMethod:        "oauth",
		OAuthAccessToken:  "access-token-123",
		OAuthRefreshToken: "refresh-token-456",
		OAuthExpiresAt:    expires,
	}

	if err := Save(original, "oauth-profile"); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load("oauth-profile")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.OAuthAccessToken != original.OAuthAccessToken {
		t.Errorf("OAuthAccessToken = %q, want %q", loaded.OAuthAccessToken, original.OAuthAccessToken)
	}
	if loaded.OAuthRefreshToken != original.OAuthRefreshToken {
		t.Errorf("OAuthRefreshToken = %q, want %q", loaded.OAuthRefreshToken, original.OAuthRefreshToken)
	}
}

func TestLoad_NonExistentProfile(t *testing.T) {
	setTestHome(t)
	_ = EnsureConfigDir()

	_, err := Load("nonexistent")
	if err == nil {
		t.Fatal("Load() should return error for non-existent profile")
	}
}

func TestAPIKeyEncryption_OnDisk(t *testing.T) {
	home := setTestHome(t)

	cfg := &Config{
		Endpoint:   "https://example.com",
		AuthMethod: "apikey",
		APIKey:     "sk-secret-key",
	}

	if err := Save(cfg, "default"); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Read the raw file to verify API key is not stored in plaintext
	configPath := filepath.Join(home, ConfigDirName, ProfilesDirName, "default", ConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	contents := string(data)
	if containsSubstring(contents, "sk-secret-key") {
		t.Error("config file contains plaintext API key")
	}
	if !containsSubstring(contents, "encrypted_api_key") {
		t.Error("config file does not contain encrypted_api_key field")
	}

	// Verify Load decrypts correctly
	loaded, err := Load("default")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.APIKey != "sk-secret-key" {
		t.Errorf("decrypted APIKey = %q, want %q", loaded.APIKey, "sk-secret-key")
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSave_RestoresAPIKeyOnStruct(t *testing.T) {
	setTestHome(t)

	cfg := &Config{
		Endpoint:   "https://example.com",
		AuthMethod: "apikey",
		APIKey:     "sk-test",
	}

	if err := Save(cfg, "default"); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	if cfg.APIKey != "sk-test" {
		t.Errorf("Save() should restore APIKey on the struct; got %q", cfg.APIKey)
	}
}

func TestProfileExists(t *testing.T) {
	setTestHome(t)

	if ProfileExists("default") {
		t.Error("ProfileExists() should return false before profile is created")
	}

	cfg := &Config{Endpoint: "https://example.com"}
	_ = Save(cfg, "default")

	if !ProfileExists("default") {
		t.Error("ProfileExists() should return true after profile is created")
	}

	if ProfileExists("other") {
		t.Error("ProfileExists() should return false for non-existent profile")
	}
}

func TestListProfiles(t *testing.T) {
	setTestHome(t)
	_ = EnsureConfigDir()

	// No profiles yet
	profiles, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles() error: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("ListProfiles() returned %d profiles, want 0", len(profiles))
	}

	// Create profiles
	_ = Save(&Config{Endpoint: "https://a.com"}, "alpha")
	_ = Save(&Config{Endpoint: "https://b.com"}, "beta")

	profiles, err = ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles() error: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("ListProfiles() returned %d profiles, want 2", len(profiles))
	}

	found := map[string]bool{}
	for _, p := range profiles {
		found[p] = true
	}
	if !found["alpha"] || !found["beta"] {
		t.Errorf("ListProfiles() = %v, want [alpha, beta]", profiles)
	}
}

func TestListProfiles_IgnoresDirsWithoutConfig(t *testing.T) {
	home := setTestHome(t)
	_ = EnsureConfigDir()

	// Create a profile dir without a config file
	emptyDir := filepath.Join(home, ConfigDirName, ProfilesDirName, "empty")
	_ = os.MkdirAll(emptyDir, 0700)

	// Create a real profile
	_ = Save(&Config{Endpoint: "https://a.com"}, "real")

	profiles, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles() error: %v", err)
	}
	if len(profiles) != 1 || profiles[0] != "real" {
		t.Errorf("ListProfiles() = %v, want [real]", profiles)
	}
}

func TestDeleteProfile(t *testing.T) {
	home := setTestHome(t)

	_ = Save(&Config{Endpoint: "https://a.com"}, "todelete")

	dir := filepath.Join(home, ConfigDirName, ProfilesDirName, "todelete")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("profile dir should exist before delete: %v", err)
	}

	if err := DeleteProfile("todelete"); err != nil {
		t.Fatalf("DeleteProfile() error: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("profile dir should not exist after delete")
	}
}

func TestDeleteProfile_NonExistent(t *testing.T) {
	setTestHome(t)
	_ = EnsureConfigDir()

	err := DeleteProfile("ghost")
	if err == nil {
		t.Error("DeleteProfile() should return error for non-existent profile")
	}
}

func TestGetEditor_Fallback(t *testing.T) {
	tests := []struct {
		name       string
		cfgEditor  string
		envEditor  string
		envVisual  string
		wantEditor string
	}{
		{"config editor wins", "code", "nano", "emacs", "code"},
		{"EDITOR fallback", "", "nano", "emacs", "nano"},
		{"VISUAL fallback", "", "", "emacs", "emacs"},
		{"default vim", "", "", "", "vim"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EDITOR", tt.envEditor)
			t.Setenv("VISUAL", tt.envVisual)

			cfg := &Config{Editor: tt.cfgEditor}
			got := cfg.GetEditor()
			if got != tt.wantEditor {
				t.Errorf("GetEditor() = %q, want %q", got, tt.wantEditor)
			}
		})
	}
}

func TestIsAuthenticated(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			"apikey with key",
			Config{AuthMethod: "apikey", APIKey: "sk-123"},
			true,
		},
		{
			"apikey without key",
			Config{AuthMethod: "apikey"},
			false,
		},
		{
			"oauth valid",
			Config{AuthMethod: "oauth", OAuthAccessToken: "tok", OAuthExpiresAt: time.Now().Add(1 * time.Hour)},
			true,
		},
		{
			"oauth expired",
			Config{AuthMethod: "oauth", OAuthAccessToken: "tok", OAuthExpiresAt: time.Now().Add(-1 * time.Hour)},
			false,
		},
		{
			"oauth no token",
			Config{AuthMethod: "oauth", OAuthExpiresAt: time.Now().Add(1 * time.Hour)},
			false,
		},
		{
			"unknown method",
			Config{AuthMethod: "magic"},
			false,
		},
		{
			"empty config",
			Config{},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.IsAuthenticated()
			if got != tt.want {
				t.Errorf("IsAuthenticated() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanRefresh(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			"oauth with refresh and endpoint",
			Config{AuthMethod: "oauth", OAuthRefreshToken: "rt-123", Endpoint: "https://example.com"},
			true,
		},
		{
			"oauth without refresh token",
			Config{AuthMethod: "oauth", Endpoint: "https://example.com"},
			false,
		},
		{
			"oauth without endpoint",
			Config{AuthMethod: "oauth", OAuthRefreshToken: "rt-123"},
			false,
		},
		{
			"apikey method",
			Config{AuthMethod: "apikey", OAuthRefreshToken: "rt-123", Endpoint: "https://example.com"},
			false,
		},
		{
			"empty config",
			Config{},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.CanRefresh()
			if got != tt.want {
				t.Errorf("CanRefresh() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAuthToken(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			"apikey returns api key",
			Config{AuthMethod: "apikey", APIKey: "sk-123"},
			"sk-123",
		},
		{
			"oauth returns access token",
			Config{AuthMethod: "oauth", OAuthAccessToken: "access-tok"},
			"access-tok",
		},
		{
			"unknown returns oauth token field",
			Config{AuthMethod: "unknown", OAuthAccessToken: "fallback"},
			"fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.GetAuthToken()
			if got != tt.want {
				t.Errorf("GetAuthToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetProfileCurrentSessionPath(t *testing.T) {
	setTestHome(t)

	path, err := GetProfileCurrentSessionPath("myprofile")
	if err != nil {
		t.Fatalf("GetProfileCurrentSessionPath() error: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ConfigDirName, ProfilesDirName, "myprofile", CurrentSessionFile)
	if path != expected {
		t.Errorf("GetProfileCurrentSessionPath() = %q, want %q", path, expected)
	}
}

func TestSaveAuth_APIKey(t *testing.T) {
	setTestHome(t)

	auth := &Auth{
		Method: "apikey",
		APIKey: "sk-auth-test",
		TeamID: "team-1",
	}

	if err := SaveAuth(auth, "default"); err != nil {
		t.Fatalf("SaveAuth() error: %v", err)
	}

	loaded, err := Load("default")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.AuthMethod != "apikey" {
		t.Errorf("AuthMethod = %q, want %q", loaded.AuthMethod, "apikey")
	}
	if loaded.APIKey != "sk-auth-test" {
		t.Errorf("APIKey = %q, want %q", loaded.APIKey, "sk-auth-test")
	}
	if loaded.TeamID != "team-1" {
		t.Errorf("TeamID = %q, want %q", loaded.TeamID, "team-1")
	}
}

func TestSaveAuth_OAuth(t *testing.T) {
	setTestHome(t)

	expires := time.Now().Add(1 * time.Hour).Truncate(time.Second)
	auth := &Auth{
		Method:       "oauth",
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		ExpiresAt:    expires,
		TeamID:       "team-2",
	}

	if err := SaveAuth(auth, "default"); err != nil {
		t.Fatalf("SaveAuth() error: %v", err)
	}

	loaded, err := Load("default")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.AuthMethod != "oauth" {
		t.Errorf("AuthMethod = %q, want %q", loaded.AuthMethod, "oauth")
	}
	if loaded.OAuthAccessToken != "access-123" {
		t.Errorf("OAuthAccessToken = %q, want %q", loaded.OAuthAccessToken, "access-123")
	}
	if loaded.APIKey != "" {
		t.Errorf("APIKey should be empty for oauth, got %q", loaded.APIKey)
	}
}

func TestSaveAuth_PreservesExistingFields(t *testing.T) {
	setTestHome(t)

	// Save initial config with extra fields
	initial := &Config{
		Endpoint: "https://example.com",
		Editor:   "code",
	}
	_ = Save(initial, "default")

	// SaveAuth should preserve non-auth fields
	auth := &Auth{
		Method: "apikey",
		APIKey: "sk-new",
		TeamID: "team-3",
	}
	_ = SaveAuth(auth, "default")

	loaded, _ := Load("default")
	if loaded.Endpoint != "https://example.com" {
		t.Errorf("Endpoint should be preserved, got %q", loaded.Endpoint)
	}
	if loaded.Editor != "code" {
		t.Errorf("Editor should be preserved, got %q", loaded.Editor)
	}
}

func TestAPIKeyEncryption_VerifyEncryptedPrefix(t *testing.T) {
	home := setTestHome(t)

	cfg := &Config{
		Endpoint:   "https://example.com",
		AuthMethod: "apikey",
		APIKey:     "test-key",
	}
	_ = Save(cfg, "default")

	// Verify the raw file on disk contains encrypted_api_key and not the plaintext value
	configPath := filepath.Join(home, ConfigDirName, ProfilesDirName, "default", ConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	contents := string(data)
	if !containsSubstring(contents, "encrypted_api_key") {
		t.Error("config file does not contain encrypted_api_key field")
	}
	if containsSubstring(contents, "\"test-key\"") {
		t.Error("config file contains plaintext API key value")
	}

	// Verify crypto produces enc: prefix
	encrypted, err := crypto.Encrypt("test-key")
	if err != nil {
		t.Fatalf("crypto.Encrypt() error: %v", err)
	}
	if !crypto.IsEncrypted(encrypted) {
		t.Error("encrypted key should have enc: prefix")
	}
}
