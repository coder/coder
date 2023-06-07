package cliui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// cliMessage provides a human-readable message for CLI errors and messages.
type cliMessage struct {
	Style  lipgloss.Style
	Header string
	Prefix string
	Lines  []string
}

// String formats the CLI message for consumption by a human.
func (m cliMessage) String() string {
	var str strings.Builder

	if m.Prefix != "" {
		_, _ = str.WriteString(m.Style.Bold(true).Render(m.Prefix))
	}

	_, _ = str.WriteString(m.Style.Bold(false).Render(m.Header))
	_, _ = str.WriteString("\r\n")
	for _, line := range m.Lines {
		_, _ = fmt.Fprintf(&str, "  %s %s\r\n", m.Style.Render("|"), line)
	}
	return str.String()
}

// Warn writes a log to the writer provided.
func Warn(wtr io.Writer, header string, lines ...string) {
	_, _ = fmt.Fprint(wtr, cliMessage{
		Style:  DefaultStyles.Warn.Copy(),
		Prefix: "WARN: ",
		Header: header,
		Lines:  lines,
	}.String())
}

// Warn writes a formatted log to the writer provided.
func Warnf(wtr io.Writer, fmtStr string, args ...interface{}) {
	Warn(wtr, fmt.Sprintf(fmtStr, args...))
}

// Info writes a log to the writer provided.
func Info(wtr io.Writer, header string, lines ...string) {
	_, _ = fmt.Fprint(wtr, cliMessage{
		Header: header,
		Lines:  lines,
	}.String())
}

// Infof writes a formatted log to the writer provided.
func Infof(wtr io.Writer, fmtStr string, args ...interface{}) {
	Info(wtr, fmt.Sprintf(fmtStr, args...))
}

// Error writes a log to the writer provided.
func Error(wtr io.Writer, header string, lines ...string) {
	_, _ = fmt.Fprint(wtr, cliMessage{
		Style:  DefaultStyles.Error.Copy(),
		Prefix: "ERROR: ",
		Header: header,
		Lines:  lines,
	}.String())
}

// Errorf writes a formatted log to the writer provided.
func Errorf(wtr io.Writer, fmtStr string, args ...interface{}) {
	Error(wtr, fmt.Sprintf(fmtStr, args...))
}
