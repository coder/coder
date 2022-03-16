package cliui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/charm/ui/common"
	"github.com/charmbracelet/lipgloss"
	"github.com/coder/coder/pty"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

var (
	Canceled = xerrors.New("canceled")

	defaultStyles = common.DefaultStyles()
)

type Validate func(string) error

// ValidateNotEmpty is a helper function to disallow empty inputs!
func ValidateNotEmpty(s string) error {
	if s == "" {
		return xerrors.New("Must be provided!")
	}
	return nil
}

// Styles compose visual elements of the UI!
var Styles = struct {
	Bold,
	Code,
	Field,
	Keyword,
	Paragraph,
	Prompt,
	FocusedPrompt,
	Logo,
	Wrap lipgloss.Style
}{
	Bold:          lipgloss.NewStyle().Bold(true),
	Code:          defaultStyles.Code,
	Field:         defaultStyles.Code.Copy().Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}),
	Keyword:       defaultStyles.Keyword,
	Paragraph:     defaultStyles.Paragraph,
	Prompt:        defaultStyles.Prompt.Foreground(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#5C5C5C"}),
	FocusedPrompt: defaultStyles.FocusedPrompt.Foreground(lipgloss.Color("#651fff")),
	Logo:          defaultStyles.Logo.SetString("Coder"),
	Wrap:          defaultStyles.Wrap,
}

func startProgram(cmd *cobra.Command, model tea.Model) error {
	readWriter := cmd.InOrStdin().(pty.ReadWriter)
	cancelReader, err := readWriter.CancelReader()
	if err != nil {
		return err
	}
	ctx, cancelFunc := context.WithCancel(cmd.Context())
	defer cancelFunc()
	program := tea.NewProgram(model, tea.WithInput(cancelReader), tea.WithOutput(cmd.OutOrStdout()))
	go func() {
		<-ctx.Done()
		program.Quit()
	}()
	return program.Start()
}
