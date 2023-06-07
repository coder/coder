package cliui

import (
	"os"

	"github.com/charmbracelet/charm/ui/common"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/xerrors"
)

var Canceled = xerrors.New("canceled")

// DefaultStyles compose visual elements of the UI.
var DefaultStyles Styles

type Styles struct {
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
}

func init() {
	lipgloss.SetDefaultRenderer(
		lipgloss.NewRenderer(os.Stdout, termenv.WithColorCache(true)),
	)

	// All Styles are set after we change the DefaultRender so that the ColorCache
	// is in effect, mitigating the severe issues seen here:
	// https://github.com/coder/coder/issues/7884.

	charmStyles := common.DefaultStyles()

	DefaultStyles = Styles{
		Bold:          lipgloss.NewStyle().Bold(true),
		Checkmark:     charmStyles.Checkmark,
		Code:          charmStyles.Code,
		Crossmark:     charmStyles.Error.Copy().SetString("✘"),
		DateTimeStamp: charmStyles.LabelDim,
		Error:         charmStyles.Error,
		Field:         charmStyles.Code.Copy().Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}),
		Keyword:       charmStyles.Keyword,
		Paragraph:     charmStyles.Paragraph,
		Placeholder:   lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#585858", Dark: "#4d46b3"}),
		Prompt:        charmStyles.Prompt.Copy().Foreground(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#5C5C5C"}),
		FocusedPrompt: charmStyles.FocusedPrompt.Copy().Foreground(lipgloss.Color("#651fff")),
		Fuchsia:       charmStyles.SelectedMenuItem.Copy(),
		Logo:          charmStyles.Logo.Copy().SetString("Coder"),
		Warn: lipgloss.NewStyle().Foreground(
			lipgloss.AdaptiveColor{Light: "#04B575", Dark: "#ECFD65"},
		),
		Wrap: lipgloss.NewStyle().Width(80),
	}
}

// ValidateNotEmpty is a helper function to disallow empty inputs!
func ValidateNotEmpty(s string) error {
	if s == "" {
		return xerrors.New("Must be provided!")
	}
	return nil
}
