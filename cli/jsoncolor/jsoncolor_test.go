package jsoncolor_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/jsoncolor"
)

func TestWrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantErr     bool
		contains    []string
		notContains []string
	}{
		{
			name:    "simple object",
			input:   `{"name":"Test User","active":true,"count":42,"metadata":null}`,
			wantErr: false,
			contains: []string{
				"\033[1;34m\"name\"\033[0m",    // blue key
				"\033[32m\"Test User\"\033[0m", // green string
				"\033[33mtrue\033[0m",          // yellow boolean
				"\033[35m42\033[0m",            // magenta number
				"\033[36mnull\033[0m",          // cyan null
			},
		},
		{
			name:    "nested object",
			input:   `{"user":{"name":"Test","id":123},"active":true}`,
			wantErr: false,
			contains: []string{
				"\033[1;34m\"user\"\033[0m", // blue key
				"\033[1;37m{\033[0m",        // white brace
				"\033[1;34m\"name\"\033[0m", // blue nested key
				"\033[32m\"Test\"\033[0m",   // green string
			},
		},
		{
			name:    "array",
			input:   `["one","two",3]`,
			wantErr: false,
			contains: []string{
				"\033[1;37m[\033[0m",     // white bracket
				"\033[32m\"one\"\033[0m", // green string
				"\033[35m3\033[0m",       // magenta number
			},
		},
		{
			name:    "empty object",
			input:   `{}`,
			wantErr: false,
			contains: []string{
				"\033[1;37m{}\033[0m", // white braces
			},
		},
		{
			name:    "empty array",
			input:   `[]`,
			wantErr: false,
			contains: []string{
				"\033[1;37m[]\033[0m", // white brackets
			},
		},
		{
			name:    "invalid json",
			input:   `{"name":"Test"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := jsoncolor.Write(&buf, strings.NewReader(tt.input), "  ")

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			output := buf.String()

			for _, substr := range tt.contains {
				require.Contains(t, output, substr)
			}

			for _, substr := range tt.notContains {
				require.NotContains(t, output, substr)
			}
		})
	}
}

func TestStringToColorMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected jsoncolor.ColorMode
	}{
		{"auto", jsoncolor.ColorModeAuto},
		{"AUTO", jsoncolor.ColorModeAuto}, // Case insensitive
		{"always", jsoncolor.ColorModeAlways},
		{"ALWAYS", jsoncolor.ColorModeAlways}, // Case insensitive
		{"never", jsoncolor.ColorModeNever},
		{"NEVER", jsoncolor.ColorModeNever},  // Case insensitive
		{"", jsoncolor.ColorModeAuto},        // Default
		{"unknown", jsoncolor.ColorModeAuto}, // Default for unknown
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			result := jsoncolor.StringToColorMode(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldUseColor(t *testing.T) {
	t.Parallel()

	// Save original env and restore after test
	origNoColor := os.Getenv("NO_COLOR")
	origForceColor := os.Getenv("FORCE_COLOR")
	origCoderColor := os.Getenv("CODER_COLOR")

	// Use t.Cleanup instead of defer for proper cleanup with t.Parallel
	t.Cleanup(func() {
		if err := os.Setenv("NO_COLOR", origNoColor); err != nil {
			t.Errorf("Failed to restore environment variable: %v", err)
		}
		if err := os.Setenv("FORCE_COLOR", origForceColor); err != nil {
			t.Errorf("Failed to restore environment variable: %v", err)
		}
		if err := os.Setenv("CODER_COLOR", origCoderColor); err != nil {
			t.Errorf("Failed to restore environment variable: %v", err)
		}
	})

	tests := []struct {
		name       string
		mode       jsoncolor.ColorMode
		envSetup   func()
		writer     func() *bytes.Buffer
		wantResult bool
	}{
		{
			name:       "always mode",
			mode:       jsoncolor.ColorModeAlways,
			envSetup:   func() {},
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: true,
		},
		{
			name:       "never mode",
			mode:       jsoncolor.ColorModeNever,
			envSetup:   func() {},
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: false,
		},
		{
			name: "auto mode with FORCE_COLOR",
			mode: jsoncolor.ColorModeAuto,
			envSetup: func() {
				if err := os.Setenv("FORCE_COLOR", "1"); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
				if err := os.Setenv("NO_COLOR", ""); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
			},
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: true,
		},
		{
			name: "auto mode with NO_COLOR",
			mode: jsoncolor.ColorModeAuto,
			envSetup: func() {
				if err := os.Setenv("NO_COLOR", "1"); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
				if err := os.Setenv("FORCE_COLOR", ""); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
			},
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: false,
		},
		{
			name: "auto mode with CODER_COLOR=always",
			mode: jsoncolor.ColorModeAuto,
			envSetup: func() {
				if err := os.Setenv("CODER_COLOR", "always"); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
				if err := os.Setenv("NO_COLOR", ""); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
				if err := os.Setenv("FORCE_COLOR", ""); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
			},
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: true,
		},
		{
			name: "auto mode with CODER_COLOR=never",
			mode: jsoncolor.ColorModeAuto,
			envSetup: func() {
				if err := os.Setenv("CODER_COLOR", "never"); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
				if err := os.Setenv("NO_COLOR", ""); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
				if err := os.Setenv("FORCE_COLOR", ""); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
			},
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: false,
		},
		{
			name: "auto mode with no env vars and non-terminal",
			mode: jsoncolor.ColorModeAuto,
			envSetup: func() {
				if err := os.Setenv("NO_COLOR", ""); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
				if err := os.Setenv("FORCE_COLOR", ""); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
				if err := os.Setenv("CODER_COLOR", ""); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
			},
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: false,
		},
		{
			name: "FORCE_COLOR=0 disables color",
			mode: jsoncolor.ColorModeAuto,
			envSetup: func() {
				if err := os.Setenv("FORCE_COLOR", "0"); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
				if err := os.Setenv("NO_COLOR", ""); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
				if err := os.Setenv("CODER_COLOR", ""); err != nil {
					t.Fatalf("Failed to set environment variable: %v", err)
				}
			},
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Set up environment
			tt.envSetup()

			// Create writer
			w := tt.writer()

			// Test
			result := jsoncolor.ShouldUseColor(tt.mode, w)
			require.Equal(t, tt.wantResult, result)
		})
	}
}

func TestWriteColorized(t *testing.T) {
	t.Parallel()

	jsonData := []byte(`{"name":"Test","active":true,"count":42}`)

	tests := []struct {
		name        string
		mode        jsoncolor.ColorMode
		expectColor bool
	}{
		{
			name:        "always mode",
			mode:        jsoncolor.ColorModeAlways,
			expectColor: true,
		},
		{
			name:        "never mode",
			mode:        jsoncolor.ColorModeNever,
			expectColor: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := jsoncolor.WriteColorized(&buf, jsonData, "  ", tt.mode)

			require.NoError(t, err)
			output := buf.String()

			if tt.expectColor {
				// Should contain ANSI color codes
				require.Contains(t, output, "\033[")
			} else {
				// Should not contain ANSI color codes
				require.NotContains(t, output, "\033[")
			}

			// Should contain the data regardless
			require.Contains(t, output, "Test")
			require.Contains(t, output, "true")
			require.Contains(t, output, "42")
		})
	}
}
