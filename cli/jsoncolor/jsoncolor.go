// Package jsoncolor provides functions for colorizing JSON output.
package jsoncolor

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Colors for different token types
const (
	colorNull   = "36"   // cyan
	colorBool   = "33"   // yellow
	colorNumber = "35"   // magenta
	colorString = "32"   // green
	colorKey    = "1;34" // bright blue
	colorDelim  = "1;37" // bright white
)

// Write colorizes and formats JSON data from a reader
func Write(w io.Writer, r io.Reader, indent string) error {
	// Parse the JSON
	var raw interface{}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}

	// Colorize and write the JSON
	return writeJSON(w, raw, 0, indent)
}

// writeJSON recursively writes colorized JSON
func writeJSON(w io.Writer, data interface{}, depth int, indent string) error {
	switch value := data.(type) {
	case nil:
		_, err := fmt.Fprintf(w, "\033[%smnull\033[0m", colorNull)
		return err

	case bool:
		_, err := fmt.Fprintf(w, "\033[%sm%t\033[0m", colorBool, value)
		return err

	case float64:
		_, err := fmt.Fprintf(w, "\033[%sm%g\033[0m", colorNumber, value)
		return err

	case json.Number:
		_, err := fmt.Fprintf(w, "\033[%sm%s\033[0m", colorNumber, value)
		return err

	case int:
		_, err := fmt.Fprintf(w, "\033[%sm%d\033[0m", colorNumber, value)
		return err

	case string:
		_, err := fmt.Fprintf(w, "\033[%sm%s\033[0m", colorString, quoteString(value))
		return err

	case map[string]interface{}:
		if err := writeMap(w, value, depth, indent); err != nil {
			return err
		}

	case []interface{}:
		if err := writeArray(w, value, depth, indent); err != nil {
			return err
		}

	default:
		// Fallback to default JSON for unknown types
		b, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}

	return nil
}

// writeMap writes a colorized JSON object
func writeMap(w io.Writer, m map[string]interface{}, depth int, indent string) error {
	if len(m) == 0 {
		_, err := fmt.Fprintf(w, "\033[%sm{}\033[0m", colorDelim)
		return err
	}

	// Write opening brace
	if _, err := fmt.Fprintf(w, "\033[%sm{\033[0m\n", colorDelim); err != nil {
		return err
	}

	// Write key-value pairs
	i := 0
	for k, v := range m {
		padding := strings.Repeat(indent, depth+1)
		if _, err := fmt.Fprint(w, padding); err != nil {
			return err
		}

		// Write key
		if _, err := fmt.Fprintf(w, "\033[%sm%s\033[0m\033[%sm:\033[0m ", colorKey, quoteString(k), colorDelim); err != nil {
			return err
		}

		// Write value
		if err := writeJSON(w, v, depth+1, indent); err != nil {
			return err
		}

		// Write comma if not the last item
		if i < len(m)-1 {
			if _, err := fmt.Fprintf(w, "\033[%sm,\033[0m\n", colorDelim); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprint(w, "\n"); err != nil {
				return err
			}
		}
		i++
	}

	// Write closing brace
	padding := strings.Repeat(indent, depth)
	if _, err := fmt.Fprintf(w, "%s\033[%sm}\033[0m", padding, colorDelim); err != nil {
		return err
	}

	return nil
}

// writeArray writes a colorized JSON array
func writeArray(w io.Writer, a []interface{}, depth int, indent string) error {
	if len(a) == 0 {
		_, err := fmt.Fprintf(w, "\033[%sm[]\033[0m", colorDelim)
		return err
	}

	// Write opening bracket
	if _, err := fmt.Fprintf(w, "\033[%sm[\033[0m\n", colorDelim); err != nil {
		return err
	}

	// Write values
	for i, v := range a {
		padding := strings.Repeat(indent, depth+1)
		if _, err := fmt.Fprint(w, padding); err != nil {
			return err
		}

		// Write value
		if err := writeJSON(w, v, depth+1, indent); err != nil {
			return err
		}

		// Write comma if not the last item
		if i < len(a)-1 {
			if _, err := fmt.Fprintf(w, "\033[%sm,\033[0m\n", colorDelim); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprint(w, "\n"); err != nil {
				return err
			}
		}
	}

	// Write closing bracket
	padding := strings.Repeat(indent, depth)
	if _, err := fmt.Fprintf(w, "%s\033[%sm]\033[0m", padding, colorDelim); err != nil {
		return err
	}

	return nil
}

// quoteString ensures a string is properly JSON quoted
func quoteString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
