// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package stream

import "fmt"

// AgentStats mirrors the hierarchical agent execution stats tree from inference.
type AgentStats struct {
	TaskLabel         string       `json:"taskLabel"`
	AgentType         string       `json:"agentType"`
	AgentID           string       `json:"agentId"`
	IsRoot            bool         `json:"isRoot"`
	Status            string       `json:"status"` // "running" or "completed"
	TotalInputTokens  int          `json:"totalInputTokens"`
	TotalOutputTokens int          `json:"totalOutputTokens"`
	ToolCallCount     int          `json:"toolCallCount"`
	ToolCallsMade     []string     `json:"toolCallsMade"`
	IterationCount    int          `json:"iterationCount"`
	ElapsedSeconds    float64      `json:"elapsedSeconds"`
	Children          []AgentStats `json:"children"`
}

// TotalTokens returns the aggregate token count (input + output) across the entire tree.
func (s *AgentStats) TotalTokens() int {
	input, output := s.aggregateTokens()
	return input + output
}

// AggregateInputTokens returns total input tokens across the tree.
func (s *AgentStats) AggregateInputTokens() int {
	input, _ := s.aggregateTokens()
	return input
}

// AggregateOutputTokens returns total output tokens across the tree.
func (s *AgentStats) AggregateOutputTokens() int {
	_, output := s.aggregateTokens()
	return output
}

func (s *AgentStats) aggregateTokens() (int, int) {
	input := s.TotalInputTokens
	output := s.TotalOutputTokens
	for i := range s.Children {
		ci, co := s.Children[i].aggregateTokens()
		input += ci
		output += co
	}
	return input, output
}

// AggregateToolCalls returns total tool call count across the tree.
func (s *AgentStats) AggregateToolCalls() int {
	count := s.ToolCallCount
	for i := range s.Children {
		count += s.Children[i].AggregateToolCalls()
	}
	return count
}

// ActiveTask returns the label of the deepest currently-running agent in the tree.
func (s *AgentStats) ActiveTask() string {
	for i := len(s.Children) - 1; i >= 0; i-- {
		if s.Children[i].Status == "running" {
			if child := s.Children[i].ActiveTask(); child != "" {
				return child
			}
			return s.Children[i].TaskLabel
		}
	}
	if s.Status == "running" {
		return s.TaskLabel
	}
	return ""
}

// FormatTokenCount formats a token count for display (e.g., 12400 -> "12.4k").
func FormatTokenCount(tokens int) string {
	if tokens >= 1000 {
		return fmt.Sprintf("%.1fk", float64(tokens)/1000)
	}
	return fmt.Sprintf("%d", tokens)
}

// FormatElapsedTime formats seconds into a human-readable duration (e.g., 123.5 -> "2m 3.5s").
func FormatElapsedTime(seconds float64) string {
	if seconds < 60 {
		return fmt.Sprintf("%.1fs", seconds)
	}
	mins := int(seconds) / 60
	secs := seconds - float64(mins*60)
	return fmt.Sprintf("%dm %.1fs", mins, secs)
}
