package httpapi_test

import (
	"testing"

	"github.com/coder/coder/v2/coderd/httpapi"
)

func TestSafeRedirectPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "/"},
		{"simple path", "/foo/bar", "/foo/bar"},
		{"path with query", "/foo/bar?baz=qux", "/foo/bar?baz=qux"},
		{"no leading slash", "foo/bar", "/foo/bar"},
		{"malformed", "http://[::1]:namedport", "/"},
		// Cure53 CDM-02-009: triple-slash open redirect.
		{"protocol relative triple slash", "///evil.example.com", "/evil.example.com"},
		{"protocol relative double slash", "//evil.example.com", "/"},
		{"absolute url with host", "http://evil.example.com/path", "/path"},
		{"absolute url with host and query", "https://evil.example.com/path?a=b", "/path?a=b"},
		// Cure53 CDM-02-009: javascript: scheme bypassing CSP.
		{"javascript scheme", "javascript:alert(origin)", "/"},
		{"nested javascript scheme", "javascript:javascript:javascript:alert(origin)", "/"},
		{"data scheme", "data:text/html,<script>alert(origin)</script>", "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := httpapi.SafeRedirectPath(tt.in); got != tt.want {
				t.Errorf("SafeRedirectPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
