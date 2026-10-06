package codersdk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestChatGatewayConfiguration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, target, key string
		valid             bool
	}{
		{"embedded", "", "", true},
		{"standalone", "https://gateway.example.com", "shared-key", true},
		{"prefix", "http://gateway.example.com:4001/prefix", "shared-key", true},
		{"missing key", "https://gateway.example.com", "", false},
		{"missing URL", "", "shared-key", false},
		{"relative URL", "/gateway", "shared-key", false},
		{"missing host", "https://:4001", "shared-key", false},
		{"port out of range", "https://gateway.example.com:65536", "shared-key", false},
		{"port zero", "https://gateway.example.com:0", "shared-key", false},
		{"scheme", "ftp://gateway.example.com", "shared-key", false},
		{"userinfo", "https://user:secret@gateway.example.com", "shared-key", false},
		{"query", "https://gateway.example.com?key=secret", "shared-key", false},
		{"empty query", "https://gateway.example.com?", "shared-key", false},
		{"fragment", "https://gateway.example.com#fragment", "shared-key", false},
		{"whitespace key", "https://gateway.example.com", " shared-key ", false},
		{"invalid key", "https://gateway.example.com", "shared-key\r\ninjected", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var dv codersdk.DeploymentValues
			opts := dv.Options()
			require.NoError(t, opts.SetDefaults())
			require.True(t, dv.AI.BridgeConfig.EmbeddedEnabled.Value())
			require.NoError(t, dv.AI.Chat.AIGatewayURL.Set(tc.target))
			dv.AI.Chat.AIGatewayKey = serpent.String(tc.key)
			err := dv.Validate()
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "shared-key")
			}
		})
	}
}

func TestChatGatewayKeyRedacted(t *testing.T) {
	t.Parallel()
	var dv codersdk.DeploymentValues
	opts := dv.Options()
	require.NoError(t, opts.SetDefaults())
	dv.AI.Chat.AIGatewayKey = "secret-shared-key"
	sanitized, err := dv.WithoutSecrets()
	require.NoError(t, err)
	require.Empty(t, sanitized.AI.Chat.AIGatewayKey)
	require.Equal(t, "secret-shared-key", dv.AI.Chat.AIGatewayKey.Value())
}
