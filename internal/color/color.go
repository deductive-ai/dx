/*
 * Copyright (c) 2023, Deductive AI, Inc. All rights reserved.
 *
 * This software is the confidential and proprietary information of
 * Deductive AI, Inc. You shall not disclose such confidential
 * information and shall use it only in accordance with the terms of
 * the license agreement you entered into with Deductive AI, Inc.
 */

package color

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// ANSI color codes
const (
	Reset     = "\033[0m"
	Bold      = "\033[1m"
	Dim       = "\033[2m"
	Italic    = "\033[3m"
	Underline = "\033[4m"

	// Foreground colors
	Black   = "\033[30m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"

	// Bright foreground colors
	BrightBlack   = "\033[90m"
	BrightRed     = "\033[91m"
	BrightGreen   = "\033[92m"
	BrightYellow  = "\033[93m"
	BrightBlue    = "\033[94m"
	BrightMagenta = "\033[95m"
	BrightCyan    = "\033[96m"
	BrightWhite   = "\033[97m"

	// Background colors
	BgBlack   = "\033[40m"
	BgRed     = "\033[41m"
	BgGreen   = "\033[42m"
	BgYellow  = "\033[43m"
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"
	BgWhite   = "\033[47m"
)

var colorsEnabled = true

func init() {
	// Disable colors on Windows (unless using Windows Terminal or similar)
	if runtime.GOOS == "windows" {
		// Check for modern terminal
		if os.Getenv("WT_SESSION") == "" && os.Getenv("TERM_PROGRAM") == "" {
			colorsEnabled = false
		}
	}

	// Respect NO_COLOR environment variable (standard)
	if os.Getenv("NO_COLOR") != "" {
		colorsEnabled = false
	}

	// Respect DX_NO_COLOR environment variable (app-specific)
	if os.Getenv("DX_NO_COLOR") != "" {
		colorsEnabled = false
	}

	// Check if stdout is a terminal
	if fileInfo, err := os.Stdout.Stat(); err != nil || (fileInfo.Mode()&os.ModeCharDevice) == 0 {
		colorsEnabled = false
	}
}

// SetEnabled allows manually enabling/disabling colors
func SetEnabled(enabled bool) {
	colorsEnabled = enabled
}

// Enabled returns whether colors are enabled
func Enabled() bool {
	return colorsEnabled
}

// wrap wraps text with the given color codes
func wrap(text string, codes ...string) string {
	if !colorsEnabled || len(codes) == 0 {
		return text
	}
	return strings.Join(codes, "") + text + Reset
}

// Title formats text as a title (bold cyan)
func Title(text string) string {
	return wrap(text, Bold, Cyan)
}

// Subtitle formats text as a subtitle (cyan)
func Subtitle(text string) string {
	return wrap(text, Cyan)
}

// Command formats text as a command (yellow)
func Command(text string) string {
	return wrap(text, Yellow)
}

// Output formats text as command output (dim white)
func Output(text string) string {
	return wrap(text, Dim)
}

// Success formats text as success (green)
func Success(text string) string {
	return wrap(text, Green)
}

// Error formats text as error (red)
func Error(text string) string {
	return wrap(text, Red)
}

// Warning formats text as warning (yellow)
func Warning(text string) string {
	return wrap(text, Yellow)
}

// Info formats text as info (blue)
func Info(text string) string {
	return wrap(text, Blue)
}

// Answer formats text as the AI answer (default, no special color)
func Answer(text string) string {
	return text
}

// AnswerMarker returns a styled marker for the final answer
func AnswerMarker() string {
	return wrap("▶ Answer", Bold, BrightGreen)
}

// Progress formats text as progress indicator (magenta)
func Progress(text string) string {
	return wrap(text, Magenta)
}

// ToolName formats a tool name (bold yellow)
func ToolName(text string) string {
	return wrap(text, Bold, Yellow)
}

// ToolOutput formats tool output (green, slightly dim)
func ToolOutput(text string) string {
	return wrap(text, Green)
}

// Prompt formats the interactive prompt
func Prompt(text string) string {
	return wrap(text, Bold, BrightCyan)
}

// SessionID formats session ID display
func SessionID(text string) string {
	return wrap(text, Dim)
}

// URL formats URLs
func URL(text string) string {
	return wrap(text, Underline, Blue)
}

// Sprintf is like fmt.Sprintf but applies color to the format result
func Sprintf(colorFunc func(string) string, format string, args ...interface{}) string {
	return colorFunc(fmt.Sprintf(format, args...))
}

// Muted formats text as dim/muted (alias for Output)
func Muted(text string) string {
	return wrap(text, BrightBlack)
}

// StatsLine formats the live stats bar (dim)
func StatsLine(text string) string {
	return wrap(text, Dim)
}

// ProgressReport formats progress report text (italic cyan)
func ProgressReport(text string) string {
	return wrap(text, Italic, Cyan)
}

// ProgressBorder formats the left border character for progress reports
func ProgressBorder() string {
	return wrap("┃", BrightBlack)
}
