package clog

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/xerrors"
)

// CLIMessage provides a human-readable message for CLI errors and messages.
type CLIMessage struct {
	Level  string
	Color  lipgloss.Color
	Header string
	Lines  []string
}

// CLIError wraps a CLIMessage and allows consumers to treat it as a normal error.
type CLIError struct {
	CLIMessage
	error
}

// String formats the CLI message for consumption by a human.
func (m CLIMessage) String() string {
	var str strings.Builder
	_, _ = fmt.Fprintf(&str, "%s: %s\r\n",
		lipgloss.NewStyle().Foreground(m.Color).Render(m.Level),
		Bold(m.Header))
	for _, line := range m.Lines {
		_, _ = fmt.Fprintf(&str, "  %s %s\r\n", lipgloss.NewStyle().Foreground(m.Color).Render("|"), line)
	}
	return str.String()
}

// Log logs the given error to stderr, defaulting to "fatal" if the error is not a CLIError.
// If the error is a CLIError, the plain error chain is ignored and the CLIError
// is logged on its own.
func Log(writer io.Writer, err error) {
	var cliErr CLIError
	if !xerrors.As(err, &cliErr) {
		cliErr = Fatal(err.Error())
	}
	_, _ = fmt.Fprintln(writer, cliErr.String())
}

// LogInfo prints the given info message to stderr.
func LogInfo(writer io.Writer, header string, lines ...string) {
	_, _ = fmt.Fprint(writer, CLIMessage{
		Level:  "info",
		Color:  lipgloss.Color("34"),
		Header: header,
		Lines:  lines,
	}.String())
}

// LogSuccess prints the given info message to stderr.
func LogSuccess(writer io.Writer, header string, lines ...string) {
	_, _ = fmt.Fprint(writer, CLIMessage{
		Level:  "success",
		Color:  lipgloss.Color("32"),
		Header: header,
		Lines:  lines,
	}.String())
}

// LogWarn prints the given warn message to stderr.
func LogWarn(writer io.Writer, header string, lines ...string) {
	_, _ = fmt.Fprint(writer, CLIMessage{
		Level:  "warning",
		Color:  lipgloss.Color("33"),
		Header: header,
		Lines:  lines,
	}.String())
}

// Error creates an error with the level "error".
func Error(header string, lines ...string) CLIError {
	return CLIError{
		CLIMessage: CLIMessage{
			Color:  lipgloss.Color("31"),
			Level:  "error",
			Header: header,
			Lines:  lines,
		},
		error: xerrors.New(header),
	}
}

// Fatal creates an error with the level "fatal".
func Fatal(header string, lines ...string) CLIError {
	return CLIError{
		CLIMessage: CLIMessage{
			Color:  lipgloss.Color("31"),
			Level:  "fatal",
			Header: header,
			Lines:  lines,
		},
		error: xerrors.New(header),
	}
}

// Bold provides a convenience wrapper around color.New for brevity when logging.
func Bold(a string) string {
	return lipgloss.NewStyle().Bold(true).Render(a)
}

// Tipf formats according to the given format specifier and prepends a bolded "tip: " header.
func Tipf(format string, a ...interface{}) string {
	return fmt.Sprintf("%s %s", Bold("tip:"), fmt.Sprintf(format, a...))
}

// Hintf formats according to the given format specifier and prepends a bolded "hint: " header.
func Hintf(format string, a ...interface{}) string {
	return fmt.Sprintf("%s %s", Bold("hint:"), fmt.Sprintf(format, a...))
}

// Causef formats according to the given format specifier and prepends a bolded "cause: " header.
func Causef(format string, a ...interface{}) string {
	return fmt.Sprintf("%s %s", Bold("cause:"), fmt.Sprintf(format, a...))
}

// BlankLine is an empty string meant to be used in CLIMessage and CLIError construction.
const BlankLine = ""
