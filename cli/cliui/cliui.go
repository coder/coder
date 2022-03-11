package cliui

import (
	"errors"

	"github.com/charmbracelet/charm/ui/common"
	"github.com/charmbracelet/lipgloss"
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
	Field,
	Prompt,
	FocusedPrompt,
	Logo lipgloss.Style
}{
	Bold:          lipgloss.NewStyle().Bold(true),
	Field:         defaultStyles.Code.Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}),
	Prompt:        defaultStyles.Prompt.Foreground(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#5C5C5C"}),
	FocusedPrompt: defaultStyles.FocusedPrompt.Foreground(lipgloss.Color("#651fff")),
	Logo:          defaultStyles.Logo.SetString("Coder"),
}
