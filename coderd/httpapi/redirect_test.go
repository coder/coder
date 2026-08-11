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
		{"path with fragment", "/foo/bar#wooble", "/foo/bar#wooble"},
		{"path with query+fragment", "/foo/bar?wibble=wobble#wooble", "/foo/bar?wibble=wobble#wooble"},
		{"no leading slash", "foo/bar", "/foo/bar"},
		{"malformed", "http://[::1]:namedport", "/"},
		// Ensure backslashes aren't a blindspot.
		{"backslash after slash", `/\evil.example.com`, "/%5Cevil.example.com"},
		{"leading double backslash", `\\evil.example.com`, "/%5C%5Cevil.example.com"},
		{"backslash then slash", `\/evil.example.com`, "/%5C/evil.example.com"},
		{"mixed slash backslash", `/\/evil.example.com`, "/%5C/evil.example.com"},
		{"scheme with backslash", `https:/\evil.example.com`, "/%5Cevil.example.com"},
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
