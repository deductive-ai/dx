package cmd

import (
	"testing"
)

func TestRemoveCommentLines_OnlyComments(t *testing.T) {
	input := `# This is a comment
# Another comment
# Third comment`

	got := removeCommentLines(input)
	if got != "" {
		t.Errorf("removeCommentLines() = %q, want empty string", got)
	}
}

func TestRemoveCommentLines_NoComments(t *testing.T) {
	input := `Hello world
This is content
More content`

	got := removeCommentLines(input)
	want := "Hello world\nThis is content\nMore content"
	if got != want {
		t.Errorf("removeCommentLines() = %q, want %q", got, want)
	}
}

func TestRemoveCommentLines_MixedContent(t *testing.T) {
	input := `# Comment line
I am a DBA
# Another comment
I work with PostgreSQL
# Final comment`

	got := removeCommentLines(input)
	want := "I am a DBA\nI work with PostgreSQL"
	if got != want {
		t.Errorf("removeCommentLines() = %q, want %q", got, want)
	}
}

func TestRemoveCommentLines_EmptyInput(t *testing.T) {
	got := removeCommentLines("")
	if got != "" {
		t.Errorf("removeCommentLines(\"\") = %q, want empty string", got)
	}
}

func TestRemoveCommentLines_TrimsWhitespace(t *testing.T) {
	input := `# Comment

  Hello world  

# Comment`

	got := removeCommentLines(input)
	// After removing comment lines and trimming, leading/trailing whitespace
	// from the overall result should be removed
	if got == "" {
		t.Error("removeCommentLines() should not return empty for input with content")
	}
	// The content "Hello world" with surrounding blank lines should be trimmed
	if len(got) > 0 && (got[0] == ' ' || got[0] == '\n') {
		t.Errorf("removeCommentLines() result has leading whitespace: %q", got)
	}
	if len(got) > 0 && (got[len(got)-1] == ' ' || got[len(got)-1] == '\n') {
		t.Errorf("removeCommentLines() result has trailing whitespace: %q", got)
	}
}

func TestRemoveCommentLines_PreservesHashInMiddle(t *testing.T) {
	input := `Title with # inside
Value is #123`

	got := removeCommentLines(input)
	want := "Title with # inside\nValue is #123"
	if got != want {
		t.Errorf("removeCommentLines() = %q, want %q", got, want)
	}
}

func TestRemoveCommentLines_HelpTextTemplate(t *testing.T) {
	input := `# Enter your role/persona below.
# This will be sent as the first message in new sessions.
# Lines starting with # will be removed.
#
# Example:
# I am a DevOps engineer debugging production issues.
# I primarily work with Kubernetes, AWS, and PostgreSQL.
# I prefer concise, actionable responses.

I am a senior DBA working with PostgreSQL and MySQL.
I prefer detailed, thorough responses with examples.`

	got := removeCommentLines(input)
	if got == "" {
		t.Fatal("removeCommentLines() should not return empty for template with content")
	}
	if containsString(got, "# ") {
		t.Error("removeCommentLines() should remove all comment lines")
	}
	if !containsString(got, "senior DBA") {
		t.Error("removeCommentLines() should preserve non-comment content")
	}
}

func TestRemoveCommentLines_OnlyWhitespace(t *testing.T) {
	input := "   \n  \n   "
	got := removeCommentLines(input)
	if got != "" {
		t.Errorf("removeCommentLines() with only whitespace = %q, want empty", got)
	}
}

func TestRemoveCommentLines_TrailingNewline(t *testing.T) {
	input := "Hello\n"
	got := removeCommentLines(input)
	if got != "Hello" {
		t.Errorf("removeCommentLines(%q) = %q, want %q", input, got, "Hello")
	}
}

func TestRemoveCommentLines_SingleLine(t *testing.T) {
	input := "Just one line"
	got := removeCommentLines(input)
	if got != "Just one line" {
		t.Errorf("removeCommentLines(%q) = %q, want %q", input, got, "Just one line")
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
