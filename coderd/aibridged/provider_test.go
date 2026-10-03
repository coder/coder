package aibridged_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridged"
)

func TestBaseURLHostname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "URL", baseURL: "https://openrouter.ai/api/v1", want: "openrouter.ai"},
		{name: "BareHost", baseURL: "openrouter.ai", want: "openrouter.ai"},
		{name: "HostWithPort", baseURL: "https://openrouter.ai:443/api/v1", want: "openrouter.ai"},
		{name: "UppercaseHost", baseURL: "https://API.OpenRouter.AI/v1", want: "api.openrouter.ai"},
		{name: "IPv6", baseURL: "https://[::1]:8080/v1", want: "::1"},
		{name: "Empty", baseURL: "", want: ""},
		{name: "Invalid", baseURL: "://", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, aibridged.BaseURLHostname(tt.baseURL))
		})
	}
}
