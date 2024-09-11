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

var (
	Green   = Color("#04B575")
	Red     = Color("#ED567A")
	Fuchsia = Color("#EE6FF8")
	Yellow  = Color("#ECFD65")
	Blue    = Color("#5000ff")
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
	DefaultStyles = Styles{
		Code: pretty.Style{
			ifTerm(pretty.XPad(1, 1)),
			pretty.FgColor(Red),
			pretty.BgColor(color.Color("#2c2c2c")),
		},
		DateTimeStamp: pretty.Style{
			pretty.FgColor(color.Color("#7571F9")),
		},
		Error: pretty.Style{
			pretty.FgColor(Red),
		},
		Field: pretty.Style{
			pretty.XPad(1, 1),
			pretty.FgColor(color.Color("#FFFFFF")),
			pretty.BgColor(color.Color("#2b2a2a")),
		},
		Keyword: pretty.Style{
			pretty.FgColor(Green),
		},
		Placeholder: pretty.Style{
			pretty.FgColor(color.Color("#4d46b3")),
		},
		Prompt: pretty.Style{
			pretty.FgColor(color.Color("#5C5C5C")),
			pretty.Wrap("> ", ""),
		},
		Warn: pretty.Style{
			pretty.FgColor(Yellow),
		},
		Wrap: pretty.Style{
			pretty.LineWrap(80),
		},
	}

	DefaultStyles.FocusedPrompt = append(
		DefaultStyles.Prompt,
		pretty.FgColor(Blue),
	)

	configDir := config.DefaultDir()
	if dir := os.Getenv("CODER_CONFIG_DIR"); dir != "" {
		configDir = dir
	}

	if theme, err := os.ReadFile(path.Join(configDir, "theme.toml")); err == nil {
		_ = LoadUserTheme(theme)
	}
}

type userThemeStyle struct {
	BgColor string `toml:"background"`
	FgColor string `toml:"foreground"`
	XPad    *struct {
		Left  int `toml:"left"`
		Right int `toml:"right"`
	} `toml:"xpad"`
	Wrap *struct {
		Prefix string `toml:"prefix"`
		Suffix string `toml:"suffix"`
	}
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
	Styles userThemeStyles   `toml:"styles"`
}

func LoadUserTheme(t []byte) error {
	var theme userThemeConfig
	if err := toml.Unmarshal(t, &theme); err != nil {
		return err
	}

	if theme.Styles.Code != nil {
		DefaultStyles.Code = theme.Styles.Code.toPrettyStyle(theme)
	}
	if theme.Styles.DateTimeStamp != nil {
		DefaultStyles.DateTimeStamp = theme.Styles.DateTimeStamp.toPrettyStyle(theme)
	}
	if theme.Styles.Error != nil {
		DefaultStyles.Error = theme.Styles.Error.toPrettyStyle(theme)
	}
	if theme.Styles.Field != nil {
		DefaultStyles.Field = theme.Styles.Field.toPrettyStyle(theme)
	}
	if theme.Styles.FocusedPrompt != nil {
		DefaultStyles.FocusedPrompt = theme.Styles.FocusedPrompt.toPrettyStyle(theme)
	}
	if theme.Styles.Keyword != nil {
		DefaultStyles.Keyword = theme.Styles.Keyword.toPrettyStyle(theme)
	}
	if theme.Styles.Prompt != nil {
		DefaultStyles.Prompt = theme.Styles.Prompt.toPrettyStyle(theme)
	}
	if theme.Styles.Warn != nil {
		DefaultStyles.Warn = theme.Styles.Warn.toPrettyStyle(theme)
	}
	if theme.Styles.Placeholder != nil {
		DefaultStyles.Placeholder = theme.Styles.Placeholder.toPrettyStyle(theme)
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

func (s *userThemeStyle) toPrettyStyle(theme userThemeConfig) pretty.Style {
	style := pretty.Style{}
	if s.XPad != nil {
		style = append(style, pretty.XPad(s.XPad.Left, s.XPad.Right))
	}
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
	if s.Wrap != nil {
		style = append(style, pretty.Wrap(s.Wrap.Prefix, s.Wrap.Suffix))
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
