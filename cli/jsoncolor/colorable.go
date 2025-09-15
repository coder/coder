package jsoncolor

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"golang.org/x/xerrors"
)

// ColorMode represents color output configuration
type ColorMode int

const (
	// ColorModeAuto automatically determines if color should be used
	ColorModeAuto ColorMode = iota
	// ColorModeAlways forces color output on
	ColorModeAlways
	// ColorModeNever disables color output
	ColorModeNever
)

// StringToColorMode converts a string to a ColorMode
func StringToColorMode(s string) ColorMode {
	switch strings.ToLower(s) {
	case "always":
		return ColorModeAlways
	case "never":
		return ColorModeNever
	default:
		return ColorModeAuto
	}
}

// ShouldUseColor determines if colors should be used based on
// the ColorMode, terminal detection, and environment variables
func ShouldUseColor(mode ColorMode, w io.Writer) bool {
	switch mode {
	case ColorModeAlways:
		return true
	case ColorModeNever:
		return false
	default:
		// AUTO: envs override, regardless of TTY or platform.
		if os.Getenv("NO_COLOR") != "" {
			return false
		}
		switch strings.ToLower(os.Getenv("CODER_COLOR")) {
		case "never":
			return false
		case "always":
			return true
		}
		if v, ok := os.LookupEnv("FORCE_COLOR"); ok {
			if v == "" || v == "0" || strings.EqualFold(v, "false") {
				return false
			}
			return true
		}

		// Fallback: TTY check only if writer is an *os.File
		if f, ok := w.(*os.File); ok && (isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())) {
			return true
		}
		return false
	}
}

// WriteColorized writes colorized JSON to the writer using the given color mode
func WriteColorized(w io.Writer, data []byte, indent string, mode ColorMode) error {
	useColor := ShouldUseColor(mode, w)

	if !useColor {
		// No color - just pretty print JSON
		var raw interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			// If parsing fails, output as-is
			if _, err := w.Write(data); err != nil {
				return xerrors.Errorf("failed to write data: %w", err)
			}
			return nil
		}

		pretty, err := json.MarshalIndent(raw, "", indent)
		if err != nil {
			// If pretty printing fails, output as-is
			if _, err := w.Write(data); err != nil {
				return xerrors.Errorf("failed to write data: %w", err)
			}
			return nil
		}

		if _, err := w.Write(pretty); err != nil {
			return xerrors.Errorf("failed to write pretty JSON: %w", err)
		}
		return nil
	}

	// Use colorized output
	return Write(w, bytes.NewReader(data), indent)
}
