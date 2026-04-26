package session

import (
	"testing"
	"time"

	"github.com/deductive-ai/dx/internal/config"
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
		CreatedAt: time.Now().Truncate(time.Second),
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

	id, err := GetCurrentSessionID(profile)
	if err != nil {
		t.Fatalf("GetCurrentSessionID() error: %v", err)
	}
	if id != "" {
		t.Errorf("GetCurrentSessionID() = %q, want empty string", id)
	}

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

	state, err := LoadCurrent(profile)
	if err != nil {
		t.Fatalf("LoadCurrent() error: %v", err)
	}
	if state != nil {
		t.Error("LoadCurrent() should return nil when no current session")
	}

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

func TestListAll(t *testing.T) {
	setTestHome(t)
	_ = config.EnsureConfigDir()

	sessions, err := ListAll()
	if err != nil {
		t.Fatalf("ListAll() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("ListAll() returned %d sessions, want 0", len(sessions))
	}

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

	remaining, _ := ListAll()
	if len(remaining) != 1 {
		t.Errorf("ListAll() after ClearAll(default) returned %d sessions, want 1", len(remaining))
	}
	if len(remaining) == 1 && remaining[0].SessionID != "sess-c" {
		t.Errorf("remaining session should be sess-c (staging), got %s", remaining[0].SessionID)
	}

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

	state, _ := Load("sess-del")
	if state != nil {
		t.Error("session should not exist after Delete()")
	}

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

	resolved, err := ResolveShortID(s1.SessionID, "default")
	if err != nil || resolved != s1.SessionID {
		t.Errorf("full ID resolution failed: got %s, err %v", resolved, err)
	}

	resolved, err = ResolveShortID("def9", "default")
	if err != nil || resolved != s3.SessionID {
		t.Errorf("unique prefix resolution failed: got %s, err %v", resolved, err)
	}

	_, err = ResolveShortID("abc1", "default")
	if err == nil {
		t.Error("expected error for ambiguous prefix, got nil")
	}

	resolved, err = ResolveShortID("abc1", "staging")
	if err != nil || resolved != s4.SessionID {
		t.Errorf("cross-profile resolution failed: got %s, err %v", resolved, err)
	}

	_, err = ResolveShortID("zzz", "default")
	if err == nil {
		t.Error("expected error for no match, got nil")
	}
}
