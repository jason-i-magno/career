package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"
)

// styling is disabled when stdout is not a terminal or NO_COLOR is set, so that
// piping into grep or a file yields clean text.
var styled = func() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}()

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBlue   = "\033[34m"
	ansiCyan   = "\033[36m"
)

func paint(s, code string) string {
	if !styled {
		return s
	}
	return code + s + ansiReset
}

func bold(s string) string   { return paint(s, ansiBold) }
func dim(s string) string    { return paint(s, ansiDim) }
func red(s string) string    { return paint(s, ansiRed) }
func green(s string) string  { return paint(s, ansiGreen) }
func yellow(s string) string { return paint(s, ansiYellow) }
func blue(s string) string   { return paint(s, ansiBlue) }
func cyan(s string) string   { return paint(s, ansiCyan) }

// newTable returns a tabwriter configured for the two-space-gutter column style
// used throughout the CLI.
func newTable(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}

// heading prints a section title with a rule beneath it.
func heading(w io.Writer, title string) {
	fmt.Fprintf(w, "\n%s\n%s\n", bold(title), dim(strings.Repeat("─", len([]rune(title)))))
}

// relDays renders a duration relative to now in the compact form used in
// listings: "today", "in 3d", "5d ago".
func relDays(t, now time.Time) string {
	days := int(t.Truncate(24*time.Hour).Sub(now.Truncate(24*time.Hour)).Hours() / 24)
	switch {
	case days == 0:
		return "today"
	case days == 1:
		return "tomorrow"
	case days == -1:
		return "yesterday"
	case days > 1:
		return fmt.Sprintf("in %dd", days)
	default:
		return fmt.Sprintf("%dd ago", -days)
	}
}

// truncate shortens s to n runes, ending with an ellipsis when cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// wrap breaks s into lines of at most width runes, indenting continuations.
func wrap(s string, width int, indent string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	line := words[0]
	for _, w := range words[1:] {
		if len([]rune(line))+1+len([]rune(w)) > width {
			b.WriteString(line + "\n" + indent)
			line = w
			continue
		}
		line += " " + w
	}
	b.WriteString(line)
	return b.String()
}
