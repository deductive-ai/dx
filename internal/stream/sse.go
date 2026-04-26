// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Event types sent by the SSE stream
const (
	EventConnected      = "connected"
	EventProgress       = "progress"
	EventAnswer         = "answer"
	EventComplete       = "complete"
	EventError          = "error"
	EventStats          = "stats"
	EventProgressReport = "progress_report"
)

// Event represents a Server-Sent Event
type Event struct {
	Type    string          `json:"type"`    // "progress", "answer", "complete", "error", "stats", "progress_report"
	Message string          `json:"message"` // Progress message
	Content string          `json:"content"` // Answer content / progress report text
	Status  string          `json:"status"`  // Status for complete events
	Stats   json.RawMessage `json:"stats"`   // Agent execution stats tree (for "stats" events)
}

// SSEClient handles Server-Sent Events streaming
type SSEClient struct {
	url        string
	authToken  string
	httpClient *http.Client
}

// NewSSEClient creates a new SSE client
func NewSSEClient(url string, authToken string) *SSEClient {
	return &SSEClient{
		url:        url,
		authToken:  authToken,
		httpClient: &http.Client{},
	}
}

// Stream connects to the SSE endpoint and returns a channel of events
func (c *SSEClient) Stream() (<-chan Event, <-chan error, func()) {
	events := make(chan Event, 100)
	errors := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer close(events)
		defer close(errors)

		req, err := http.NewRequestWithContext(ctx, "GET", c.url, nil)
		if err != nil {
			errors <- fmt.Errorf("failed to create request: %w", err)
			return
		}

		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")
		req.Header.Set("Connection", "keep-alive")

		if c.authToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.authToken)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return // cancelled, not an error
			}
			errors <- fmt.Errorf("failed to connect: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			errors <- fmt.Errorf("server returned status %d", resp.StatusCode)
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		var eventData strings.Builder

		for scanner.Scan() {
			line := scanner.Text()

			if line == "" {
				if eventData.Len() > 0 {
					var event Event
					if err := json.Unmarshal([]byte(eventData.String()), &event); err == nil {
						select {
						case events <- event:
						case <-ctx.Done():
							return
						}
					}
					eventData.Reset()
				}
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				eventData.WriteString(data)
			} else if strings.HasPrefix(line, "data:") {
				data := strings.TrimPrefix(line, "data:")
				eventData.WriteString(data)
			}
		}

		if eventData.Len() > 0 {
			var event Event
			if err := json.Unmarshal([]byte(eventData.String()), &event); err == nil {
				select {
				case events <- event:
				case <-ctx.Done():
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			if ctx.Err() != nil {
				return // cancelled
			}
			errors <- fmt.Errorf("read error: %w", err)
		}
	}()

	return events, errors, cancel
}

// StreamResponse streams the response for a session
func StreamResponse(baseURL string, sessionID string, authToken string) (<-chan Event, <-chan error, func()) {
	url := fmt.Sprintf("%s/api/v1/sessions/%s/stream", baseURL, sessionID)
	client := NewSSEClient(url, authToken)
	return client.Stream()
}
