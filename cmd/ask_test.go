package cmd

import (
	"testing"
)

func TestFormatByteSize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int
		want  string
	}{
		{"zero bytes", 0, "0 bytes"},
		{"one byte", 1, "1 byte"},
		{"500 bytes", 500, "500 bytes"},
		{"1023 bytes", 1023, "1023 bytes"},
		{"exactly 1 KB", 1024, "1.0 KB"},
		{"1.5 KB", 1536, "1.5 KB"},
		{"10 KB", 10240, "10.0 KB"},
		{"exactly 1 MB", 1048576, "1.0 MB"},
		{"1.5 MB", 1572864, "1.5 MB"},
		{"10 MB", 10485760, "10.0 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatByteSize(tt.bytes)
			if got != tt.want {
				t.Errorf("formatByteSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestVisibleLen(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want int
	}{
		{"plain text", "hello", 5},
		{"empty string", "", 0},
		{"with ANSI bold", "\033[1mhello\033[0m", 5},
		{"with ANSI color", "\033[31mred text\033[0m", 8},
		{"multiple ANSI codes", "\033[1m\033[36mhello\033[0m", 5},
		{"mixed plain and ANSI", "hi \033[32mgreen\033[0m end", 12},
		{"only ANSI codes", "\033[1m\033[0m", 0},
		{"unicode chars", "hello 世界", 8},
		{"with spaces", "  hello  ", 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := visibleLen(tt.s)
			if got != tt.want {
				t.Errorf("visibleLen(%q) = %d, want %d", tt.s, got, tt.want)
			}
		})
	}
}

func TestOutputState_ClearStatsLine_NoOp(t *testing.T) {
	state := &OutputState{isTTY: false, statsLineLen: 10}
	state.clearStatsLine()
	// Should not panic or change anything meaningful when not TTY
}

func TestOutputState_PrintStatsLine_NoTTY(t *testing.T) {
	state := &OutputState{isTTY: false}
	// Should be a no-op for non-TTY
	state.printStatsLine(nil)
}
