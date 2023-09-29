package patternmatcher_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw/patternmatcher"
)

func Test_RoutePatterns(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		patterns    []string
		errContains string
		output      string
	}{
		{
			name:     "Empty",
			patterns: []string{},
			output:   "^()$",
		},
		{
			name: "Single",
			patterns: []string{
				"/api",
			},
			output: "^(/api/?)$",
		},
		{
			name: "TrailingSlash",
			patterns: []string{
				"/api/",
			},
			output: "^(/api/)$",
		},
		{
			name: "Multiple",
			patterns: []string{
				"/api",
				"/api2",
			},
			output: "^(/api/?|/api2/?)$",
		},
		{
			name: "Star",
			patterns: []string{
				"/api/*",
			},
			output: "^(/api/[^/]+/?)$",
		},
		{
			name: "StarStar",
			patterns: []string{
				"/api/**",
			},
			output: "^(/api/.+/?)$",
		},
		{
			name: "TelemetryPatterns",
			patterns: []string{
				"/api",
				"/api/**",
				"/@*/*/apps/**",
				"/%40*/*/apps/**",
				"/externalauth/*/callback",
			},
			output: "^(/api/?|/api/.+/?|/@[^/]+/[^/]+/apps/.+/?|/%40[^/]+/[^/]+/apps/.+/?|/externalauth/[^/]+/callback/?)$",
		},
		{
			name: "Slash",
			patterns: []string{
				"/",
			},
			output: "^(/)$",
		},
		{
			name: "SlashStar",
			patterns: []string{
				"/*",
			},
			output: "^(/[^/]+/?)$",
		},
		{
			name: "SlashStarStar",
			patterns: []string{
				"/**",
			},
			output: "^(/.+/?)$",
		},
		{
			name: "SlashSlash",
			patterns: []string{
				"//",
				"/api//v1",
			},
			output: "^(//|/api//v1/?)$",
		},
		{
			name: "Invalid",
			patterns: []string{
				"/api(",
			},
			errContains: "compile regex",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			rp := patternmatcher.RoutePatterns(c.patterns)
			re, err := rp.Compile()
			if c.errContains != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, c.errContains)

				require.Panics(t, func() {
					_ = rp.MustCompile()
				})
			} else {
				require.NoError(t, err)
				require.Equal(t, c.output, re.String())

				require.NotPanics(t, func() {
					re := rp.MustCompile()
					require.Equal(t, c.output, re.String())
				})
			}
		})
	}
}
