package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

func TestEffectiveChatProviderConfigHasAPIKey(t *testing.T) {
	t.Parallel()

	fallback := chatprovider.ProviderAPIKeys{
		ByProvider: map[string]string{"openai": "deployment-key"},
	}

	cases := []struct {
		name     string
		provider database.ChatProvider
		fallback chatprovider.ProviderAPIKeys
		want     bool
	}{
		{
			name: "CentralDisabledIgnoresStoredKey",
			provider: database.ChatProvider{
				Provider:             "openai",
				APIKey:               "stored-key",
				CentralApiKeyEnabled: false,
			},
			fallback: fallback,
			want:     false,
		},
		{
			name: "CentralDisabledIgnoresFallbackKey",
			provider: database.ChatProvider{
				Provider:             "openai",
				CentralApiKeyEnabled: false,
			},
			fallback: fallback,
			want:     false,
		},
		{
			name: "CentralEnabledUsesStoredKey",
			provider: database.ChatProvider{
				Provider:             "openai",
				APIKey:               "stored-key",
				CentralApiKeyEnabled: true,
			},
			fallback: chatprovider.ProviderAPIKeys{},
			want:     true,
		},
		{
			name: "CentralEnabledUsesFallbackKey",
			provider: database.ChatProvider{
				Provider:             "openai",
				CentralApiKeyEnabled: true,
			},
			fallback: fallback,
			want:     true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, effectiveChatProviderConfigHasAPIKey(tc.provider, tc.fallback))
		})
	}
}

func TestChatProviderConfigKeyBooleans(t *testing.T) {
	t.Parallel()

	fallback := chatprovider.ProviderAPIKeys{
		ByProvider: map[string]string{"openai": "fallback-key"},
	}

	cases := []struct {
		name             string
		provider         database.ChatProvider
		fallback         chatprovider.ProviderAPIKeys
		wantHasAPIKey    bool
		wantHasEffective bool
	}{
		{
			name: "StoredKeyPresentCentralDisabled",
			provider: database.ChatProvider{
				APIKey:               "stored-key",
				CentralApiKeyEnabled: false,
			},
			fallback:         chatprovider.ProviderAPIKeys{},
			wantHasAPIKey:    true,
			wantHasEffective: false,
		},
		{
			name: "StoredKeyPresentCentralEnabled",
			provider: database.ChatProvider{
				Provider:             "openai",
				APIKey:               "stored-key",
				CentralApiKeyEnabled: true,
			},
			fallback:         chatprovider.ProviderAPIKeys{},
			wantHasAPIKey:    true,
			wantHasEffective: true,
		},
		{
			name: "NoStoredKeyCentralEnabledFamilyFallback",
			provider: database.ChatProvider{
				Provider:             "openai",
				CentralApiKeyEnabled: true,
			},
			fallback:         fallback,
			wantHasAPIKey:    false,
			wantHasEffective: true,
		},
		{
			name: "NoStoredKeyCentralDisabledFamilyFallback",
			provider: database.ChatProvider{
				Provider:             "openai",
				CentralApiKeyEnabled: false,
			},
			fallback:         fallback,
			wantHasAPIKey:    false,
			wantHasEffective: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hasAPIKey, hasEffectiveAPIKey := chatProviderConfigKeyBooleans(tc.provider, tc.fallback)

			require.Equal(t, tc.wantHasAPIKey, hasAPIKey)
			require.Equal(t, tc.wantHasEffective, hasEffectiveAPIKey)
		})
	}
}
