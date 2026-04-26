package color

import (
	"testing"
)

func TestSetEnabled_And_Enabled(t *testing.T) {
	original := Enabled()
	defer SetEnabled(original)

	SetEnabled(true)
	if !Enabled() {
		t.Error("Enabled() should return true after SetEnabled(true)")
	}

	SetEnabled(false)
	if Enabled() {
		t.Error("Enabled() should return false after SetEnabled(false)")
	}
}

func TestColorFunctions_Disabled(t *testing.T) {
	original := Enabled()
	defer SetEnabled(original)
	SetEnabled(false)

	tests := []struct {
		name string
		fn   func(string) string
		text string
	}{
		{"Title", Title, "hello"},
		{"Subtitle", Subtitle, "hello"},
		{"Command", Command, "hello"},
		{"Output", Output, "hello"},
		{"Success", Success, "hello"},
		{"Error", Error, "hello"},
		{"Warning", Warning, "hello"},
		{"Info", Info, "hello"},
		{"Progress", Progress, "hello"},
		{"ToolName", ToolName, "hello"},
		{"ToolOutput", ToolOutput, "hello"},
		{"Prompt", Prompt, "hello"},
		{"SessionID", SessionID, "hello"},
		{"URL", URL, "hello"},
		{"Muted", Muted, "hello"},
		{"StatsLine", StatsLine, "hello"},
		{"ProgressReport", ProgressReport, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.text)
			if got != tt.text {
				t.Errorf("%s(%q) with colors disabled = %q, want %q", tt.name, tt.text, got, tt.text)
			}
		})
	}
}

func TestColorFunctions_Enabled(t *testing.T) {
	original := Enabled()
	defer SetEnabled(original)
	SetEnabled(true)

	tests := []struct {
		name string
		fn   func(string) string
		text string
	}{
		{"Title", Title, "hello"},
		{"Subtitle", Subtitle, "hello"},
		{"Command", Command, "hello"},
		{"Success", Success, "hello"},
		{"Error", Error, "hello"},
		{"Warning", Warning, "hello"},
		{"Info", Info, "hello"},
		{"Progress", Progress, "hello"},
		{"Muted", Muted, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.text)
			if got == tt.text {
				t.Errorf("%s(%q) with colors enabled should wrap with ANSI codes", tt.name, tt.text)
			}
			if len(got) <= len(tt.text) {
				t.Errorf("%s(%q) result length %d should be > %d", tt.name, tt.text, len(got), len(tt.text))
			}
			// Should contain the original text
			if !containsText(got, tt.text) {
				t.Errorf("%s(%q) result %q should contain original text", tt.name, tt.text, got)
			}
			// Should end with reset code
			if !hasSuffix(got, Reset) {
				t.Errorf("%s(%q) result should end with Reset code", tt.name, tt.text)
			}
		})
	}
}

func TestAnswer_NoWrapping(t *testing.T) {
	original := Enabled()
	defer SetEnabled(original)

	SetEnabled(true)
	got := Answer("test")
	if got != "test" {
		t.Errorf("Answer(%q) = %q, want %q (no color wrapping)", "test", got, "test")
	}

	SetEnabled(false)
	got = Answer("test")
	if got != "test" {
		t.Errorf("Answer(%q) = %q, want %q (no color wrapping)", "test", got, "test")
	}
}

func TestSprintf(t *testing.T) {
	original := Enabled()
	defer SetEnabled(original)
	SetEnabled(false)

	got := Sprintf(Success, "count: %d", 42)
	if got != "count: 42" {
		t.Errorf("Sprintf(Success, ...) = %q, want %q", got, "count: 42")
	}

	SetEnabled(true)
	got = Sprintf(Success, "count: %d", 42)
	if !containsText(got, "count: 42") {
		t.Errorf("Sprintf(Success, ...) should contain formatted text, got %q", got)
	}
}

func TestProgressBorder(t *testing.T) {
	original := Enabled()
	defer SetEnabled(original)

	SetEnabled(true)
	got := ProgressBorder()
	if !containsText(got, "┃") {
		t.Errorf("ProgressBorder() = %q, should contain ┃", got)
	}

	SetEnabled(false)
	got = ProgressBorder()
	if got != "┃" {
		t.Errorf("ProgressBorder() disabled = %q, want %q", got, "┃")
	}
}

func TestAnswerMarker(t *testing.T) {
	original := Enabled()
	defer SetEnabled(original)

	SetEnabled(true)
	got := AnswerMarker()
	if !containsText(got, "Answer") {
		t.Errorf("AnswerMarker() should contain 'Answer', got %q", got)
	}

	SetEnabled(false)
	got = AnswerMarker()
	if got != "▶ Answer" {
		t.Errorf("AnswerMarker() disabled = %q, want %q", got, "▶ Answer")
	}
}

func TestWrap_EmptyCodes(t *testing.T) {
	original := Enabled()
	defer SetEnabled(original)
	SetEnabled(true)

	got := wrap("test")
	if got != "test" {
		t.Errorf("wrap with no codes = %q, want %q", got, "test")
	}
}

func containsText(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func hasSuffix(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}
