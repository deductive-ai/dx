package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("failed to write script %s: %v", name, err)
	}
	return path
}

func TestRunHooks_EmptyList(t *testing.T) {
	output, err := RunHooks([]string{})
	if err != nil {
		t.Fatalf("RunHooks([]) error: %v", err)
	}
	if output != "" {
		t.Errorf("RunHooks([]) = %q, want empty string", output)
	}
}

func TestRunHooks_NilList(t *testing.T) {
	output, err := RunHooks(nil)
	if err != nil {
		t.Fatalf("RunHooks(nil) error: %v", err)
	}
	if output != "" {
		t.Errorf("RunHooks(nil) = %q, want empty string", output)
	}
}

func TestRunHooks_SuccessfulHook(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "hook.sh", "#!/bin/bash\necho 'hello from hook'")

	output, err := RunHooks([]string{script})
	if err != nil {
		t.Fatalf("RunHooks() error: %v", err)
	}
	if output != "hello from hook" {
		t.Errorf("RunHooks() = %q, want %q", output, "hello from hook")
	}
}

func TestRunHooks_FailingHook(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "fail.sh", "#!/bin/bash\nexit 1")

	_, err := RunHooks([]string{script})
	if err == nil {
		t.Fatal("RunHooks() should return error for failing hook")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("error should describe failure, got %q", err.Error())
	}
}

func TestRunHooks_MultipleHooks_CombinedOutput(t *testing.T) {
	dir := t.TempDir()
	script1 := writeScript(t, dir, "hook1.sh", "#!/bin/bash\necho 'output1'")
	script2 := writeScript(t, dir, "hook2.sh", "#!/bin/bash\necho 'output2'")

	output, err := RunHooks([]string{script1, script2})
	if err != nil {
		t.Fatalf("RunHooks() error: %v", err)
	}
	if !strings.Contains(output, "output1") {
		t.Error("output should contain output from first hook")
	}
	if !strings.Contains(output, "output2") {
		t.Error("output should contain output from second hook")
	}
	if !strings.Contains(output, "---") {
		t.Error("output should contain separator between hooks")
	}
}

func TestRunHooks_MixedSuccessAndFailure(t *testing.T) {
	dir := t.TempDir()
	good := writeScript(t, dir, "good.sh", "#!/bin/bash\necho 'good output'")
	bad := writeScript(t, dir, "bad.sh", "#!/bin/bash\nexit 1")

	output, err := RunHooks([]string{good, bad})
	if err == nil {
		t.Fatal("RunHooks() should return error when any hook fails")
	}
	if !strings.Contains(output, "good output") {
		t.Error("output should contain successful hook output")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("error should describe failure, got %q", err.Error())
	}
}

func TestRunHooks_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}

	dir := t.TempDir()
	script := writeScript(t, dir, "slow.sh", "#!/bin/bash\nsleep 60")

	_, err := RunHooks([]string{script})
	if err == nil {
		t.Fatal("RunHooks() should return error for timed-out hook")
	}
}

func TestRunHooks_EmptyOutput(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "empty.sh", "#!/bin/bash\n# no output")

	output, err := RunHooks([]string{script})
	if err != nil {
		t.Fatalf("RunHooks() error: %v", err)
	}
	if output != "" {
		t.Errorf("RunHooks() = %q, want empty string for hook with no output", output)
	}
}

func TestRunHooks_MultilineOutput(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "multi.sh", "#!/bin/bash\necho 'line1'\necho 'line2'\necho 'line3'")

	output, err := RunHooks([]string{script})
	if err != nil {
		t.Fatalf("RunHooks() error: %v", err)
	}
	if !strings.Contains(output, "line1") || !strings.Contains(output, "line3") {
		t.Errorf("RunHooks() should contain all output lines, got %q", output)
	}
}

func TestFormatAppendix(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"non-empty", "some data", "<appendix>\nsome data\n</appendix>"},
		{"empty", "", ""},
		{"multiline", "line1\nline2", "<appendix>\nline1\nline2\n</appendix>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatAppendix(tt.output)
			if got != tt.want {
				t.Errorf("FormatAppendix(%q) = %q, want %q", tt.output, got, tt.want)
			}
		})
	}
}

func TestShouldIncludeAppendix(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		previous string
		want     bool
	}{
		{"different output", "new data", "old data", true},
		{"same output", "same", "same", false},
		{"empty current", "", "old data", false},
		{"empty previous", "new data", "", true},
		{"both empty", "", "", false},
		{"first run", "initial output", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldIncludeAppendix(tt.current, tt.previous)
			if got != tt.want {
				t.Errorf("ShouldIncludeAppendix(%q, %q) = %v, want %v", tt.current, tt.previous, got, tt.want)
			}
		})
	}
}
