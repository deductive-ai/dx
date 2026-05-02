// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"time"

	"github.com/deductive-ai/dx/internal/api"
	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/logging"
)

// EnsureAuth checks authentication and transparently refreshes an expired
// OAuth access token when a valid refresh token is available. Returns the
// (possibly updated) config or an error describing what the user must do.
func EnsureAuth(cfg *config.Config, profile string) (*config.Config, error) {
	if cfg.IsAuthenticated() {
		return cfg, nil
	}

	if !cfg.CanRefresh() {
		return nil, fmt.Errorf("not authenticated. Run 'dx auth' to re-authenticate")
	}

	client := api.NewClientWithEndpoint(cfg.Endpoint)
	resp, err := client.RefreshAccessToken(cfg.OAuthRefreshToken)
	if err != nil {
		return nil, fmt.Errorf("token refresh failed: %v. Run 'dx auth' to re-authenticate", err)
	}

	auth := &config.Auth{
		Method:       "oauth",
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second),
		TeamID:       resp.TeamID,
		TeamName:     resp.TeamName,
	}

	if err := config.SaveAuth(auth, profile); err != nil {
		return nil, fmt.Errorf("failed to save refreshed credentials: %v", err)
	}

	updated, err := config.Load(profile)
	if err != nil {
		return nil, fmt.Errorf("failed to reload config after refresh: %v", err)
	}

	logging.Debug("Auth token refreshed", "team_id", resp.TeamID)

	fmt.Println("✓ Access token refreshed")
	return updated, nil
}
