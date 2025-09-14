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
		// Auto mode — env vars first (highest precedence wins)
		if os.Getenv("NO_COLOR") != "" {
			return false
		}
		if v := os.Getenv("FORCE_COLOR"); v != "" && v != "0" {
			return true
		}
		switch strings.ToLower(os.Getenv("CODER_COLOR")) {
		case "always":
			return true
		case "never":
			return false
		}
		// Fallback to TTY detection
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
