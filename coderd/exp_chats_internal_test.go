package coderd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

func TestShouldCleanUnboundModelsAfterProviderDelete(t *testing.T) {
	t.Parallel()

	deletedProvider := database.ChatProvider{
		ID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Provider: "openai",
		Enabled:  true,
	}

	t.Run("LastEnabledProviderWithoutFallback", func(t *testing.T) {
		t.Parallel()

		require.True(t, shouldCleanUnboundModelsAfterProviderDelete(
			deletedProvider,
			1,
			[]database.ChatProvider{deletedProvider},
			chatprovider.ProviderAPIKeys{},
		))
	})

	t.Run("DeploymentFallbackKeepsFamilyRunnable", func(t *testing.T) {
		t.Parallel()

		require.False(t, shouldCleanUnboundModelsAfterProviderDelete(
			deletedProvider,
			0,
			[]database.ChatProvider{deletedProvider},
			chatprovider.ProviderAPIKeys{
				ByProvider: map[string]string{"openai": "env-key"},
			},
		))
	})

	t.Run("DeletingDisabledProviderWithDisabledSiblingDoesNotClean", func(t *testing.T) {
		t.Parallel()

		disabledProvider := deletedProvider
		disabledProvider.Enabled = false

		require.False(t, shouldCleanUnboundModelsAfterProviderDelete(
			disabledProvider,
			1,
			[]database.ChatProvider{},
			chatprovider.ProviderAPIKeys{},
		))
	})

	t.Run("EnabledSiblingKeepsFamilyRunnable", func(t *testing.T) {
		t.Parallel()

		sibling := deletedProvider
		sibling.ID = uuid.MustParse("00000000-0000-0000-0000-000000000002")

		require.False(t, shouldCleanUnboundModelsAfterProviderDelete(
			deletedProvider,
			1,
			[]database.ChatProvider{deletedProvider, sibling},
			chatprovider.ProviderAPIKeys{},
		))
	})
}

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
		name          string
		provider      database.ChatProvider
		fallback      chatprovider.ProviderAPIKeys
		wantHasAPIKey bool
	}{
		{
			name: "StoredKeyPresentCentralDisabled",
			provider: database.ChatProvider{
				APIKey:               "stored-key",
				CentralApiKeyEnabled: false,
			},
			fallback:      chatprovider.ProviderAPIKeys{},
			wantHasAPIKey: true,
		},
		{
			name: "StoredKeyPresentCentralEnabled",
			provider: database.ChatProvider{
				Provider:             "openai",
				APIKey:               "stored-key",
				CentralApiKeyEnabled: true,
			},
			fallback:      chatprovider.ProviderAPIKeys{},
			wantHasAPIKey: true,
		},
		{
			name: "NoStoredKeyCentralEnabledFamilyFallback",
			provider: database.ChatProvider{
				Provider:             "openai",
				CentralApiKeyEnabled: true,
			},
			fallback:      fallback,
			wantHasAPIKey: false,
		},
		{
			name: "NoStoredKeyCentralDisabledFamilyFallback",
			provider: database.ChatProvider{
				Provider:             "openai",
				CentralApiKeyEnabled: false,
			},
			fallback:      fallback,
			wantHasAPIKey: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hasAPIKey := chatProviderConfigKeyBooleans(tc.provider, tc.fallback)

			require.Equal(t, tc.wantHasAPIKey, hasAPIKey)
		})
	}
}

func TestResolveVisibleProviderAvailability(t *testing.T) {
	t.Parallel()

	configuredProvider := func(id uuid.UUID, provider string, apiKey string, central bool, allowUser bool, allowFallback bool) chatprovider.ConfiguredProvider {
		return chatprovider.ConfiguredProvider{
			ProviderID:                 id,
			Provider:                   provider,
			APIKey:                     apiKey,
			CentralAPIKeyEnabled:       central,
			AllowUserAPIKey:            allowUser,
			AllowCentralAPIKeyFallback: allowFallback,
		}
	}

	cases := []struct {
		name      string
		fallback  chatprovider.ProviderAPIKeys
		providers []chatprovider.ConfiguredProvider
		userKeys  []chatprovider.UserProviderKey
		provider  string
		want      chatprovider.ProviderAvailability
	}{
		{
			name: "FamilyAvailableWhenAnySiblingHasCentralKey",
			providers: []chatprovider.ConfiguredProvider{
				configuredProvider(uuid.MustParse("00000000-0000-0000-0000-000000000011"), "openai", "central-key", true, false, false),
				configuredProvider(uuid.MustParse("00000000-0000-0000-0000-000000000012"), "openai", "", false, true, false),
			},
			provider: "openai",
			want:     chatprovider.ProviderAvailability{Available: true},
		},
		{
			name: "FamilyAvailableWhenUserKeyPresent",
			providers: []chatprovider.ConfiguredProvider{
				configuredProvider(uuid.MustParse("00000000-0000-0000-0000-000000000021"), "anthropic", "", false, true, false),
			},
			userKeys: []chatprovider.UserProviderKey{{
				ChatProviderID: uuid.MustParse("00000000-0000-0000-0000-000000000021"),
				APIKey:         "user-key",
			}},
			provider: "anthropic",
			want:     chatprovider.ProviderAvailability{Available: true},
		},
		{
			name: "FamilyAvailableWhenFallbackKeyPresent",
			fallback: chatprovider.ProviderAPIKeys{
				ByProvider: map[string]string{"openai": "fallback-key"},
			},
			providers: []chatprovider.ConfiguredProvider{
				configuredProvider(uuid.MustParse("00000000-0000-0000-0000-000000000031"), "openai", "", true, true, true),
			},
			provider: "openai",
			want:     chatprovider.ProviderAvailability{Available: true},
		},
		{
			name: "MissingKeyRequiresUserAPIKeyWhenUserCanFixIt",
			providers: []chatprovider.ConfiguredProvider{
				configuredProvider(uuid.MustParse("00000000-0000-0000-0000-000000000041"), "openai", "", false, true, false),
			},
			provider: "openai",
			want: chatprovider.ProviderAvailability{
				UnavailableReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			availabilityByProvider := resolveVisibleProviderAvailability(tc.fallback, tc.providers, tc.userKeys)
			require.Equal(t, tc.want, availabilityByProvider[tc.provider])
		})
	}
}

func TestMergeProviderAvailability(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current chatprovider.ProviderAvailability
		next    chatprovider.ProviderAvailability
		want    chatprovider.ProviderAvailability
	}{
		{
			name:    "AvailableWins",
			current: chatprovider.ProviderAvailability{Available: true},
			next: chatprovider.ProviderAvailability{
				UnavailableReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired,
			},
			want: chatprovider.ProviderAvailability{Available: true},
		},
		{
			name: "UserAPIKeyRequiredBeatsFetchFailed",
			current: chatprovider.ProviderAvailability{
				UnavailableReason: codersdk.ChatModelProviderUnavailableFetchFailed,
			},
			next: chatprovider.ProviderAvailability{
				UnavailableReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired,
			},
			want: chatprovider.ProviderAvailability{
				UnavailableReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired,
			},
		},
		{
			name: "FetchFailedBeatsMissingAPIKey",
			current: chatprovider.ProviderAvailability{
				UnavailableReason: codersdk.ChatModelProviderUnavailableFetchFailed,
			},
			next: chatprovider.ProviderAvailability{
				UnavailableReason: codersdk.ChatModelProviderUnavailableMissingAPIKey,
			},
			want: chatprovider.ProviderAvailability{
				UnavailableReason: codersdk.ChatModelProviderUnavailableFetchFailed,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, mergeProviderAvailability(tc.current, tc.next))
		})
	}
}

func TestFilterUserVisibleChatModelConfigs(t *testing.T) {
	t.Parallel()

	provider := func(id uuid.UUID, family string, apiKey string, central bool, allowUser bool) database.ChatProvider {
		return database.ChatProvider{
			ID:                   id,
			Provider:             family,
			APIKey:               apiKey,
			CentralApiKeyEnabled: central,
			AllowUserApiKey:      allowUser,
		}
	}
	modelConfig := func(id uuid.UUID, family string, providerConfigID *uuid.UUID) database.ChatModelConfig {
		config := database.ChatModelConfig{
			ID:       id,
			Provider: family,
			Model:    "test-model",
		}
		if providerConfigID != nil {
			config.ProviderConfigID = uuid.NullUUID{UUID: *providerConfigID, Valid: true}
		}
		return config
	}

	t.Run("BoundModelHiddenWhenProviderIsNotUserVisible", func(t *testing.T) {
		t.Parallel()

		visibleProvider := provider(
			uuid.MustParse("00000000-0000-0000-0000-000000000101"),
			"openai",
			"central-key",
			true,
			false,
		)
		hiddenProviderID := uuid.MustParse("00000000-0000-0000-0000-000000000102")
		hiddenProvider := provider(hiddenProviderID, "openai", "", false, false)
		boundModel := modelConfig(
			uuid.MustParse("00000000-0000-0000-0000-000000001001"),
			"openai",
			&hiddenProviderID,
		)
		unboundModel := modelConfig(
			uuid.MustParse("00000000-0000-0000-0000-000000001002"),
			"openai",
			nil,
		)
		configs := []database.ChatModelConfig{boundModel, unboundModel}
		originalConfigs := append([]database.ChatModelConfig(nil), configs...)

		visibleConfigs := filterUserVisibleChatModelConfigs(
			configs,
			[]database.ChatProvider{visibleProvider, hiddenProvider},
			chatprovider.ProviderAPIKeys{},
		)

		require.Equal(t, originalConfigs, configs)
		require.Equal(t, []database.ChatModelConfig{unboundModel}, visibleConfigs)
	})

	t.Run("BoundModelHiddenWhenProviderIsMissing", func(t *testing.T) {
		t.Parallel()

		providerConfigID := uuid.MustParse("00000000-0000-0000-0000-000000000103")
		boundModel := modelConfig(
			uuid.MustParse("00000000-0000-0000-0000-000000001003"),
			"openai",
			&providerConfigID,
		)

		visibleConfigs := filterUserVisibleChatModelConfigs(
			[]database.ChatModelConfig{boundModel},
			nil,
			chatprovider.ProviderAPIKeys{},
		)

		require.Empty(t, visibleConfigs)
	})

	t.Run("UnboundModelKeptWhenFallbackProvidesKey", func(t *testing.T) {
		t.Parallel()

		unboundModel := modelConfig(
			uuid.MustParse("00000000-0000-0000-0000-000000001004"),
			"openai",
			nil,
		)

		visibleConfigs := filterUserVisibleChatModelConfigs(
			[]database.ChatModelConfig{unboundModel},
			nil,
			chatprovider.ProviderAPIKeys{
				ByProvider: map[string]string{"openai": "fallback-key"},
			},
		)

		require.Equal(t, []database.ChatModelConfig{unboundModel}, visibleConfigs)
	})
}

func TestRuntimeProvidersForVisibleModels(t *testing.T) {
	t.Parallel()

	provider := func(id uuid.UUID, family string, apiKey string, central bool, allowUser bool) database.ChatProvider {
		return database.ChatProvider{
			ID:                   id,
			Provider:             family,
			APIKey:               apiKey,
			CentralApiKeyEnabled: central,
			AllowUserApiKey:      allowUser,
		}
	}
	modelConfig := func(id uuid.UUID, family string, providerConfigID *uuid.UUID) database.ChatModelConfig {
		config := database.ChatModelConfig{
			ID:       id,
			Provider: family,
			Model:    "test-model",
		}
		if providerConfigID != nil {
			config.ProviderConfigID = uuid.NullUUID{UUID: *providerConfigID, Valid: true}
		}
		return config
	}

	t.Run("BoundModelsUseBoundProviderOnly", func(t *testing.T) {
		t.Parallel()

		hiddenDefault := provider(
			uuid.MustParse("00000000-0000-0000-0000-000000000201"),
			"openai",
			"",
			false,
			false,
		)
		visibleBoundID := uuid.MustParse("00000000-0000-0000-0000-000000000202")
		visibleBound := provider(visibleBoundID, "openai", "", false, true)
		boundModel := modelConfig(
			uuid.MustParse("00000000-0000-0000-0000-000000002001"),
			"openai",
			&visibleBoundID,
		)

		providers := runtimeProvidersForVisibleModels(
			[]database.ChatModelConfig{boundModel},
			[]database.ChatProvider{hiddenDefault, visibleBound},
		)

		require.Equal(t, []database.ChatProvider{visibleBound}, providers)
	})

	t.Run("UnboundModelsUseFamilyDefault", func(t *testing.T) {
		t.Parallel()

		defaultProvider := provider(
			uuid.MustParse("00000000-0000-0000-0000-000000000211"),
			"openai",
			"central-key",
			true,
			false,
		)
		secondaryProvider := provider(
			uuid.MustParse("00000000-0000-0000-0000-000000000212"),
			"openai",
			"",
			false,
			true,
		)
		unboundModel := modelConfig(
			uuid.MustParse("00000000-0000-0000-0000-000000002002"),
			"openai",
			nil,
		)

		providers := runtimeProvidersForVisibleModels(
			[]database.ChatModelConfig{unboundModel},
			[]database.ChatProvider{defaultProvider, secondaryProvider},
		)

		require.Equal(t, []database.ChatProvider{defaultProvider}, providers)
	})
}
