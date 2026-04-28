// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package version provides CLI version blocking checks.
// On first invocation (and at most once per hour) it fetches the list of
// blocked CLI versions from the Deductive API. If the running version
// appears in that list the user is told to upgrade and the process exits.
package version

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/logging"
)

const (
	versionCacheFile = "version_cache.json"
	cacheTTL         = time.Hour
	versionEndpoint  = "/api/cli/version"
)

// osExit is a package-level hook so tests can intercept os.Exit calls
// without spawning a subprocess.
var osExit = os.Exit

type versionResponse struct {
	Blocked []string `json:"blocked"`
	Latest  string   `json:"latest"`
}

type versionCache struct {
	Blocked   []string  `json:"blocked"`
	Latest    string    `json:"latest"`
	FetchedAt time.Time `json:"fetched_at"`
}

// Check fetches the blocked-versions list (at most once per hour) and exits
// with a user-friendly message if the running version is blocked.
//
// Parameters:
//   - currentVersion: the version string injected at build time (e.g. "v1.2.3")
//   - profile:        the active CLI profile (used to load endpoint + auth token)
//
// The check is silently skipped when:
//   - currentVersion is "dev" (local build)
//   - no profile config exists yet (user hasn't run dx config)
//   - the auth token is missing (user hasn't authenticated yet)
//   - any network or parse error occurs (fail-open)
func Check(currentVersion string, profile string) {
	if currentVersion == "dev" {
		return
	}

	cache, err := loadOrFetch(profile)
	if err != nil {
		logging.Debug("Version check skipped", "reason", err.Error())
		return
	}

	for _, blocked := range cache.Blocked {
		if blocked == currentVersion {
			fmt.Fprintln(os.Stderr, color.Error("✗ This version of dx ("+currentVersion+") is no longer supported."))
			fmt.Fprintln(os.Stderr, color.Warning("  Please upgrade by running:"))
			fmt.Fprintln(os.Stderr, "  curl https://app.deductive.ai/cli/install | bash")
			osExit(1)
		}
	}

	if !SuppressHint && cache.Latest != "" && CompareVersions(cache.Latest, currentVersion) > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", color.Muted(
			"A newer version of dx is available ("+cache.Latest+"). Run 'dx upgrade' to update.",
		))
	}
}

// loadOrFetch returns a valid versionCache, either from the on-disk cache
// (if it is younger than cacheTTL) or by fetching it from the API.
func loadOrFetch(profile string) (*versionCache, error) {
	cachePath, err := getCachePath()
	if err != nil {
		return nil, err
	}

	if cached, err := readCache(cachePath); err == nil {
		if time.Since(cached.FetchedAt) < cacheTTL {
			return cached, nil
		}
	}

	fetched, err := fetchFromAPI(profile)
	if err != nil {
		// If we have a stale cache, use it rather than failing open with no data.
		if cached, cacheErr := readCache(cachePath); cacheErr == nil {
			logging.Debug("Using stale version cache", "reason", err.Error())
			return cached, nil
		}
		return nil, err
	}

	cache := &versionCache{
		Blocked:   fetched.Blocked,
		Latest:    fetched.Latest,
		FetchedAt: time.Now(),
	}
	if err := writeCache(cachePath, cache); err != nil {
		logging.Debug("Failed to write version cache", "reason", err.Error())
	}
	return cache, nil
}

func fetchFromAPI(profile string) (*versionResponse, error) {
	cfg, err := config.Load(profile)
	if err != nil {
		return nil, fmt.Errorf("config not loaded: %w", err)
	}

	token := cfg.GetAuthToken()
	if token == "" {
		return nil, fmt.Errorf("no auth token available")
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint not configured")
	}

	url := endpoint + versionEndpoint
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized (token may be expired)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var ver versionResponse
	if err := json.Unmarshal(body, &ver); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	logging.Debug("Fetched version info", "latest", ver.Latest, "blocked", ver.Blocked)
	return &ver, nil
}

func getCachePath() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, versionCacheFile), nil
}

func readCache(path string) (*versionCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cache versionCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

// writeCache writes the cache atomically via a temp file + rename so a
// concurrent process or a crash mid-write never leaves a partial file.
func writeCache(path string, cache *versionCache) error {
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".version_cache_*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp cache file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("writing temp cache file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp cache file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming temp cache file: %w", err)
	}
	return nil
}

// SuppressHint disables the upgrade nudge (set by dx upgrade before Check runs).
var SuppressHint bool

// CompareVersions compares two semver strings (major.minor.patch).
// Returns -1 if a < b, 0 if equal, 1 if a > b.
func CompareVersions(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	for i := 0; i < 3; i++ {
		av, bv := 0, 0
		if i < len(aParts) {
			av, _ = strconv.Atoi(aParts[i])
		}
		if i < len(bParts) {
			bv, _ = strconv.Atoi(bParts[i])
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}
