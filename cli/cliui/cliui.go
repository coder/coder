package cliui

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/charm/ui/common"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	Canceled = errors.New("canceled")

	defaultStyles = common.DefaultStyles()
)

type Validate func(string) error

// ValidateNotEmpty is a helper function to disallow empty inputs!
func ValidateNotEmpty(s string) error {
	if s == "" {
		return errors.New("Must be provided!")
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

func print(cmd *cobra.Command, str string) {
	_, _ = fmt.Fprint(cmd.OutOrStdout(), str)
}
