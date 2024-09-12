package cliui

import (
	"flag"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/muesli/termenv"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/pretty"
)

var Canceled = xerrors.New("canceled")

// DefaultStyles compose visual elements of the UI.
var DefaultStyles Styles

type Styles struct {
	Code,
	DateTimeStamp,
	Error,
	Field,
	Keyword,
	Placeholder,
	Prompt,
	FocusedPrompt,
	Fuchsia,
	Warn,
	Wrap pretty.Style
}

var (
	color     termenv.Profile
	colorOnce sync.Once
)

// Color returns a color for the given string.
func Color(s string) termenv.Color {
	colorOnce.Do(func() {
		color = termenv.NewOutput(os.Stdout).ColorProfile()
		if flag.Lookup("test.v") != nil {
			// Use a consistent colorless profile in tests so that results
			// are deterministic.
			color = termenv.Ascii
		}
	})
	return color.Color(s)
}

func isTerm() bool {
	return color != termenv.Ascii
}

// Bold returns a formatter that renders text in bold
// if the terminal supports it.
func Bold(s string) string {
	if !isTerm() {
		return s
	}
	return pretty.Sprint(pretty.Bold(), s)
}

// BoldFmt returns a formatter that renders text in bold
// if the terminal supports it.
func BoldFmt() pretty.Formatter {
	if !isTerm() {
		return pretty.Style{}
	}
	return pretty.Bold()
}

// Timestamp formats a timestamp for display.
func Timestamp(t time.Time) string {
	return pretty.Sprint(DefaultStyles.DateTimeStamp, t.Format(time.Stamp))
}

// Keyword formats a keyword for display.
func Keyword(s string) string {
	return pretty.Sprint(DefaultStyles.Keyword, s)
}

// Placeholder formats a placeholder for display.
func Placeholder(s string) string {
	return pretty.Sprint(DefaultStyles.Placeholder, s)
}

// Wrap prevents the text from overflowing the terminal.
func Wrap(s string) string {
	return pretty.Sprint(DefaultStyles.Wrap, s)
}

// Code formats code for display.
func Code(s string) string {
	return pretty.Sprint(DefaultStyles.Code, s)
}

// Field formats a field for display.
func Field(s string) string {
	return pretty.Sprint(DefaultStyles.Field, s)
}

func ifTerm(fmt pretty.Formatter) pretty.Formatter {
	if !isTerm() {
		return pretty.Nop
	}
	return fmt
}

func init() {
	// We do not adapt the color based on whether the terminal is light or dark.
	// Doing so would require a round-trip between the program and the terminal
	// due to the OSC query and response.
	DefaultStyles = Styles{Wrap: pretty.Style{pretty.LineWrap(80)}}

	defaultTheme := userThemeConfig{
		Colors: map[string]string{
			"green":  "#04B575",
			"red":    "#ED567A",
			"yellow": "#ECFD65",
			"blue":   "#5000ff",
		},
		Styles: &userThemeStyles{
			Code: &userThemeStyle{
				FgColor: "$red",
				BgColor: "#2C2C2C",
			},
			DateTimeStamp: &userThemeStyle{
				FgColor: "#7571F9",
			},
			Error: &userThemeStyle{
				FgColor: "$red",
			},
			Field: &userThemeStyle{
				FgColor: "#FFFFFF",
				BgColor: "#2B2A2A",
			},
			FocusedPrompt: &userThemeStyle{
				FgColor: "$blue",
			},
			Keyword: &userThemeStyle{
				FgColor: "$green",
			},
			Placeholder: &userThemeStyle{
				FgColor: "#4d46b3",
			},
			Prompt: &userThemeStyle{
				FgColor: "#5C5C5C",
			},
			Warn: &userThemeStyle{
				FgColor: "$yellow",
			},
		},
	}

	configDir := config.DefaultDir()
	if dir := os.Getenv("CODER_CONFIG_DIR"); dir != "" {
		configDir = dir
	}

	theme, _ := os.ReadFile(path.Join(configDir, "theme.toml"))
	_ = LoadUserTheme(&defaultTheme, theme)
}

type userThemeStyle struct {
	BgColor string `toml:"background"`
	FgColor string `toml:"foreground"`
}

type userThemeStyles struct {
	Code          *userThemeStyle `toml:"code"`
	DateTimeStamp *userThemeStyle `toml:"datetimestamp"`
	Error         *userThemeStyle `toml:"error"`
	Field         *userThemeStyle `toml:"field"`
	FocusedPrompt *userThemeStyle `toml:"focusedprompt"`
	Keyword       *userThemeStyle `toml:"keyword"`
	Placeholder   *userThemeStyle `toml:"placeholder"`
	Prompt        *userThemeStyle `toml:"prompt"`
	Warn          *userThemeStyle `toml:"warn"`
}

type userThemeConfig struct {
	Colors map[string]string `toml:"colors"`
	Styles *userThemeStyles  `toml:"styles"`
}

func LoadUserTheme(theme *userThemeConfig, t []byte) error {
	if err := toml.Unmarshal(t, theme); err != nil {
		return err
	}

	if theme.Styles.Code != nil {
		DefaultStyles.Code = pretty.Style{pretty.XPad(1, 1)}
		DefaultStyles.Code = append(DefaultStyles.Code, theme.Styles.Code.toPrettyStyle(theme)...)
	}
	if theme.Styles.DateTimeStamp != nil {
		DefaultStyles.DateTimeStamp = theme.Styles.DateTimeStamp.toPrettyStyle(theme)
	}
	if theme.Styles.Error != nil {
		DefaultStyles.Error = theme.Styles.Error.toPrettyStyle(theme)
	}
	if theme.Styles.Field != nil {
		DefaultStyles.Field = pretty.Style{pretty.XPad(1, 1)}
		DefaultStyles.Field = append(DefaultStyles.Field, theme.Styles.Field.toPrettyStyle(theme)...)
	}
	if theme.Styles.Keyword != nil {
		DefaultStyles.Keyword = theme.Styles.Keyword.toPrettyStyle(theme)
	}
	if theme.Styles.Warn != nil {
		DefaultStyles.Warn = theme.Styles.Warn.toPrettyStyle(theme)
	}
	if theme.Styles.Placeholder != nil {
		DefaultStyles.Placeholder = theme.Styles.Placeholder.toPrettyStyle(theme)
	}
	// NOTE: Prompt should be styled first as FocusedPrompt depends on the styling
	// of Prompt.
	if theme.Styles.Prompt != nil {
		DefaultStyles.Prompt = theme.Styles.Prompt.toPrettyStyle(theme)
		DefaultStyles.Prompt = append(DefaultStyles.Prompt, pretty.Wrap("> ", ""))
	}
	if theme.Styles.FocusedPrompt != nil {
		style := theme.Styles.FocusedPrompt.toPrettyStyle(theme)
		DefaultStyles.FocusedPrompt = append(DefaultStyles.Prompt, style...)
	}
	return nil
}

func (t userThemeConfig) getColor(color string) string {
	if strings.HasPrefix(color, "$") {
		if c, ok := t.Colors[color[1:]]; ok {
			return c
		}
	}
	return color
}

func (s *userThemeStyle) toPrettyStyle(theme *userThemeConfig) pretty.Style {
	style := pretty.Style{}
	if s.FgColor != "" {
		if color := Color(theme.getColor(s.FgColor)); color != nil {
			style = append(style, pretty.FgColor(color))
		}
	}
	if s.BgColor != "" {
		if color := Color(theme.getColor(s.BgColor)); color != nil {
			style = append(style, pretty.BgColor(color))
		}
	}
	return style
}

// ValidateNotEmpty is a helper function to disallow empty inputs!
func ValidateNotEmpty(s string) error {
	if s == "" {
		return xerrors.New("Must be provided!")
	}
	return nil
}
