//nolint:paralleltest,testpackage
package rulesengine

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoundTrip(t *testing.T) {
	tcs := []struct {
		name        string
		rules       []string
		url         string
		method      string
		expectParse bool
		expectMatch bool
	}{
		{
			name:        "basic all three",
			rules:       []string{"method=GET,HEAD domain=github.com path=/wibble/wobble"},
			url:         "https://github.com/wibble/wobble",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "method rejects properly",
			rules:       []string{"method=GET"},
			url:         "https://github.com/wibble/wobble",
			method:      "POST",
			expectParse: true,
			expectMatch: false,
		},
		{
			name:        "domain rejects properly",
			rules:       []string{"domain=github.com"},
			url:         "https://example.com/wibble/wobble",
			method:      "GET",
			expectParse: true,
			expectMatch: false,
		},
		{
			name:        "path rejects properly",
			rules:       []string{"path=/wibble/wobble"},
			url:         "https://github.com/different/path",
			method:      "GET",
			expectParse: true,
			expectMatch: false,
		},
		{
			name:        "multiple rules - one matches",
			rules:       []string{"domain=github.com", "domain=example.com"},
			url:         "https://github.com/wibble/wobble",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "method wildcard matches anything",
			rules:       []string{"method=*"},
			url:         "https://github.com/wibble/wobble",
			method:      "POST",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "domain wildcard matches anything",
			rules:       []string{"domain=*"},
			url:         "https://example.com/wibble/wobble",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "path wildcard matches anything",
			rules:       []string{"path=*"},
			url:         "https://github.com/any/path/here",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "all three wildcards match anything",
			rules:       []string{"method=* domain=* path=*"},
			url:         "https://example.com/some/random/path",
			method:      "DELETE",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "query parameters don't break matching",
			rules:       []string{"domain=github.com path=/wibble/wobble"},
			url:         "https://github.com/wibble/wobble?param1=value1&param2=value2",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "domain wildcard segment matches",
			rules:       []string{"domain=*.github.com"},
			url:         "https://api.github.com/repos",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "domain cannot end with asterisk",
			rules:       []string{"domain=github.*"},
			url:         "https://github.com/repos",
			method:      "GET",
			expectParse: false,
			expectMatch: false,
		},
		{
			name:        "domain asterisk in middle matches",
			rules:       []string{"domain=github.*.com"},
			url:         "https://github.api.com/repos",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "domain wildcard matches multiple subdomains",
			rules:       []string{"domain=*.github.com"},
			url:         "https://v1.api.github.com/repos",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "path asterisk in middle matches",
			rules:       []string{"path=/api/*/users"},
			url:         "https://github.com/api/v1/users",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "path asterisk at start matches",
			rules:       []string{"path=/*/users"},
			url:         "https://github.com/api/users",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "path asterisk doesn't match multiple segments",
			rules:       []string{"path=/api/*/users"},
			url:         "https://github.com/api/../admin/users",
			method:      "GET",
			expectParse: true,
			expectMatch: false,
		},
		{
			name:        "path asterisk at end matches",
			rules:       []string{"path=/api/v1/*"},
			url:         "https://github.com/api/v1/users",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "path asterisk at end matches multiple segments",
			rules:       []string{"path=/api/*"},
			url:         "https://github.com/api/v1/users/123/details",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "subpaths do not match automatically",
			rules:       []string{"path=/api"},
			url:         "https://github.com/api/users",
			method:      "GET",
			expectParse: true,
			expectMatch: false,
		},
		{
			name:        "multiple rules match specific path and subpaths",
			rules:       []string{"path=/wibble/wobble,/wibble/wobble/*"},
			url:         "https://github.com/wibble/wobble/sub",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "domain matches without scheme - example.com case",
			rules:       []string{"domain=example.com"},
			url:         "example.com",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "domain matches without scheme - jsonplaceholder case",
			rules:       []string{"domain=jsonplaceholder.typicode.com"},
			url:         "jsonplaceholder.typicode.com",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "domain matches without scheme - dev.coder.com case",
			rules:       []string{"domain=dev.coder.com"},
			url:         "dev.coder.com",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
	}

	logHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(logHandler)

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			rules, err := ParseAllowSpecs(tc.rules)
			if tc.expectParse {
				require.Nil(t, err)
				engine := NewRuleEngine(rules, logger)
				result := engine.Evaluate(tc.method, tc.url)
				require.Equal(t, tc.expectMatch, result.Allowed)
			} else {
				require.NotNil(t, err)
			}
		})
	}
}

func TestRoundTripExtraRules(t *testing.T) {
	tcs := []struct {
		name        string
		rules       []string
		url         string
		method      string
		expectParse bool
		expectMatch bool
	}{
		{
			name:        "domain=* allows everything",
			rules:       []string{"domain=*"},
			url:         "https://github.com/wibble/wobble",
			method:      "DELETE",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "specifying port in Domain key is NOT allowed",
			rules:       []string{"domain=github.com:8080"},
			url:         "https://github.com/wibble/wobble",
			method:      "DELETE",
			expectParse: false,
			expectMatch: false,
		},
		{
			name:        "specifying port in URL is allowed",
			rules:       []string{"domain=github.com"},
			url:         "https://github.com:8080/wibble/wobble",
			method:      "DELETE",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "wildcard symbol at the end of path",
			rules:       []string{"method=GET,POST,PUT domain=github.com path=/api/issues/*"},
			url:         "https://github.com/api/issues/123/edit",
			method:      "POST",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "wildcard symbol at the end of path doesn't match base path",
			rules:       []string{"method=GET domain=github.com path=/api/issues/*"},
			url:         "https://github.com/api/issues",
			method:      "GET",
			expectParse: true,
			expectMatch: false,
		},
		{
			name:        "includes all subdomains by default",
			rules:       []string{"domain=github.com"},
			url:         "https://x.users.api.github.com",
			method:      "GET",
			expectParse: true,
			expectMatch: true,
		},
		{
			name:        "domain wildcard in the middle matches exactly one label",
			rules:       []string{"domain=api.*.com"},
			url:         "https://api.v1.github.com",
			method:      "POST",
			expectParse: true,
			expectMatch: false,
		},
	}

	logHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(logHandler)

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			rules, err := ParseAllowSpecs(tc.rules)
			if tc.expectParse {
				require.Nil(t, err)
				engine := NewRuleEngine(rules, logger)
				result := engine.Evaluate(tc.method, tc.url)
				require.Equal(t, tc.expectMatch, result.Allowed)
			} else {
				require.NotNil(t, err)
			}
		})
	}
}
