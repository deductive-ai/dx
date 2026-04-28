package render

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var (
	styleError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true)

	styleSuccess = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true)

	styleMuted = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	styleURL = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Underline(true)

	styleViewBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			PaddingTop(1)
)

// Markdown renders a markdown string for terminal display using glamour.
// Falls back to plain text if rendering fails.
func Markdown(content string) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		return content
	}
	out, err := r.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimSpace(out)
}

// MarkdownWriter creates a writer that renders markdown content line-by-line
// for streaming output. When the complete flag is set, the accumulated content
// is re-rendered as a whole for final display.
type MarkdownWriter struct {
	w      io.Writer
	buffer strings.Builder
}

// NewMarkdownWriter wraps w to enable streaming markdown output.
func NewMarkdownWriter(w io.Writer) *MarkdownWriter {
	return &MarkdownWriter{w: w}
}

// Write accumulates content and writes it through.
func (mw *MarkdownWriter) Write(p []byte) (n int, err error) {
	mw.buffer.Write(p)
	return mw.w.Write(p)
}

// Flush renders the complete accumulated content as markdown.
// Call this after all streaming is done for a polished final output.
func (mw *MarkdownWriter) Flush() string {
	return Markdown(mw.buffer.String())
}

// Thinking creates and starts a spinner to indicate the agent is thinking.
// Returns the spinner so the caller can stop it.
func Thinking() *spinner.Spinner {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = styleMuted.Render("  Thinking...")
	s.Writer = os.Stderr
	s.Start()
	return s
}

// ThinkingWithMessage creates a spinner with a custom message.
func ThinkingWithMessage(msg string) *spinner.Spinner {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = styleMuted.Render("  " + msg)
	s.Writer = os.Stderr
	s.Start()
	return s
}

// Success prints a success message.
func Success(msg string) string {
	return styleSuccess.Render("✓") + " " + msg
}

// Error prints an error message.
func Error(msg string) string {
	return styleError.Render("✗") + " " + msg
}

// ViewURL formats the session URL for display.
func ViewURL(url string) string {
	return styleViewBar.Render("View: " + styleURL.Render(url))
}

// AnswerSeparator returns a separator line before the answer.
func AnswerSeparator() string {
	return styleMuted.Render("─── Answer ───")
}

// FormatTable renders a simple table from headers and rows.
func FormatTable(headers []string, rows [][]string) string {
	if len(headers) == 0 {
		return ""
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	var sb strings.Builder

	// Header
	for i, h := range headers {
		if i > 0 {
			sb.WriteString("  ")
		}
		_, _ = fmt.Fprintf(&sb, "%-*s", widths[i], h)
	}
	sb.WriteString("\n")

	// Separator
	for i, w := range widths {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(strings.Repeat("─", w))
	}
	sb.WriteString("\n")

	// Rows
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				sb.WriteString("  ")
			}
			if i < len(widths) {
				_, _ = fmt.Fprintf(&sb, "%-*s", widths[i], cell)
			} else {
				sb.WriteString(cell)
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// StatusIndicator returns a colored status dot.
func StatusIndicator(ok bool) string {
	if ok {
		return styleSuccess.Render("●")
	}
	return styleError.Render("●")
}
