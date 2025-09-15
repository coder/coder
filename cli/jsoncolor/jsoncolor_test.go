package jsoncolor_test

import (
	"bytes"
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
	// NOTE: This test mutates process-wide env vars; keep it serial to avoid flakiness.
	// Other tests in this file remain parallel-safe.
	tests := []struct {
		name       string
		mode       jsoncolor.ColorMode
		envSetup   func(t *testing.T)
		writer     func() *bytes.Buffer
		wantResult bool
	}{
		{
			name:       "always mode",
			mode:       jsoncolor.ColorModeAlways,
			envSetup:   func(t *testing.T) { baselineEnv(t) },
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: true,
		},
		{
			name:       "never mode",
			mode:       jsoncolor.ColorModeNever,
			envSetup:   func(t *testing.T) { baselineEnv(t) },
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: false,
		},
		{
			name: "auto mode with FORCE_COLOR",
			mode: jsoncolor.ColorModeAuto,
			envSetup: func(t *testing.T) {
				baselineEnv(t)
				t.Setenv("FORCE_COLOR", "1")
			},
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: true,
		},
		{
			name: "auto mode with NO_COLOR",
			mode: jsoncolor.ColorModeAuto,
			envSetup: func(t *testing.T) {
				baselineEnv(t)
				t.Setenv("NO_COLOR", "1")
			},
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: false,
		},
		{
			name: "auto mode with CODER_COLOR=always",
			mode: jsoncolor.ColorModeAuto,
			envSetup: func(t *testing.T) {
				baselineEnv(t)
				t.Setenv("CODER_COLOR", "always")
			},
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: true,
		},
		{
			name: "auto mode with CODER_COLOR=never",
			mode: jsoncolor.ColorModeAuto,
			envSetup: func(t *testing.T) {
				baselineEnv(t)
				t.Setenv("CODER_COLOR", "never")
			},
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: false,
		},
		{
			name: "auto mode with no env vars and non-terminal",
			mode: jsoncolor.ColorModeAuto,
			envSetup: func(t *testing.T) {
				// Baseline clears overrides; writer is a bytes.Buffer (non-tty)
				baselineEnv(t)
			},
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: false,
		},
		{
			name: "FORCE_COLOR=0 disables color",
			mode: jsoncolor.ColorModeAuto,
			envSetup: func(t *testing.T) {
				baselineEnv(t)
				t.Setenv("FORCE_COLOR", "0")
			},
			writer:     func() *bytes.Buffer { return &bytes.Buffer{} },
			wantResult: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Do NOT call t.Parallel() here; this test intentionally runs serially.
			tt.envSetup(t)
			w := tt.writer()
			result := jsoncolor.ShouldUseColor(tt.mode, w)
			require.Equal(t, tt.wantResult, result)
		})
	}
}

// baselineEnv clears the three override env vars for a clean start in each subtest.
// We use empty strings to represent "unset" consistently across platforms.
func baselineEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("CODER_COLOR", "")
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
