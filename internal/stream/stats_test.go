package stream

import (
	"testing"
)

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		name   string
		tokens int
		want   string
	}{
		{"zero", 0, "0"},
		{"small number", 42, "42"},
		{"just under 1k", 999, "999"},
		{"exactly 1k", 1000, "1.0k"},
		{"1.5k", 1500, "1.5k"},
		{"12.4k", 12400, "12.4k"},
		{"100k", 100000, "100.0k"},
		{"999.9k", 999999, "1000.0k"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTokenCount(tt.tokens)
			if got != tt.want {
				t.Errorf("FormatTokenCount(%d) = %q, want %q", tt.tokens, got, tt.want)
			}
		})
	}
}

func TestFormatElapsedTime(t *testing.T) {
	tests := []struct {
		name    string
		seconds float64
		want    string
	}{
		{"zero", 0, "0.0s"},
		{"sub-second", 0.5, "0.5s"},
		{"few seconds", 5.3, "5.3s"},
		{"59.9 seconds", 59.9, "59.9s"},
		{"exactly 60 seconds", 60, "1m 0.0s"},
		{"1 min 30 sec", 90.5, "1m 30.5s"},
		{"2 min 3.5 sec", 123.5, "2m 3.5s"},
		{"10 minutes", 600, "10m 0.0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatElapsedTime(tt.seconds)
			if got != tt.want {
				t.Errorf("FormatElapsedTime(%v) = %q, want %q", tt.seconds, got, tt.want)
			}
		})
	}
}

func TestAgentStats_TotalTokens_Leaf(t *testing.T) {
	stats := &AgentStats{
		TotalInputTokens:  100,
		TotalOutputTokens: 50,
	}

	if got := stats.TotalTokens(); got != 150 {
		t.Errorf("TotalTokens() = %d, want 150", got)
	}
}

func TestAgentStats_TotalTokens_Tree(t *testing.T) {
	stats := &AgentStats{
		TotalInputTokens:  100,
		TotalOutputTokens: 50,
		Children: []AgentStats{
			{
				TotalInputTokens:  200,
				TotalOutputTokens: 75,
				Children: []AgentStats{
					{
						TotalInputTokens:  50,
						TotalOutputTokens: 25,
					},
				},
			},
			{
				TotalInputTokens:  300,
				TotalOutputTokens: 100,
			},
		},
	}

	wantInput := 100 + 200 + 50 + 300
	wantOutput := 50 + 75 + 25 + 100
	wantTotal := wantInput + wantOutput

	if got := stats.TotalTokens(); got != wantTotal {
		t.Errorf("TotalTokens() = %d, want %d", got, wantTotal)
	}
	if got := stats.AggregateInputTokens(); got != wantInput {
		t.Errorf("AggregateInputTokens() = %d, want %d", got, wantInput)
	}
	if got := stats.AggregateOutputTokens(); got != wantOutput {
		t.Errorf("AggregateOutputTokens() = %d, want %d", got, wantOutput)
	}
}

func TestAgentStats_AggregateToolCalls(t *testing.T) {
	stats := &AgentStats{
		ToolCallCount: 3,
		Children: []AgentStats{
			{ToolCallCount: 5},
			{
				ToolCallCount: 2,
				Children: []AgentStats{
					{ToolCallCount: 4},
				},
			},
		},
	}

	want := 3 + 5 + 2 + 4
	if got := stats.AggregateToolCalls(); got != want {
		t.Errorf("AggregateToolCalls() = %d, want %d", got, want)
	}
}

func TestAgentStats_AggregateToolCalls_NoChildren(t *testing.T) {
	stats := &AgentStats{ToolCallCount: 7}
	if got := stats.AggregateToolCalls(); got != 7 {
		t.Errorf("AggregateToolCalls() = %d, want 7", got)
	}
}

func TestAgentStats_ActiveTask_DeepestRunning(t *testing.T) {
	stats := &AgentStats{
		TaskLabel: "root",
		Status:    "running",
		Children: []AgentStats{
			{
				TaskLabel: "child-completed",
				Status:    "completed",
			},
			{
				TaskLabel: "child-running",
				Status:    "running",
				Children: []AgentStats{
					{
						TaskLabel: "grandchild-running",
						Status:    "running",
					},
				},
			},
		},
	}

	got := stats.ActiveTask()
	if got != "grandchild-running" {
		t.Errorf("ActiveTask() = %q, want %q", got, "grandchild-running")
	}
}

func TestAgentStats_ActiveTask_RootRunning(t *testing.T) {
	stats := &AgentStats{
		TaskLabel: "root-task",
		Status:    "running",
	}

	got := stats.ActiveTask()
	if got != "root-task" {
		t.Errorf("ActiveTask() = %q, want %q", got, "root-task")
	}
}

func TestAgentStats_ActiveTask_AllCompleted(t *testing.T) {
	stats := &AgentStats{
		TaskLabel: "root",
		Status:    "completed",
		Children: []AgentStats{
			{TaskLabel: "child", Status: "completed"},
		},
	}

	got := stats.ActiveTask()
	if got != "" {
		t.Errorf("ActiveTask() = %q, want empty string when all completed", got)
	}
}

func TestAgentStats_ActiveTask_LastChildRunning(t *testing.T) {
	stats := &AgentStats{
		TaskLabel: "root",
		Status:    "running",
		Children: []AgentStats{
			{TaskLabel: "child1", Status: "completed"},
			{TaskLabel: "child2", Status: "completed"},
			{TaskLabel: "child3", Status: "running"},
		},
	}

	got := stats.ActiveTask()
	if got != "child3" {
		t.Errorf("ActiveTask() = %q, want %q", got, "child3")
	}
}

func TestAgentStats_ActiveTask_MiddleChildRunning(t *testing.T) {
	stats := &AgentStats{
		TaskLabel: "root",
		Status:    "running",
		Children: []AgentStats{
			{TaskLabel: "child1", Status: "completed"},
			{TaskLabel: "child2", Status: "running"},
			{TaskLabel: "child3", Status: "completed"},
		},
	}

	// ActiveTask iterates from last child backwards, so child3 is checked first (completed),
	// then child2 (running) is found.
	got := stats.ActiveTask()
	if got != "child2" {
		t.Errorf("ActiveTask() = %q, want %q", got, "child2")
	}
}

func TestAgentStats_ZeroValues(t *testing.T) {
	stats := &AgentStats{}

	if got := stats.TotalTokens(); got != 0 {
		t.Errorf("TotalTokens() = %d, want 0", got)
	}
	if got := stats.AggregateToolCalls(); got != 0 {
		t.Errorf("AggregateToolCalls() = %d, want 0", got)
	}
	if got := stats.ActiveTask(); got != "" {
		t.Errorf("ActiveTask() = %q, want empty", got)
	}
}
