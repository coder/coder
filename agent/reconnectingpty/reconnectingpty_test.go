//nolint:testpackage // Tests private env helpers directly.
package reconnectingpty

import (
	"runtime"
	"testing"
)

func TestWithTerminalEnv(t *testing.T) {
	t.Parallel()

	defaultLocale := "C.UTF-8"
	if runtime.GOOS == "darwin" {
		defaultLocale = "UTF-8"
	}

	tests := []struct {
		name       string
		env        []string
		wantLocale string
	}{
		{
			name:       "adds locale when missing",
			env:        []string{"PATH=/bin"},
			wantLocale: defaultLocale,
		},
		{
			name:       "adds locale when lang is not utf8",
			env:        []string{"LANG=C"},
			wantLocale: defaultLocale,
		},
		{
			name:       "keeps utf8 lang",
			env:        []string{"LANG=C.UTF-8"},
			wantLocale: "",
		},
		{
			name:       "overrides non utf8 ctype",
			env:        []string{"LANG=C.UTF-8", "LC_CTYPE=C"},
			wantLocale: defaultLocale,
		},
		{
			name:       "keeps utf8 lc all",
			env:        []string{"LC_ALL=C.UTF-8"},
			wantLocale: "",
		},
		{
			name:       "preserves non empty lc all",
			env:        []string{"LC_ALL=C"},
			wantLocale: "",
		},
		{
			name:       "ignores empty lc all",
			env:        []string{"LC_ALL="},
			wantLocale: defaultLocale,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := withTerminalEnv(tt.env)
			if term, ok := envValue(got, "TERM"); !ok || term != xterm256Color {
				t.Fatalf("TERM = %q, %v, want %q, true", term, ok, xterm256Color)
			}

			locale, ok := envValue(got, "LC_CTYPE")
			if runtime.GOOS == "windows" {
				if ok && locale == defaultLocale {
					t.Fatalf("LC_CTYPE = %q, want no default", locale)
				}
				return
			}
			if tt.wantLocale == "" {
				if ok && locale == defaultLocale {
					t.Fatalf("LC_CTYPE = %q, want no default", locale)
				}
				return
			}
			if !ok || locale != tt.wantLocale {
				t.Fatalf("LC_CTYPE = %q, %v, want %q, true", locale, ok, tt.wantLocale)
			}
		})
	}
}
