package reconnectingpty

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithTerminalEnv(t *testing.T) {
	t.Parallel()

	defaultLocale := "C.UTF-8"
	if runtime.GOOS == "darwin" {
		defaultLocale = "UTF-8"
	}

	tests := []struct {
		name           string
		env            []string
		wantLCCTYPE    string
		wantLCCTYPESet bool
	}{
		{
			name:           "adds locale when missing",
			env:            []string{"PATH=/bin"},
			wantLCCTYPE:    defaultLocale,
			wantLCCTYPESet: true,
		},
		{
			name:           "adds locale when lang is not utf8",
			env:            []string{"LANG=C"},
			wantLCCTYPE:    defaultLocale,
			wantLCCTYPESet: true,
		},
		{
			name: "keeps utf8 lang",
			env:  []string{"LANG=C.UTF-8"},
		},
		{
			name: "keeps unhyphenated utf8 lang",
			env:  []string{"LANG=C.UTF8"},
		},
		{
			name:           "keeps utf8 ctype",
			env:            []string{"LC_CTYPE=C.UTF-8"},
			wantLCCTYPE:    "C.UTF-8",
			wantLCCTYPESet: true,
		},
		{
			name:           "overrides non utf8 ctype",
			env:            []string{"LANG=C.UTF-8", "LC_CTYPE=C"},
			wantLCCTYPE:    defaultLocale,
			wantLCCTYPESet: true,
		},
		{
			name: "keeps utf8 lc all",
			env:  []string{"LC_ALL=C.UTF-8"},
		},
		{
			name: "preserves non empty lc all",
			env:  []string{"LC_ALL=C"},
		},
		{
			name:           "ignores empty lc all",
			env:            []string{"LC_ALL="},
			wantLCCTYPE:    defaultLocale,
			wantLCCTYPESet: true,
		},
		{
			name: "continues after empty lc all",
			env:  []string{"LC_ALL=", "LANG=C.UTF-8"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := withTerminalEnv(tt.env)
			term, ok := envValue(got, "TERM")
			require.True(t, ok)
			require.Equal(t, xterm256Color, term)

			wantLCCTYPE := tt.wantLCCTYPE
			wantLCCTYPESet := tt.wantLCCTYPESet
			if runtime.GOOS == "windows" {
				wantLCCTYPE, wantLCCTYPESet = envValue(tt.env, "LC_CTYPE")
			}

			locale, ok := envValue(got, "LC_CTYPE")
			require.Equal(t, wantLCCTYPESet, ok)
			if wantLCCTYPESet {
				require.Equal(t, wantLCCTYPE, locale)
			}
		})
	}
}
