//go:build !slim

package cli

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/database"
)

func TestResolveAIGatewayProxyTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		accessURL   *url.URL
		target      string
		want        string
		wantErr     bool
		errContains string
	}{
		{
			name:      "ExplicitTarget",
			accessURL: &url.URL{Scheme: "https", Host: "coder.example.com", Path: "/coder"},
			target:    "https://gateway.example.com/custom/path",
			want:      "https://gateway.example.com/custom/path",
		},
		{
			name:      "EmbeddedFallback",
			accessURL: &url.URL{Scheme: "https", Host: "coder.example.com", Path: "/coder"},
			want:      "https://coder.example.com/coder" + agplaibridge.AIGatewayRootPath,
		},
		{
			name:        "InvalidAccessURL",
			accessURL:   &url.URL{Scheme: "https", Host: "[::1"},
			wantErr:     true,
			errContains: "build embedded AI Gateway proxy target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveAIGatewayProxyTarget(tt.accessURL, tt.target)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.errContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestClassifyProviderRow covers every branch of the classifier so the
// disabled, error, and enabled paths are exercised through the
// production code instead of relying on classifyRaw, the test mirror in
// reload_test.go.
func TestClassifyProviderRow(t *testing.T) {
	t.Parallel()

	enabledRow := func(name, baseURL string) database.AIProvider {
		return database.AIProvider{
			Name:    name,
			Type:    database.AIProviderTypeOpenai,
			Enabled: true,
			BaseUrl: baseURL,
		}
	}

	t.Run("Enabled", func(t *testing.T) {
		t.Parallel()

		seen := map[string]string{}
		got := classifyProviderRow(enabledRow("openai", "https://api.openai.com/v1"), seen)
		assert.Equal(t, "openai", got.Name)
		assert.Equal(t, string(database.AIProviderTypeOpenai), got.Type)
		assert.Equal(t, aibridged.ProviderStatusEnabled, got.Status)
		assert.Equal(t, "api.openai.com", got.Host)
		assert.NoError(t, got.Err)
		assert.Equal(t, "openai", seen["api.openai.com"])
	})

	t.Run("DisabledRow", func(t *testing.T) {
		t.Parallel()

		seen := map[string]string{}
		row := enabledRow("off", "https://api.off.example.com/v1")
		row.Enabled = false
		got := classifyProviderRow(row, seen)
		assert.Equal(t, aibridged.ProviderStatusDisabled, got.Status)
		assert.Empty(t, got.Host, "disabled provider must not claim a host")
		assert.NoError(t, got.Err)
		assert.Empty(t, seen, "disabled provider must not occupy a host slot")
	})

	t.Run("EmptyBaseURL", func(t *testing.T) {
		t.Parallel()

		seen := map[string]string{}
		got := classifyProviderRow(enabledRow("no-url", "   "), seen)
		assert.Equal(t, aibridged.ProviderStatusError, got.Status)
		assert.Empty(t, got.Host)
		assert.ErrorContains(t, got.Err, "base url is empty")
	})

	t.Run("MalformedBaseURL", func(t *testing.T) {
		t.Parallel()

		seen := map[string]string{}
		got := classifyProviderRow(enabledRow("bad", "://not-a-url"), seen)
		assert.Equal(t, aibridged.ProviderStatusError, got.Status)
		assert.ErrorContains(t, got.Err, "invalid base url")
	})

	t.Run("BaseURLWithoutHostname", func(t *testing.T) {
		t.Parallel()

		seen := map[string]string{}
		got := classifyProviderRow(enabledRow("no-host", "https://"), seen)
		assert.Equal(t, aibridged.ProviderStatusError, got.Status)
		assert.ErrorContains(t, got.Err, "no hostname")
	})

	t.Run("DuplicateHostnameFirstWins", func(t *testing.T) {
		t.Parallel()

		seen := map[string]string{}
		first := classifyProviderRow(enabledRow("first", "https://shared.example.com/v1"), seen)
		assert.Equal(t, aibridged.ProviderStatusEnabled, first.Status)

		second := classifyProviderRow(enabledRow("second", "https://shared.example.com/v2"), seen)
		assert.Equal(t, aibridged.ProviderStatusError, second.Status)
		assert.ErrorContains(t, second.Err, "already claimed by provider \"first\"")
		assert.Equal(t, "first", seen["shared.example.com"], "first wins must not be overwritten")
	})

	t.Run("HostnameLowercased", func(t *testing.T) {
		t.Parallel()

		seen := map[string]string{}
		got := classifyProviderRow(enabledRow("mixed", "https://API.Example.COM/v1"), seen)
		assert.Equal(t, aibridged.ProviderStatusEnabled, got.Status)
		assert.Equal(t, "api.example.com", got.Host)
	})
}
