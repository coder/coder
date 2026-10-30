package main

import (
	"io"

	"golang.org/x/xerrors"

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

// prompter asks the operator questions. In non-interactive mode it
// answers every prompt with its default, which is what pressing
// Enter does interactively.
type prompter struct {
	inv            *serpent.Invocation
	nonInteractive bool
}

// prompt behaves like cliui.Prompt. In non-interactive mode it
// returns the default answer and logs it, and fails when the prompt
// has no default. A declined confirm returns an error wrapping
// cliui.ErrCanceled, matching cliui.Prompt.
func (p *prompter) prompt(opts cliui.PromptOptions) (string, error) {
	if !p.nonInteractive {
		return cliui.Prompt(p.inv, opts)
	}

	answer := opts.Default
	if opts.IsConfirm && answer == "" {
		answer = cliui.ConfirmYes
	}
	if answer == "" {
		return "", xerrors.Errorf("%q requires input, which is not available in non-interactive mode", opts.Text)
	}
	infof(p.inv.Stderr, "%s %s (non-interactive default)", opts.Text, answer)

	if opts.IsConfirm && answer != cliui.ConfirmYes {
		return answer, xerrors.Errorf("%q declined by default in non-interactive mode: %w", opts.Text, cliui.ErrCanceled)
	}
	if opts.Validate != nil {
		if err := opts.Validate(answer); err != nil {
			return "", xerrors.Errorf("default answer %q for %q is invalid: %w", answer, opts.Text, err)
		}
	}
	return answer, nil
}

// confirm asks a yes/no question that defaults to yes. Returns nil if
// the user confirms, or a cancellation error otherwise.
func (p *prompter) confirm(msg string) error {
	_, err := p.prompt(cliui.PromptOptions{
		Text:      msg,
		IsConfirm: true,
	})
	return err
}

// confirmWithDefault asks a yes/no question with the specified
// default ("yes" or "no").
func (p *prompter) confirmWithDefault(msg, def string) error {
	_, err := p.prompt(cliui.PromptOptions{
		Text:      msg,
		IsConfirm: true,
		Default:   def,
	})
	return err
}
