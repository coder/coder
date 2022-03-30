package cliui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// cliMessage provides a human-readable message for CLI errors and messages.
type cliMessage struct {
	Level  string
	Style  lipgloss.Style
	Header string
	Lines  []string
}

// String formats the CLI message for consumption by a human.
func (m cliMessage) String() string {
	var str strings.Builder
	_, _ = fmt.Fprintf(&str, "%s\r\n",
		Styles.Bold.Render(m.Header))
	for _, line := range m.Lines {
		_, _ = fmt.Fprintf(&str, "  %s %s\r\n", m.Style.Render("|"), line)
	}
	return str.String()
}

// Warn writes a log to the writer provided.
func Warn(wtr io.Writer, header string, lines ...string) {
	_, _ = fmt.Fprint(wtr, cliMessage{
		Level:  "warning",
		Style:  Styles.Warn,
		Header: header,
		Lines:  lines,
	}.String())
}
