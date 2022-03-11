package cliui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

type PromptOptions struct {
	Text          string
	Default       string
	CharLimit     int
	Validate      Validate
	EchoMode      textinput.EchoMode
	EchoCharacter rune
	IsConfirm     bool
}

func Prompt(cmd *cobra.Command, opts PromptOptions) (string, error) {
	input := &inputModel{
		opts:  opts,
		model: textinput.New(),
	}
	input.model.Prompt = opts.Text + " "
	input.model.Placeholder = opts.Default
	input.model.CharLimit = opts.CharLimit
	input.model.EchoCharacter = opts.EchoCharacter
	input.model.EchoMode = opts.EchoMode
	input.model.Focus()

	program := tea.NewProgram(input, tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.OutOrStdout()))
	err := program.Start()
	if err != nil {
		return "", err
	}
	if input.canceled {
		return "", Canceled
	}
	if opts.IsConfirm && !strings.EqualFold(input.model.Value(), "yes") {
		return "", Canceled
	}
	return input.model.Value(), nil
}

type inputModel struct {
	opts     PromptOptions
	model    textinput.Model
	err      string
	canceled bool
}

func (*inputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (i *inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			// Apply the default placeholder value.
			if i.model.Value() == "" && i.model.Placeholder != "" {
				i.model.SetValue(i.model.Placeholder)
			}

			// Validate the value.
			if i.opts.Validate != nil {
				err := i.opts.Validate(i.model.Value())
				if err != nil {
					i.err = err.Error()
					return i, nil
				}
			}
			i.err = ""
			i.model.SetCursorMode(textinput.CursorHide)

			return i, tea.Quit
		case tea.KeyCtrlC, tea.KeyEsc:
			i.canceled = true
			i.model.SetCursorMode(textinput.CursorHide)
			return i, tea.Quit
		}

	// We handle errors just like any other message
	case error:
		i.err = msg.Error()
		return i, nil
	}

	i.model, cmd = i.model.Update(msg)
	return i, cmd
}

func (i *inputModel) View() string {
	prompt := Styles.FocusedPrompt
	if i.model.CursorMode() == textinput.CursorHide {
		prompt = Styles.Prompt
	}
	validate := ""
	if i.err != "" {
		validate = "\n" + defaultStyles.Error.Render("▲ "+i.err)
	}
	return prompt.String() + i.model.View() + "\n" + validate
}
