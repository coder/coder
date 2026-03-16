package main

import (
	"io"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

// outputPrefix is prepended to every message line. Set to
// "[DRYRUN] " when running in dry-run mode.
var outputPrefix string

// warnf prints a yellow warning to stderr.
func warnf(w io.Writer, format string, args ...any) {
	pretty.Fprintf(w, cliui.DefaultStyles.Warn, outputPrefix+format+"\n", args...)
}

// infof prints a cyan info message to stderr.
func infof(w io.Writer, format string, args ...any) {
	pretty.Fprintf(w, cliui.DefaultStyles.Keyword, outputPrefix+format+"\n", args...)
}

// successf prints a green success message to stderr.
func successf(w io.Writer, format string, args ...any) {
	pretty.Fprintf(w, cliui.DefaultStyles.DateTimeStamp, outputPrefix+format+"\n", args...)
}

// confirm asks a yes/no question. Returns nil if the user confirms,
// or a cancellation error otherwise.
func confirm(inv *serpent.Invocation, msg string) error {
	_, err := cliui.Prompt(inv, cliui.PromptOptions{
		Text:      msg,
		IsConfirm: true,
	})
	return err
}

// confirmWithDefault asks a yes/no question with the specified
// default ("yes" or "no").
func confirmWithDefault(inv *serpent.Invocation, msg, def string) error {
	_, err := cliui.Prompt(inv, cliui.PromptOptions{
		Text:      msg,
		IsConfirm: true,
		Default:   def,
	})
	return err
}
