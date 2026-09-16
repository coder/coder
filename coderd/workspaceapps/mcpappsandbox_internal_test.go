package workspaceapps

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
)

func Test_validateCSPDomain(t *testing.T) {
	t.Parallel()

	const (
		dashboardHost  = "coder.example.com"
		accessHost     = "proxy.example.org"
		wildcardSuffix = "apps.example.com"
	)
	protectedHosts := []string{dashboardHost, accessHost}

	cases := []struct {
		name  string
		entry string
		// wantErr is a substring of the expected error. Empty means the entry
		// must be accepted.
		wantErr string
	}{
		// Accepted entries.
		{name: "HTTPS", entry: "https://api.other.com"},
		{name: "WSS", entry: "wss://api.other.com"},
		{name: "UppercaseScheme", entry: "HTTPS://API.other.com"},
		{name: "Port", entry: "https://api.other.com:8443"},
		{name: "TrailingSlash", entry: "https://api.other.com/"},
		{name: "LeadingWildcard", entry: "https://*.other.com"},
		{name: "WildcardWithPort", entry: "wss://*.other.com:443"},
		{name: "Hyphen", entry: "https://my-api.other-site.com"},
		{name: "SiblingOfDashboard", entry: "https://other.example.com"},
		{name: "SiblingOfWildcard", entry: "https://apps2.example.com"},
		{name: "SuffixLookalike", entry: "https://notapps.example.com"},
		{name: "DashboardLookalike", entry: "https://notcoder.example.com"},
		{name: "AccessHostLookalike", entry: "https://notproxy.example.org"},
		{name: "NumericMiddleLabel", entry: "https://123.other.com"},
		{name: "HexMiddleLabel", entry: "https://0x7f.other.com"},
		{name: "DigitsInLastLabel", entry: "https://other.c0m"},

		// Rejected entries.
		{name: "Empty", entry: "", wantErr: "empty"},
		{name: "SchemeLess", entry: "api.other.com", wantErr: "scheme"},
		{name: "SchemeOnly", entry: "https://", wantErr: "host"},
		{name: "HTTP", entry: "http://api.other.com", wantErr: "not https or wss"},
		{name: "WS", entry: "ws://api.other.com", wantErr: "not https or wss"},
		{name: "Path", entry: "https://api.other.com/v1", wantErr: "path"},
		{name: "Query", entry: "https://api.other.com/?x=1", wantErr: "characters"},
		{name: "QueryOnlyAllowedChars", entry: "https://api.other.com?", wantErr: "characters"},
		{name: "Fragment", entry: "https://api.other.com#f", wantErr: "characters"},
		{name: "UserInfo", entry: "https://user@api.other.com", wantErr: "characters"},
		{name: "NonNumericPort", entry: "https://api.other.com:abc", wantErr: "port"},
		{name: "PortTooLarge", entry: "https://api.other.com:70000", wantErr: "port"},
		{name: "BareWildcard", entry: "*", wantErr: "scheme"},
		{name: "SchemeWildcard", entry: "https://*", wantErr: "wildcard"},
		{name: "WildcardDot", entry: "https://*.", wantErr: "wildcard"},
		{name: "DoubleWildcard", entry: "https://*.*.other.com", wantErr: "wildcard"},
		{name: "MiddleWildcard", entry: "https://api.*.other.com", wantErr: "wildcard"},
		{name: "PartialWildcardLabel", entry: "https://*api.other.com", wantErr: "wildcard"},
		{name: "SelfKeyword", entry: "'self'", wantErr: "characters"},
		{name: "UnsafeInlineKeyword", entry: "'unsafe-inline'", wantErr: "characters"},
		{name: "DataScheme", entry: "data:", wantErr: "scheme"},
		{name: "BlobScheme", entry: "blob:", wantErr: "scheme"},
		{name: "Semicolon", entry: "https://api.other.com;script-src", wantErr: "characters"},
		{name: "Space", entry: "https://api.other.com https://evil.com", wantErr: "characters"},
		{name: "Newline", entry: "https://api.other.com\nX", wantErr: "characters"},
		{name: "Quote", entry: "https://api.other.com'", wantErr: "characters"},
		{name: "Underscore", entry: "https://api_v2.other.com", wantErr: "characters"},
		{name: "TrailingDot", entry: "https://api.other.com.", wantErr: "label"},
		{name: "EmptyLabel", entry: "https://api..other.com", wantErr: "label"},
		{name: "IPv4", entry: "https://1.2.3.4", wantErr: "IP address"},
		{name: "IPv4Port", entry: "https://1.2.3.4:443", wantErr: "IP address"},
		{name: "IPv6", entry: "https://[::1]", wantErr: "characters"},
		{name: "IPv6Port", entry: "https://[2001:db8::1]:443", wantErr: "characters"},
		{name: "IPv4Decimal", entry: "https://2130706433", wantErr: "IP address"},
		{name: "IPv4Hex", entry: "https://0x7f000001", wantErr: "IP address"},
		{name: "IPv4HexUppercase", entry: "https://0X7F000001", wantErr: "IP address"},
		{name: "IPv4Short", entry: "https://127.1", wantErr: "IP address"},
		{name: "IPv4ShortPort", entry: "https://127.1:443", wantErr: "IP address"},
		{name: "IPv4MixedHex", entry: "https://127.0.0.0x1", wantErr: "IP address"},
		{name: "IPv4Octal", entry: "https://0177.0.0.1", wantErr: "IP address"},
		{name: "WildcardNumeric", entry: "https://*.1", wantErr: "IP address"},
		{name: "Localhost", entry: "https://localhost", wantErr: "localhost"},
		{name: "LocalhostPort", entry: "https://localhost:3000", wantErr: "localhost"},
		{name: "LocalhostUppercase", entry: "https://LOCALHOST", wantErr: "localhost"},
		{name: "DotLocalhost", entry: "https://app.localhost", wantErr: "localhost"},
		{name: "WildcardLocalhost", entry: "https://*.localhost", wantErr: "localhost"},
		{name: "DashboardHost", entry: "https://coder.example.com", wantErr: "access URL"},
		{name: "DashboardHostUppercase", entry: "https://CODER.example.com", wantErr: "access URL"},
		{name: "DashboardHostPort", entry: "wss://coder.example.com:443", wantErr: "access URL"},
		{name: "WildcardCoveringDashboard", entry: "https://*.example.com", wantErr: "access URL"},
		{name: "AccessHost", entry: "https://proxy.example.org", wantErr: "access URL"},
		{name: "AccessHostUppercase", entry: "wss://PROXY.example.org:443", wantErr: "access URL"},
		{name: "WildcardCoveringAccessHost", entry: "https://*.example.org", wantErr: "access URL"},
		{name: "WildcardSuffix", entry: "https://apps.example.com", wantErr: "wildcard access URL"},
		{name: "UnderWildcardSuffix", entry: "https://foo.apps.example.com", wantErr: "wildcard access URL"},
		{name: "WildcardUnderWildcardSuffix", entry: "https://*.apps.example.com", wantErr: "wildcard access URL"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateCSPDomain(tc.entry, protectedHosts, wildcardSuffix)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func Test_validateCSPDomainWildcardPattern(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		pattern string
		entry   string
		wantErr string
	}{
		{name: "DotPatternUnrelated", pattern: "*.apps.example.com", entry: "https://api.other.com"},
		{name: "DotPatternSibling", pattern: "*.apps.example.com", entry: "https://coder.example.com"},
		{name: "DotPatternMatch", pattern: "*.apps.example.com", entry: "https://foo.apps.example.com", wantErr: "matches"},
		{name: "DotPatternWildcardCovers", pattern: "*.apps.example.com", entry: "https://*.example.com", wantErr: "covers"},
		{name: "DotPatternWildcardEqual", pattern: "*.apps.example.com", entry: "https://*.apps.example.com", wantErr: "covers"},
		{name: "DashPatternUnrelated", pattern: "*--apps.example.com", entry: "https://api.other.com"},
		{name: "DashPatternSibling", pattern: "*--apps.example.com", entry: "https://coder.example.com"},
		{name: "DashPatternMatch", pattern: "*--apps.example.com", entry: "https://foo--apps.example.com", wantErr: "matches"},
		{name: "DashPatternMatchUppercase", pattern: "*--apps.example.com", entry: "https://FOO--APPS.example.com", wantErr: "matches"},
		{name: "DashPatternWildcardCovers", pattern: "*--apps.example.com", entry: "https://*.example.com", wantErr: "covers"},
		{name: "DashPatternWildcardCoversTLD", pattern: "*--apps.example.com", entry: "https://*.com", wantErr: "covers"},
		{name: "PatternWithPort", pattern: "*.apps.example.com:8443", entry: "https://*.example.com", wantErr: "covers"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hostnameRegex, err := appurl.CompileHostnamePattern(tc.pattern)
			require.NoError(t, err)

			err = validateCSPDomainWildcardPattern(tc.entry, hostnameRegex, tc.pattern)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}

	t.Run("NoPattern", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validateCSPDomainWildcardPattern("https://*.com", nil, ""))
	})
}

func Test_mcpAppSandboxWildcardSuffix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		pattern string
		want    string
	}{
		{pattern: "*.apps.example.com", want: "apps.example.com"},
		{pattern: "*.APPS.Example.com", want: "apps.example.com"},
		{pattern: "*.apps.example.com:8443", want: "apps.example.com"},
		{pattern: "*--apps.example.com", want: "--apps.example.com"},
	}

	for _, tc := range cases {
		t.Run(tc.pattern, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, mcpAppSandboxWildcardSuffix(tc.pattern))
		})
	}
}

func Test_buildMCPAppSandboxCSP(t *testing.T) {
	t.Parallel()

	const frameAncestor = "https://coder.example.com"

	t.Run("Default", func(t *testing.T) {
		t.Parallel()

		got := buildMCPAppSandboxCSP(mcpAppSandboxCSP{}, frameAncestor)
		want := strings.Join([]string{
			"default-src 'none'",
			"script-src 'self' 'unsafe-inline'",
			"style-src 'self' 'unsafe-inline'",
			"connect-src 'self'",
			"img-src 'self' data:",
			"font-src 'self'",
			"media-src 'self' data:",
			"frame-src 'none'",
			"object-src 'none'",
			"base-uri 'self'",
			"form-action 'none'",
			"frame-ancestors https://coder.example.com",
		}, "; ")
		require.Equal(t, want, got)
	})

	t.Run("WithDomains", func(t *testing.T) {
		t.Parallel()

		got := buildMCPAppSandboxCSP(mcpAppSandboxCSP{
			ConnectDomains:  []string{"https://api.other.com", "wss://ws.other.com"},
			ResourceDomains: []string{"https://cdn.other.com"},
			FrameDomains:    []string{"https://frames.other.com"},
			BaseURIDomains:  []string{"https://base.other.com"},
		}, frameAncestor)
		want := strings.Join([]string{
			"default-src 'none'",
			"script-src 'self' 'unsafe-inline' https://cdn.other.com",
			"style-src 'self' 'unsafe-inline' https://cdn.other.com",
			"connect-src 'self' https://api.other.com wss://ws.other.com",
			"img-src 'self' data: https://cdn.other.com",
			"font-src 'self' https://cdn.other.com",
			"media-src 'self' data: https://cdn.other.com",
			"frame-src https://frames.other.com",
			"object-src 'none'",
			"base-uri https://base.other.com",
			"form-action 'none'",
			"frame-ancestors https://coder.example.com",
		}, "; ")
		require.Equal(t, want, got)
	})
}
