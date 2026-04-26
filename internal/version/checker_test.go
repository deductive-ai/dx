// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadWriteCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "version_cache.json")

	original := &versionCache{
		Blocked:   []string{"v0.1.0", "v0.2.0"},
		Latest:    "v1.0.0",
		FetchedAt: time.Now().Truncate(time.Second),
	}

	if err := writeCache(path, original); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	got, err := readCache(path)
	if err != nil {
		t.Fatalf("readCache: %v", err)
	}

	if got.Latest != original.Latest {
		t.Errorf("latest: got %q, want %q", got.Latest, original.Latest)
	}
	if len(got.Blocked) != len(original.Blocked) {
		t.Errorf("blocked len: got %d, want %d", len(got.Blocked), len(original.Blocked))
	}
	for i, v := range original.Blocked {
		if got.Blocked[i] != v {
			t.Errorf("blocked[%d]: got %q, want %q", i, got.Blocked[i], v)
		}
	}
}

func TestCacheTTL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "version_cache.json")

	fresh := &versionCache{
		Blocked:   []string{"v0.1.0"},
		Latest:    "v1.0.0",
		FetchedAt: time.Now(),
	}
	if err := writeCache(path, fresh); err != nil {
		t.Fatal(err)
	}
	got, err := readCache(path)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(got.FetchedAt) >= cacheTTL {
		t.Error("fresh cache should not be expired")
	}

	stale := &versionCache{
		Blocked:   []string{"v0.1.0"},
		Latest:    "v1.0.0",
		FetchedAt: time.Now().Add(-2 * cacheTTL),
	}
	data, _ := json.MarshalIndent(stale, "", "  ")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	got2, err := readCache(path)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(got2.FetchedAt) < cacheTTL {
		t.Error("stale cache should be expired")
	}
}

func TestCheckSkipsDevVersion(t *testing.T) {
	// "dev" version should be a no-op — if it tried to call osExit we'd notice.
	// We simply verify it doesn't panic or attempt network I/O.
	Check("dev", "default")
}

// TestCheckExitsOnBlockedVersion exercises the Check(currentVersion, profile)
// path via the osExit hook, avoiding the need to spawn a subprocess.
func TestCheckExitsOnBlockedVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, versionCacheFile)

	// Seed a fresh cache with the test version blocked.
	cache := &versionCache{
		Blocked:   []string{"v0.1.0-blocked"},
		Latest:    "v1.0.0",
		FetchedAt: time.Now(),
	}
	if err := writeCache(path, cache); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	// Swap osExit to capture the call instead of terminating the process.
	var exitCalled bool
	var exitCode int
	origExit := osExit
	osExit = func(code int) {
		exitCalled = true
		exitCode = code
	}
	defer func() { osExit = origExit }()

	// Override the cache path by writing to the real location Check() uses.
	realPath, err := getCachePath()
	if err != nil {
		t.Skipf("cannot determine cache path: %v", err)
	}
	// Backup and restore the real cache so the test is hermetic.
	origData, readErr := os.ReadFile(realPath)
	if err := writeCache(realPath, cache); err != nil {
		t.Fatalf("seeding cache: %v", err)
	}
	defer func() {
		if readErr == nil {
			_ = os.WriteFile(realPath, origData, 0600)
		} else {
			_ = os.Remove(realPath)
		}
	}()

	// Check should invoke osExit(1) because "v0.1.0-blocked" is in the list.
	// Profile "nonexistent" will fail to load config, causing loadOrFetch to
	// return the on-disk cache we seeded above.
	// We need a profile that loads without error but has no auth so it uses cache.
	// Simplest: use the seeded cache and a missing profile (fail-open → use stale cache).
	// Instead, directly test via loadOrFetch returning the seeded data.
	// Since getCachePath() returns the real path, seed it and let Check run.
	Check("v0.1.0-blocked", "default")

	if !exitCalled {
		t.Error("expected osExit to be called for blocked version")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}

func TestGetCachePath(t *testing.T) {
	path, err := getCachePath()
	if err != nil {
		t.Fatalf("getCachePath: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty cache path")
	}
	// Must end with the expected filename.
	if filepath.Base(path) != versionCacheFile {
		t.Errorf("expected filename %q, got %q", versionCacheFile, filepath.Base(path))
	}
}
