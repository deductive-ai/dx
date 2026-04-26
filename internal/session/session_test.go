package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/crypto"
)

func setTestHome(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	return tmpDir
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	setTestHome(t)

	original := &State{
		SessionID: "sess-abc-123",
		Profile:   "default",
		URL:       "https://app.example.com/session/sess-abc-123",
		PresignedURLs: []api.PresignedURL{
			{UploadURL: "https://s3.example.com/upload1?sig=abc", Key: "team1/thread_sess/uploads/file_0", ExpiresAt: "2025-12-31T23:59:59Z"},
			{UploadURL: "https://s3.example.com/upload2?sig=def", Key: "team1/thread_sess/uploads/file_1", ExpiresAt: "2025-12-31T23:59:59Z"},
		},
		CreatedAt: time.Now().Truncate(time.Second),
		URLsUsed:  1,
	}

	if err := Save(original); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load(original.SessionID)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load() returned nil")
	}

	if loaded.SessionID != original.SessionID {
		t.Errorf("SessionID = %q, want %q", loaded.SessionID, original.SessionID)
	}
	if loaded.Profile != original.Profile {
		t.Errorf("Profile = %q, want %q", loaded.Profile, original.Profile)
	}
	if loaded.URL != original.URL {
		t.Errorf("URL = %q, want %q", loaded.URL, original.URL)
	}
	if loaded.URLsUsed != original.URLsUsed {
		t.Errorf("URLsUsed = %d, want %d", loaded.URLsUsed, original.URLsUsed)
	}
	if len(loaded.PresignedURLs) != len(original.PresignedURLs) {
		t.Fatalf("PresignedURLs length = %d, want %d", len(loaded.PresignedURLs), len(original.PresignedURLs))
	}
	for i, u := range loaded.PresignedURLs {
		if u.Key != original.PresignedURLs[i].Key {
			t.Errorf("PresignedURLs[%d].Key = %q, want %q", i, u.Key, original.PresignedURLs[i].Key)
		}
	}
}

func TestLoad_NonExistent(t *testing.T) {
	setTestHome(t)
	_ = config.EnsureConfigDir()

	state, err := Load("nonexistent-session")
	if err != nil {
		t.Fatalf("Load() should return nil,nil for non-existent session, got error: %v", err)
	}
	if state != nil {
		t.Error("Load() should return nil for non-existent session")
	}
}

func TestGetSetCurrentSessionID(t *testing.T) {
	setTestHome(t)

	profile := "default"
	_ = config.EnsureProfileDir(profile)

	// No current session initially
	id, err := GetCurrentSessionID(profile)
	if err != nil {
		t.Fatalf("GetCurrentSessionID() error: %v", err)
	}
	if id != "" {
		t.Errorf("GetCurrentSessionID() = %q, want empty string", id)
	}

	// Set current session
	if err := SetCurrentSessionID("sess-xyz", profile); err != nil {
		t.Fatalf("SetCurrentSessionID() error: %v", err)
	}

	id, err = GetCurrentSessionID(profile)
	if err != nil {
		t.Fatalf("GetCurrentSessionID() error: %v", err)
	}
	if id != "sess-xyz" {
		t.Errorf("GetCurrentSessionID() = %q, want %q", id, "sess-xyz")
	}
}

func TestLoadCurrent(t *testing.T) {
	setTestHome(t)

	profile := "default"

	// No current session
	state, err := LoadCurrent(profile)
	if err != nil {
		t.Fatalf("LoadCurrent() error: %v", err)
	}
	if state != nil {
		t.Error("LoadCurrent() should return nil when no current session")
	}

	// Save a session and set it as current
	s := &State{
		SessionID: "sess-current",
		Profile:   profile,
		URL:       "https://example.com",
		CreatedAt: time.Now(),
	}
	_ = Save(s)
	_ = SetCurrentSessionID(s.SessionID, profile)

	state, err = LoadCurrent(profile)
	if err != nil {
		t.Fatalf("LoadCurrent() error: %v", err)
	}
	if state == nil {
		t.Fatal("LoadCurrent() returned nil")
	}
	if state.SessionID != "sess-current" {
		t.Errorf("SessionID = %q, want %q", state.SessionID, "sess-current")
	}
}

func TestGetAvailableURLCount(t *testing.T) {
	tests := []struct {
		name     string
		total    int
		used     int
		expected int
	}{
		{"all available", 5, 0, 5},
		{"some used", 5, 3, 2},
		{"all used", 5, 5, 0},
		{"none", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			urls := make([]api.PresignedURL, tt.total)
			s := &State{PresignedURLs: urls, URLsUsed: tt.used}
			got := s.GetAvailableURLCount()
			if got != tt.expected {
				t.Errorf("GetAvailableURLCount() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestGetNextPresignedURL(t *testing.T) {
	s := &State{
		PresignedURLs: []api.PresignedURL{
			{UploadURL: "url1", Key: "key1"},
			{UploadURL: "url2", Key: "key2"},
		},
		URLsUsed: 0,
	}

	url1, err := s.GetNextPresignedURL()
	if err != nil {
		t.Fatalf("GetNextPresignedURL() error: %v", err)
	}
	if url1.Key != "key1" {
		t.Errorf("first call Key = %q, want %q", url1.Key, "key1")
	}

	url2, err := s.GetNextPresignedURL()
	if err != nil {
		t.Fatalf("GetNextPresignedURL() error: %v", err)
	}
	if url2.Key != "key2" {
		t.Errorf("second call Key = %q, want %q", url2.Key, "key2")
	}

	_, err = s.GetNextPresignedURL()
	if err == nil {
		t.Error("GetNextPresignedURL() should return error when exhausted")
	}
}

func TestListAll(t *testing.T) {
	setTestHome(t)
	_ = config.EnsureConfigDir()

	// No sessions
	sessions, err := ListAll()
	if err != nil {
		t.Fatalf("ListAll() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("ListAll() returned %d sessions, want 0", len(sessions))
	}

	// Save some sessions
	_ = Save(&State{SessionID: "sess-1", Profile: "default", CreatedAt: time.Now()})
	_ = Save(&State{SessionID: "sess-2", Profile: "default", CreatedAt: time.Now()})

	sessions, err = ListAll()
	if err != nil {
		t.Fatalf("ListAll() error: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("ListAll() returned %d sessions, want 2", len(sessions))
	}
}

func TestClearAll(t *testing.T) {
	setTestHome(t)
	_ = config.EnsureConfigDir()

	defaultProfile := "default"
	stagingProfile := "staging"
	_ = config.EnsureProfileDir(defaultProfile)
	_ = config.EnsureProfileDir(stagingProfile)

	_ = Save(&State{SessionID: "sess-a", Profile: defaultProfile, CreatedAt: time.Now()})
	_ = Save(&State{SessionID: "sess-b", Profile: defaultProfile, CreatedAt: time.Now()})
	_ = Save(&State{SessionID: "sess-c", Profile: stagingProfile, CreatedAt: time.Now()})
	_ = SetCurrentSessionID("sess-a", defaultProfile)
	_ = SetCurrentSessionID("sess-c", stagingProfile)

	count, err := ClearAll(defaultProfile)
	if err != nil {
		t.Fatalf("ClearAll() error: %v", err)
	}
	if count != 2 {
		t.Errorf("ClearAll() removed %d sessions, want 2", count)
	}

	// Staging session should be untouched
	remaining, _ := ListAll()
	if len(remaining) != 1 {
		t.Errorf("ListAll() after ClearAll(default) returned %d sessions, want 1", len(remaining))
	}
	if len(remaining) == 1 && remaining[0].SessionID != "sess-c" {
		t.Errorf("remaining session should be sess-c (staging), got %s", remaining[0].SessionID)
	}

	// Default current session should be cleared, staging should not
	id, _ := GetCurrentSessionID(defaultProfile)
	if id != "" {
		t.Errorf("default current session should be cleared, got %q", id)
	}
	stagingID, _ := GetCurrentSessionID(stagingProfile)
	if stagingID != "sess-c" {
		t.Errorf("staging current session should be untouched, got %q", stagingID)
	}
}

func TestDelete(t *testing.T) {
	setTestHome(t)
	_ = config.EnsureConfigDir()

	profile := "default"
	_ = config.EnsureProfileDir(profile)

	_ = Save(&State{SessionID: "sess-del", Profile: profile, CreatedAt: time.Now()})
	_ = SetCurrentSessionID("sess-del", profile)

	if err := Delete("sess-del", profile); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	// Session file should be gone
	state, _ := Load("sess-del")
	if state != nil {
		t.Error("session should not exist after Delete()")
	}

	// Current session pointer should be cleared
	id, _ := GetCurrentSessionID(profile)
	if id != "" {
		t.Errorf("current session should be cleared after deleting current, got %q", id)
	}
}

func TestDelete_NonMatchingCurrent(t *testing.T) {
	setTestHome(t)
	_ = config.EnsureConfigDir()

	profile := "default"
	_ = config.EnsureProfileDir(profile)

	_ = Save(&State{SessionID: "sess-keep", Profile: profile, CreatedAt: time.Now()})
	_ = Save(&State{SessionID: "sess-remove", Profile: profile, CreatedAt: time.Now()})
	_ = SetCurrentSessionID("sess-keep", profile)

	_ = Delete("sess-remove", profile)

	// Current session pointer should still point to sess-keep
	id, _ := GetCurrentSessionID(profile)
	if id != "sess-keep" {
		t.Errorf("current session should remain %q, got %q", "sess-keep", id)
	}
}

func TestDelete_NonExistent(t *testing.T) {
	setTestHome(t)
	_ = config.EnsureConfigDir()

	err := Delete("ghost-session", "default")
	if err == nil {
		t.Error("Delete() should return error for non-existent session")
	}
}

func TestSessionEncryption_PresignedURLs(t *testing.T) {
	home := setTestHome(t)
	_ = config.EnsureConfigDir()

	state := &State{
		SessionID: "sess-enc-test",
		Profile:   "default",
		PresignedURLs: []api.PresignedURL{
			{UploadURL: "https://s3.example.com/secret-upload?sig=topsecret", Key: "key1"},
		},
		CreatedAt: time.Now(),
	}

	_ = Save(state)

	// Read raw file to verify URLs are encrypted
	sessionsDir := filepath.Join(home, config.ConfigDirName, config.SessionsDirName)
	data, err := os.ReadFile(filepath.Join(sessionsDir, "sess-enc-test"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	var raw map[string]interface{}
	_ = json.Unmarshal(data, &raw)

	encURLs, ok := raw["encrypted_urls"].(string)
	if !ok || encURLs == "" {
		t.Fatal("session file should contain encrypted_urls field")
	}
	if !crypto.IsEncrypted(encURLs) {
		t.Error("encrypted_urls value should have encryption prefix")
	}

	// Verify the raw file does NOT contain the presigned URL in plaintext
	contents := string(data)
	if containsSubstring(contents, "topsecret") {
		t.Error("session file contains presigned URL signature in plaintext")
	}

	// Verify Load decrypts correctly
	loaded, err := Load("sess-enc-test")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(loaded.PresignedURLs) != 1 {
		t.Fatalf("PresignedURLs length = %d, want 1", len(loaded.PresignedURLs))
	}
	if loaded.PresignedURLs[0].UploadURL != state.PresignedURLs[0].UploadURL {
		t.Errorf("decrypted UploadURL = %q, want %q", loaded.PresignedURLs[0].UploadURL, state.PresignedURLs[0].UploadURL)
	}
}

func TestUpdateFromResponse(t *testing.T) {
	s := &State{
		SessionID: "old-id",
		URLsUsed:  3,
	}

	resp := &api.SessionResponse{
		SessionID: "new-id",
		URL:       "https://app.example.com/new",
		PresignedURLs: []api.PresignedURL{
			{UploadURL: "url1", Key: "key1"},
		},
	}

	s.UpdateFromResponse(resp)

	if s.SessionID != "new-id" {
		t.Errorf("SessionID = %q, want %q", s.SessionID, "new-id")
	}
	if s.URL != "https://app.example.com/new" {
		t.Errorf("URL = %q, want %q", s.URL, "https://app.example.com/new")
	}
	if s.URLsUsed != 0 {
		t.Errorf("URLsUsed = %d, want 0 (should be reset)", s.URLsUsed)
	}
	if len(s.PresignedURLs) != 1 {
		t.Errorf("PresignedURLs length = %d, want 1", len(s.PresignedURLs))
	}
}

func TestExists(t *testing.T) {
	setTestHome(t)
	_ = config.EnsureConfigDir()

	profile := "default"
	_ = config.EnsureProfileDir(profile)

	if Exists(profile) {
		t.Error("Exists() should return false with no current session")
	}

	_ = Save(&State{SessionID: "sess-exists", Profile: profile, CreatedAt: time.Now()})
	_ = SetCurrentSessionID("sess-exists", profile)

	if !Exists(profile) {
		t.Error("Exists() should return true after setting current session")
	}
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestListForProfile(t *testing.T) {
	setTestHome(t)

	s1 := &State{SessionID: "sess-aaa", Profile: "default", CreatedAt: time.Now()}
	s2 := &State{SessionID: "sess-bbb", Profile: "staging", CreatedAt: time.Now()}
	s3 := &State{SessionID: "sess-ccc", Profile: "default", CreatedAt: time.Now()}
	_ = Save(s1)
	_ = Save(s2)
	_ = Save(s3)

	defaultSessions, err := ListForProfile("default")
	if err != nil {
		t.Fatalf("ListForProfile failed: %v", err)
	}
	if len(defaultSessions) != 2 {
		t.Errorf("expected 2 default sessions, got %d", len(defaultSessions))
	}

	stagingSessions, err := ListForProfile("staging")
	if err != nil {
		t.Fatalf("ListForProfile failed: %v", err)
	}
	if len(stagingSessions) != 1 {
		t.Errorf("expected 1 staging session, got %d", len(stagingSessions))
	}

	emptySessions, err := ListForProfile("nonexistent")
	if err != nil {
		t.Fatalf("ListForProfile failed: %v", err)
	}
	if len(emptySessions) != 0 {
		t.Errorf("expected 0 sessions for nonexistent profile, got %d", len(emptySessions))
	}
}

func TestResolveShortID(t *testing.T) {
	setTestHome(t)

	s1 := &State{SessionID: "abc12345-1111-2222-3333-444444444444", Profile: "default", CreatedAt: time.Now()}
	s2 := &State{SessionID: "abc12345-5555-6666-7777-888888888888", Profile: "default", CreatedAt: time.Now()}
	s3 := &State{SessionID: "def99999-1111-2222-3333-444444444444", Profile: "default", CreatedAt: time.Now()}
	s4 := &State{SessionID: "abc12345-9999-0000-1111-222222222222", Profile: "staging", CreatedAt: time.Now()}
	_ = Save(s1)
	_ = Save(s2)
	_ = Save(s3)
	_ = Save(s4)

	// Full ID should return as-is
	resolved, err := ResolveShortID(s1.SessionID, "default")
	if err != nil || resolved != s1.SessionID {
		t.Errorf("full ID resolution failed: got %s, err %v", resolved, err)
	}

	// Unique prefix should resolve
	resolved, err = ResolveShortID("def9", "default")
	if err != nil || resolved != s3.SessionID {
		t.Errorf("unique prefix resolution failed: got %s, err %v", resolved, err)
	}

	// Ambiguous prefix within same profile should error
	_, err = ResolveShortID("abc1", "default")
	if err == nil {
		t.Error("expected error for ambiguous prefix, got nil")
	}

	// Same prefix but different profile should resolve (only 1 match in staging)
	resolved, err = ResolveShortID("abc1", "staging")
	if err != nil || resolved != s4.SessionID {
		t.Errorf("cross-profile resolution failed: got %s, err %v", resolved, err)
	}

	// No match should error
	_, err = ResolveShortID("zzz", "default")
	if err == nil {
		t.Error("expected error for no match, got nil")
	}
}
