package aibridged_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridged"
)

// TestBaseURLHostname pins the strict contract: only an absolute http
// or https URL with a hostname yields a hostname. Scheme-less and
// otherwise malformed values return "" so callers surface the provider
// as misconfigured rather than routing to a synthesized host.
func TestBaseURLHostname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "HTTPS", baseURL: "https://openrouter.ai/api/v1", want: "openrouter.ai"},
		{name: "HTTP", baseURL: "http://llm-relay.internal/v1", want: "llm-relay.internal"},
		{name: "HostWithPort", baseURL: "https://openrouter.ai:443/api/v1", want: "openrouter.ai"},
		{name: "HTTPHostWithPort", baseURL: "http://localhost:8080/v1", want: "localhost"},
		{name: "UppercaseHost", baseURL: "https://API.OpenRouter.AI/v1", want: "api.openrouter.ai"},
		{name: "UppercaseScheme", baseURL: "HTTPS://OpenRouter.AI/api/v1", want: "openrouter.ai"},
		{name: "SurroundingWhitespace", baseURL: "  https://openrouter.ai/api/v1\n", want: "openrouter.ai"},
		{name: "IPv6", baseURL: "https://[::1]:8080/v1", want: "::1"},
		{name: "BareHost", baseURL: "openrouter.ai", want: ""},
		{name: "SchemeLessPath", baseURL: "openrouter.ai/api/v1", want: ""},
		{name: "SchemeLessHostWithPort", baseURL: "localhost:8080", want: ""},
		{name: "ProtocolRelative", baseURL: "//openrouter.ai/api/v1", want: ""},
		{name: "UnsupportedScheme", baseURL: "ftp://openrouter.ai/api/v1", want: ""},
		{name: "FileScheme", baseURL: "file:///etc/hosts", want: ""},
		{name: "Empty", baseURL: "", want: ""},
		{name: "Whitespace", baseURL: "   ", want: ""},
		{name: "MissingScheme", baseURL: "://", want: ""},
		{name: "NoHost", baseURL: "https://", want: ""},
		{name: "UnbracketedIPv6", baseURL: "https://[::1", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, aibridged.BaseURLHostname(tt.baseURL))
		})
	}
}
