// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/deductive-ai/dx/internal/config"
	"github.com/deductive-ai/dx/internal/logging"
	"github.com/deductive-ai/dx/internal/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Client is the HTTP client for communicating with app-ui APIs
type Client struct {
	baseURL    string
	authToken  string
	authMethod string
	teamID     string
	httpClient *http.Client
}

// newTransport creates an HTTP transport, wrapping with OTel instrumentation only when telemetry is enabled.
func newTransport() http.RoundTripper {
	base := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}
	if telemetry.Enabled() {
		return otelhttp.NewTransport(base)
	}
	return base
}

// NewClient creates a new API client from configuration
func NewClient(cfg *config.Config) *Client {
	return &Client{
		baseURL:    cfg.Endpoint,
		authToken:  cfg.GetAuthToken(),
		authMethod: cfg.AuthMethod,
		teamID:     cfg.TeamID,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: newTransport(),
		},
	}
}

// NewClientWithEndpoint creates a new API client with just an endpoint (for config/ping)
func NewClientWithEndpoint(endpoint string) *Client {
	return &Client{
		baseURL: endpoint,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: newTransport(),
		},
	}
}

// SetAuthToken sets the auth token for requests
func (c *Client) SetAuthToken(token string, method string) {
	c.authToken = token
	c.authMethod = method
}

// SetTeamID sets the team ID for the X-Team-Id header
func (c *Client) SetTeamID(teamID string) {
	c.teamID = teamID
}

// doRequest performs an HTTP request with proper headers
func (c *Client) doRequest(method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	if c.teamID != "" {
		req.Header.Set("X-Team-Id", c.teamID)
	}

	logging.Debug("API request", "method", method, "path", path)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	logging.Debug("API response", "method", method, "path", path, "status", resp.StatusCode)

	return resp, nil
}

// PingResponse represents the response from the ping endpoint
type PingResponse struct {
	Status string `json:"status"`
}

// Ping checks if the endpoint is reachable
func Ping(endpoint string) error {
	client := NewClientWithEndpoint(endpoint)
	// Try to reach the root URL - any response (even 404) means server is up
	resp, err := client.httpClient.Get(endpoint)
	if err != nil {
		return fmt.Errorf("failed to reach endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Accept any response - just checking that the server responds
	// Even redirects or 404s are fine, they indicate the server is running
	return nil
}

// VerifyResponse represents the response from auth verification
type VerifyResponse struct {
	TeamID   string `json:"team_id"`
	TeamName string `json:"team_name"`
	Valid    bool   `json:"valid"`
}

// Verify verifies the authentication token with the server
func (c *Client) Verify(token string) (*VerifyResponse, error) {
	// Temporarily set the token for this request
	originalToken := c.authToken
	c.authToken = token
	defer func() { c.authToken = originalToken }()

	resp, err := c.doRequest("GET", "/api/v1/auth/verify", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("%s", errResp.Error)
		}
		return nil, fmt.Errorf("authentication failed (HTTP 401)")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("verification failed: %s", string(body))
	}

	var result VerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// Team represents a single team entry from the teams endpoint
type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TeamsResponse represents the response from the teams endpoint
type TeamsResponse struct {
	Teams []Team `json:"teams"`
}

// ListTeams returns the teams the authenticated user belongs to
func (c *Client) ListTeams() (*TeamsResponse, error) {
	resp, err := c.doRequest("GET", "/api/v1/auth/teams", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("not authenticated — run 'dx auth' first")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list teams: %s", string(body))
	}

	var result TeamsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// DeviceCodeResponse represents the response from device code request
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// RequestDeviceCode requests a device code for OAuth flow
func (c *Client) RequestDeviceCode() (*DeviceCodeResponse, error) {
	resp, err := c.doRequest("POST", "/api/v1/auth/device/code", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to request device code: %s", string(body))
	}

	var result DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// TokenResponse represents the response from token exchange
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TeamID       string `json:"team_id"`
	TeamName     string `json:"team_name"`
	Error        string `json:"error,omitempty"`
}

// PollForToken polls for the OAuth token after device authorization
func (c *Client) PollForToken(deviceCode string, interval int) (*TokenResponse, error) {
	if interval <= 0 {
		interval = 5
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	timeout := time.After(15 * time.Minute)

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("authorization timeout")
		case <-ticker.C:
			resp, err := c.doRequest("POST", "/api/v1/auth/device/token", map[string]string{
				"device_code": deviceCode,
			})
			if err != nil {
				continue
			}

			var result TokenResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				_ = resp.Body.Close()
				continue
			}
			_ = resp.Body.Close()

			if result.Error == "authorization_pending" {
				continue
			}

			if result.Error == "access_denied" {
				return nil, fmt.Errorf("authorization denied by user")
			}

			if result.Error != "" {
				return nil, fmt.Errorf("authorization error: %s", result.Error)
			}

			if result.AccessToken != "" {
				return &result, nil
			}
		}
	}
}

// RefreshAccessToken exchanges a refresh token for a new access + refresh token pair.
func (c *Client) RefreshAccessToken(refreshToken string) (*TokenResponse, error) {
	resp, err := c.doRequest("POST", "/api/v1/auth/refresh", map[string]string{
		"refresh_token": refreshToken,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("refresh token expired or invalid")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed: %s", string(body))
	}

	var result TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// SessionRequest represents a request to create a session
type SessionRequest struct {
	Message string `json:"message"`
	Mode    string `json:"mode"` // "ask"
	Title   string `json:"title,omitempty"`
}

// SessionResponse represents the response from session creation
type SessionResponse struct {
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
}

// CreateSession creates a new chat session
func (c *Client) CreateSession(req *SessionRequest) (*SessionResponse, error) {
	resp, err := c.doRequest("POST", "/api/v1/sessions", req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create session: %s", string(body))
	}

	var result SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// GetSession retrieves an existing session
func (c *Client) GetSession(sessionID string) (*SessionResponse, error) {
	resp, err := c.doRequest("GET", "/api/v1/sessions/"+sessionID, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("session not found")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get session: %s", string(body))
	}

	var result SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// SendMessageRequest represents a request to send a message
type SendMessageRequest struct {
	Message        string `json:"message"`
	AdditionalText string `json:"additional_text,omitempty"`
	OutputSchema   string `json:"output_schema,omitempty"`
}

// SendMessage sends a message to a session. outputSchema is an optional JSON
// object string (e.g. {"action":"str","pid":"int"}) that instructs the server
// to enforce structured output; pass "" to use free-form text.
func (c *Client) SendMessage(sessionID string, message string, additionalText string, outputSchema string) error {
	req := &SendMessageRequest{
		Message:        message,
		AdditionalText: additionalText,
		OutputSchema:   outputSchema,
	}

	resp, err := c.doRequest("POST", "/api/v1/sessions/"+sessionID+"/messages", req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to send message: %s", string(body))
	}

	return nil
}

// UploadURLRequest represents a request for a presigned upload URL
type UploadURLRequest struct {
	Filename string `json:"filename"`
}

// UploadURLResponse represents the response with a presigned upload URL
type UploadURLResponse struct {
	UploadURL string `json:"upload_url"`
	Key       string `json:"key"`
	ExpiresAt string `json:"expires_at"`
}

// RequestUploadURL requests a single presigned upload URL for a session
func (c *Client) RequestUploadURL(sessionID string, filename string) (*UploadURLResponse, error) {
	req := &UploadURLRequest{Filename: filename}

	resp, err := c.doRequest("POST", "/api/v1/sessions/"+sessionID+"/upload-url", req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get upload URL: %s", string(body))
	}

	var result UploadURLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// AttachFilesRequest represents a request to attach files to a session
type AttachFilesRequest struct {
	S3Keys []string `json:"s3_keys"`
}

// AttachFilesResponse represents the response from attaching files
type AttachFilesResponse struct {
	Success       bool     `json:"success"`
	AttachedFiles []string `json:"attached_files"`
}

// AttachFiles notifies the server that files have been uploaded to S3
func (c *Client) AttachFiles(sessionID string, s3Keys []string) (*AttachFilesResponse, error) {
	req := &AttachFilesRequest{
		S3Keys: s3Keys,
	}

	resp, err := c.doRequest("POST", "/api/v1/sessions/"+sessionID+"/files", req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to attach files: %s", string(body))
	}

	var result AttachFilesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}
