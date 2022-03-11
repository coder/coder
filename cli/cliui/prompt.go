package cliui

import (
	"errors"
	"fmt"
	"io"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

type PromptOptions struct {
	Text      string
	Default   string
	Mask      rune
	Validate  Validate
	IsConfirm bool
}

// Prompt displays an input for user data.
// See this live: "go run ./cmd/cliui prompt"
func Prompt(cmd *cobra.Command, opts PromptOptions) (string, error) {
	prompt := &promptui.Prompt{
		Label:     opts.Text,
		Default:   opts.Default,
		Mask:      opts.Mask,
		Validate:  promptui.ValidateFunc(opts.Validate),
		IsConfirm: opts.IsConfirm,
	}

	prompt.Stdin = io.NopCloser(cmd.InOrStdin())
	prompt.Stdout = readWriteCloser{
		Writer: cmd.OutOrStdout(),
	}

	// The prompt library displays defaults in a jarring way for the user
	// by attempting to autocomplete it. This sets no default enabling us
	// to customize the display.
	defaultValue := prompt.Default
	if !prompt.IsConfirm {
		prompt.Default = ""
	}

	// Rewrite the confirm template to remove bold, and fit to the Coder style.
	confirmEnd := fmt.Sprintf("[y/%s] ", Styles.Bold.Render("N"))
	if prompt.Default == "y" {
		confirmEnd = fmt.Sprintf("[%s/n] ", Styles.Bold.Render("Y"))
	}
	confirm := Styles.FocusedPrompt.String() + `{{ . }} ` + confirmEnd

	// Customize to remove bold.
	valid := Styles.FocusedPrompt.String() + "{{ . }} "
	if defaultValue != "" {
		valid += fmt.Sprintf("(%s) ", defaultValue)
	}

	success := valid
	invalid := valid
	if prompt.IsConfirm {
		success = confirm
		invalid = confirm
	}

	prompt.Templates = &promptui.PromptTemplates{
		Confirm:         confirm,
		Success:         success,
		Invalid:         invalid,
		Valid:           valid,
		ValidationError: defaultStyles.Error.Render("{{ . }}"),
	}
	oldValidate := prompt.Validate
	if oldValidate != nil {
		// Override the validate function to pass our default!
		prompt.Validate = func(s string) error {
			if s == "" {
				s = defaultValue
			}
			return oldValidate(s)
		}
	}
	value, err := prompt.Run()
	if value == "" && !prompt.IsConfirm {
		value = defaultValue
	}
	if errors.Is(err, promptui.ErrInterrupt) || errors.Is(err, promptui.ErrAbort) {
		return "", Canceled
	}
	return value, err
}

// readWriteCloser fakes reads, writes, and closing!
type readWriteCloser struct {
	io.Reader
	io.Writer
	io.Closer
}
