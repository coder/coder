package cliui

import (
	"github.com/charmbracelet/charm/ui/common"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/xerrors"
)

var (
	Canceled = xerrors.New("canceled")

	defaultStyles = common.DefaultStyles()
)

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
	Checkmark,
	Code,
	Crossmark,
	DateTimeStamp,
	Error,
	Field,
	Keyword,
	Paragraph,
	Placeholder,
	Prompt,
	FocusedPrompt,
	Fuchsia,
	Logo,
	Warn,
	Wrap lipgloss.Style
}{
	Bold:          lipgloss.NewStyle().Bold(true),
	Checkmark:     defaultStyles.Checkmark,
	Code:          defaultStyles.Code,
	Crossmark:     defaultStyles.Error.Copy().SetString("✘"),
	DateTimeStamp: defaultStyles.LabelDim,
	Error:         defaultStyles.Error,
	Field:         defaultStyles.Code.Copy().Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}),
	Keyword:       defaultStyles.Keyword,
	Paragraph:     defaultStyles.Paragraph,
	Placeholder:   lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#585858", Dark: "#4d46b3"}),
	Prompt:        defaultStyles.Prompt.Foreground(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#5C5C5C"}),
	FocusedPrompt: defaultStyles.FocusedPrompt.Foreground(lipgloss.Color("#651fff")),
	Fuchsia:       defaultStyles.SelectedMenuItem.Copy(),
	Logo:          defaultStyles.Logo.SetString("Coder"),
	Warn:          lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#04B575", Dark: "#ECFD65"}),
	Wrap:          lipgloss.NewStyle().Width(80),
}
