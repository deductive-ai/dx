package cmd

import (
	"bytes"
	"io"
	"os"
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

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestConfigShow_NoConfig(t *testing.T) {
	setTestHome(t)

	out := captureStdout(t, func() {
		runConfigShow(nil, nil)
	})

	if out == "" {
		t.Error("expected output, got empty")
	}
	if !bytes.Contains([]byte(out), []byte("Not configured")) {
		t.Errorf("expected 'Not configured' message, got: %s", out)
	}
}

func TestConfigShow_WithConfig(t *testing.T) {
	setTestHome(t)

	cfg := &config.Config{
		Endpoint:         "https://acme.deductive.ai",
		AuthMethod:       "oauth",
		OAuthAccessToken: "tok",
		OAuthExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	if err := config.Save(cfg, "default"); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if err := config.WriteActiveProfile("default"); err != nil {
		t.Fatalf("WriteActiveProfile() error: %v", err)
	}

	profileFlag = "default"
	profileExplicit = true
	defer func() { profileExplicit = false }()

	out := captureStdout(t, func() {
		runConfigShow(nil, nil)
	})

	if !bytes.Contains([]byte(out), []byte("https://acme.deductive.ai")) {
		t.Errorf("expected endpoint in output, got: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("authenticated")) {
		t.Errorf("expected auth status in output, got: %s", out)
	}
}

func TestConfigShow_ExpiredOAuth(t *testing.T) {
	setTestHome(t)

	cfg := &config.Config{
		Endpoint:         "https://acme.deductive.ai",
		AuthMethod:       "oauth",
		OAuthAccessToken: "tok",
		OAuthExpiresAt:   time.Now().Add(-1 * time.Hour),
	}
	if err := config.Save(cfg, "default"); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	profileFlag = "default"
	profileExplicit = true
	defer func() { profileExplicit = false }()

	out := captureStdout(t, func() {
		runConfigShow(nil, nil)
	})

	if !bytes.Contains([]byte(out), []byte("expired")) {
		t.Errorf("expected 'expired' in output, got: %s", out)
	}
}

func TestConfigReset_DeletesProfile(t *testing.T) {
	setTestHome(t)

	cfg := &config.Config{
		Endpoint:   "https://acme.deductive.ai",
		AuthMethod: "oauth",
	}
	if err := config.Save(cfg, "staging"); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if err := config.WriteActiveProfile("staging"); err != nil {
		t.Fatalf("WriteActiveProfile() error: %v", err)
	}

	if !config.ProfileExists("staging") {
		t.Fatal("expected profile to exist before reset")
	}

	profileFlag = "staging"
	profileExplicit = true
	defer func() { profileExplicit = false }()

	captureStdout(t, func() {
		runConfigReset(nil, nil)
	})

	if config.ProfileExists("staging") {
		t.Error("expected profile to be deleted after reset")
	}

	active, err := config.ReadActiveProfile()
	if err != nil {
		t.Fatalf("ReadActiveProfile() error: %v", err)
	}
	if active != config.DefaultProfile {
		t.Errorf("active profile = %q, want %q", active, config.DefaultProfile)
	}
}

func TestConfigReset_AlsoDeletesDefault(t *testing.T) {
	setTestHome(t)

	if err := config.Save(&config.Config{Endpoint: "https://turing.deductive.ai", AuthMethod: "oauth"}, config.DefaultProfile); err != nil {
		t.Fatalf("Save(default) error: %v", err)
	}
	if err := config.Save(&config.Config{Endpoint: "https://acme.deductive.ai", AuthMethod: "oauth"}, "staging"); err != nil {
		t.Fatalf("Save(staging) error: %v", err)
	}
	if err := config.WriteActiveProfile("staging"); err != nil {
		t.Fatalf("WriteActiveProfile() error: %v", err)
	}

	profileFlag = "staging"
	profileExplicit = true
	defer func() { profileExplicit = false }()

	captureStdout(t, func() {
		runConfigReset(nil, nil)
	})

	if config.ProfileExists("staging") {
		t.Error("expected staging profile to be deleted")
	}
	if config.ProfileExists(config.DefaultProfile) {
		t.Error("expected default profile to also be deleted")
	}

	active, err := config.ReadActiveProfile()
	if err != nil {
		t.Fatalf("ReadActiveProfile() error: %v", err)
	}
	if active != config.DefaultProfile {
		t.Errorf("active profile = %q, want %q", active, config.DefaultProfile)
	}
}

func TestConfigReset_NoConfig(t *testing.T) {
	setTestHome(t)

	profileFlag = "default"
	profileExplicit = true
	defer func() { profileExplicit = false }()

	out := captureStdout(t, func() {
		runConfigReset(nil, nil)
	})

	if !bytes.Contains([]byte(out), []byte("Nothing to reset")) {
		t.Errorf("expected 'Nothing to reset' message, got: %s", out)
	}
}
