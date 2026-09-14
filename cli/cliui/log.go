package cliui

import (
	"fmt"
	"io"
	"strings"

	"github.com/coder/pretty"
)

// cliMessage provides a human-readable message for CLI errors and messages.
type cliMessage struct {
	Style  pretty.Style
	Header string
	Prefix string
	Lines  []string
}

// String formats the CLI message for consumption by a human.
func (m cliMessage) String() string {
	var str strings.Builder

	if m.Prefix != "" {
		_, _ = str.WriteString(Bold(m.Prefix))
	}

	pretty.Fprint(&str, m.Style, m.Header)
	_, _ = str.WriteString("\r\n")
	for _, line := range m.Lines {
		_, _ = fmt.Fprintf(&str, "  %s %s\r\n", pretty.Sprint(m.Style, "|"), line)
	}
	return str.String()
}

// Warn writes a log to the writer provided.
func Warn(wtr io.Writer, header string, lines ...string) {
	_, _ = fmt.Fprint(wtr, cliMessage{
		Style:  DefaultStyles.Warn,
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
		Style:  DefaultStyles.Error,
		Prefix: "ERROR: ",
		Header: header,
		Lines:  lines,
	}.String())
}

// Errorf writes a formatted log to the writer provided.
func Errorf(wtr io.Writer, fmtStr string, args ...interface{}) {
	Error(wtr, fmt.Sprintf(fmtStr, args...))
}
