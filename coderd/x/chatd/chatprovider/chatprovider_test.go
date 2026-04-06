package chatprovider_test

import (
	"net/http"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestResolveUserProviderKeys(t *testing.T) {
	t.Parallel()

	configuredProvider := func(id uuid.UUID, provider string, centralEnabled bool, centralKey string, allowUser bool, allowCentralFallback bool) chatprovider.ConfiguredProvider {
		return chatprovider.ConfiguredProvider{
			ProviderID:                 id,
			Provider:                   provider,
			APIKey:                     centralKey,
			CentralAPIKeyEnabled:       centralEnabled,
			AllowUserAPIKey:            allowUser,
			AllowCentralAPIKeyFallback: allowCentralFallback,
		}
	}

	userProviderKey := func(id uuid.UUID, apiKey string) chatprovider.UserProviderKey {
		return chatprovider.UserProviderKey{
			ChatProviderID: id,
			APIKey:         apiKey,
		}
	}

	openAIProviderID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	anthropicProviderID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	tests := []struct {
		name             string
		fallback         chatprovider.ProviderAPIKeys
		providers        []chatprovider.ConfiguredProvider
		userKeys         []chatprovider.UserProviderKey
		wantAvailability map[string]chatprovider.ProviderAvailability
		wantKeys         map[string]string
	}{
		{
			name:      "CentralOnlyKeyPresent",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(openAIProviderID, fantasyopenai.Name, true, "sk-central", false, false)},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasyopenai.Name: {Available: true},
			},
			wantKeys: map[string]string{
				fantasyopenai.Name: "sk-central",
			},
		},
		{
			name:      "CentralOnlyKeyMissing",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(openAIProviderID, fantasyopenai.Name, true, "", false, false)},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasyopenai.Name: {Available: false, UnavailableReason: codersdk.ChatModelProviderUnavailableMissingAPIKey},
			},
			wantKeys: map[string]string{
				fantasyopenai.Name: "",
			},
		},
		{
			name:      "UserOnlyUserHasKey",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(openAIProviderID, fantasyopenai.Name, false, "sk-central", true, false)},
			userKeys:  []chatprovider.UserProviderKey{userProviderKey(openAIProviderID, "sk-user")},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasyopenai.Name: {Available: true},
			},
			wantKeys: map[string]string{
				fantasyopenai.Name: "sk-user",
			},
		},
		{
			name:      "UserOnlyUserHasNoKey",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(openAIProviderID, fantasyopenai.Name, false, "sk-central", true, false)},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasyopenai.Name: {Available: false, UnavailableReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired},
			},
			wantKeys: map[string]string{
				fantasyopenai.Name: "",
			},
		},
		{
			name:      "BothEnabledFallbackOffUserHasKey",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(openAIProviderID, fantasyopenai.Name, true, "sk-central", true, false)},
			userKeys:  []chatprovider.UserProviderKey{userProviderKey(openAIProviderID, "sk-user")},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasyopenai.Name: {Available: true},
			},
			wantKeys: map[string]string{
				fantasyopenai.Name: "sk-user",
			},
		},
		{
			name:      "BothEnabledFallbackOffUserHasNoKey",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(openAIProviderID, fantasyopenai.Name, true, "sk-central", true, false)},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasyopenai.Name: {Available: false, UnavailableReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired},
			},
			wantKeys: map[string]string{
				fantasyopenai.Name: "",
			},
		},
		{
			name:      "BothEnabledFallbackOnUserHasKey",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(openAIProviderID, fantasyopenai.Name, true, "sk-central", true, true)},
			userKeys:  []chatprovider.UserProviderKey{userProviderKey(openAIProviderID, "sk-user")},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasyopenai.Name: {Available: true},
			},
			wantKeys: map[string]string{
				fantasyopenai.Name: "sk-user",
			},
		},
		{
			name:      "BothEnabledFallbackOnUserHasNoKey",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(openAIProviderID, fantasyopenai.Name, true, "sk-central", true, true)},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasyopenai.Name: {Available: true},
			},
			wantKeys: map[string]string{
				fantasyopenai.Name: "sk-central",
			},
		},
		{
			name:      "BothEnabledFallbackOnCentralKeyEmptyUserHasNoKey",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(openAIProviderID, fantasyopenai.Name, true, "", true, true)},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasyopenai.Name: {Available: false, UnavailableReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired},
			},
			wantKeys: map[string]string{
				fantasyopenai.Name: "",
			},
		},
		{
			name: "MultipleProvidersDifferentPolicies",
			providers: []chatprovider.ConfiguredProvider{
				configuredProvider(openAIProviderID, fantasyopenai.Name, true, "sk-central", false, false),
				configuredProvider(anthropicProviderID, fantasyanthropic.Name, false, "", true, false),
			},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasyopenai.Name:    {Available: true},
				fantasyanthropic.Name: {Available: false, UnavailableReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired},
			},
			wantKeys: map[string]string{
				fantasyopenai.Name:    "sk-central",
				fantasyanthropic.Name: "",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			keys, availability := chatprovider.ResolveUserProviderKeys(tt.fallback, tt.providers, tt.userKeys)

			require.Len(t, availability, len(tt.wantAvailability))
			for provider, wantAvailability := range tt.wantAvailability {
				gotAvailability, ok := availability[provider]
				require.True(t, ok, "expected availability for provider %q", provider)
				require.Equal(t, wantAvailability, gotAvailability)
				require.Equal(t, tt.wantKeys[provider], keys.APIKey(provider))
			}
		})
	}
}

func TestReasoningEffortFromChat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
		input    *string
		want     *string
	}{
		{
			name:     "OpenAICaseInsensitive",
			provider: "openai",
			input:    ptr.Ref(" HIGH "),
			want:     ptr.Ref(string(fantasyopenai.ReasoningEffortHigh)),
		},
		{
			name:     "OpenAIXHighEffort",
			provider: "openai",
			input:    ptr.Ref("xhigh"),
			want:     ptr.Ref(string(fantasyopenai.ReasoningEffortXHigh)),
		},
		{
			name:     "AnthropicEffort",
			provider: "anthropic",
			input:    ptr.Ref("max"),
			want:     ptr.Ref(string(fantasyanthropic.EffortMax)),
		},
		{
			name:     "OpenRouterEffort",
			provider: "openrouter",
			input:    ptr.Ref("medium"),
			want:     ptr.Ref(string(fantasyopenrouter.ReasoningEffortMedium)),
		},
		{
			name:     "VercelEffort",
			provider: "vercel",
			input:    ptr.Ref("xhigh"),
			want:     ptr.Ref(string(fantasyvercel.ReasoningEffortXHigh)),
		},
		{
			name:     "InvalidEffortReturnsNil",
			provider: "openai",
			input:    ptr.Ref("unknown"),
			want:     nil,
		},
		{
			name:     "UnsupportedProviderReturnsNil",
			provider: "bedrock",
			input:    ptr.Ref("high"),
			want:     nil,
		},
		{
			name:     "NilInputReturnsNil",
			provider: "openai",
			input:    nil,
			want:     nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatprovider.ReasoningEffortFromChat(tt.provider, tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestResolveUserProviderKeys_UnavailableReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		provider   chatprovider.ConfiguredProvider
		wantReason codersdk.ChatModelProviderUnavailableReason
	}{
		{
			name: "FallbackConfiguredWithoutCentralKeyReturnsUserAPIKeyRequired",
			provider: chatprovider.ConfiguredProvider{
				Provider:                   "anthropic",
				CentralAPIKeyEnabled:       true,
				AllowUserAPIKey:            true,
				AllowCentralAPIKeyFallback: true,
			},
			wantReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired,
		},
		{
			name: "UserKeyRequiredWithoutFallback",
			provider: chatprovider.ConfiguredProvider{
				Provider:             "anthropic",
				CentralAPIKeyEnabled: true,
				AllowUserAPIKey:      true,
			},
			wantReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			keys, availability := chatprovider.ResolveUserProviderKeys(
				chatprovider.ProviderAPIKeys{},
				[]chatprovider.ConfiguredProvider{tt.provider},
				nil,
			)

			require.Empty(t, keys.APIKey(tt.provider.Provider))
			resolved, ok := availability[tt.provider.Provider]
			require.True(t, ok)
			require.False(t, resolved.Available)
			require.Equal(t, tt.wantReason, resolved.UnavailableReason)
		})
	}
}

func TestListConfiguredModels_PolicyAwareAvailability(t *testing.T) {
	t.Parallel()

	configuredProvider := func(provider string, apiKey string) chatprovider.ConfiguredProvider {
		return chatprovider.ConfiguredProvider{
			ProviderID: uuid.New(),
			Provider:   provider,
			APIKey:     apiKey,
		}
	}
	enabledProviders := func(providers ...string) map[string]struct{} {
		result := make(map[string]struct{}, len(providers))
		for _, provider := range providers {
			result[chatprovider.NormalizeProvider(provider)] = struct{}{}
		}
		return result
	}

	catalog := chatprovider.NewModelCatalog()
	tests := []struct {
		name                   string
		configuredProviders    []chatprovider.ConfiguredProvider
		configuredModels       []chatprovider.ConfiguredModel
		availabilityByProvider map[string]chatprovider.ProviderAvailability
		enabledProviders       map[string]struct{}
		want                   codersdk.ChatModelsResponse
	}{
		{
			name: "PolicyUnavailableOverridesConfiguredKey",
			configuredProviders: []chatprovider.ConfiguredProvider{
				configuredProvider(fantasyopenai.Name, "sk-central"),
			},
			configuredModels: []chatprovider.ConfiguredModel{{
				Provider: fantasyopenai.Name,
				Model:    "gpt-4",
			}},
			availabilityByProvider: map[string]chatprovider.ProviderAvailability{
				fantasyopenai.Name: {
					Available:         false,
					UnavailableReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired,
				},
			},
			enabledProviders: enabledProviders(fantasyopenai.Name),
			want: codersdk.ChatModelsResponse{Providers: []codersdk.ChatModelProvider{{
				Provider:          fantasyopenai.Name,
				Available:         false,
				UnavailableReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired,
				Models: []codersdk.ChatModel{{
					ID:          fantasyopenai.Name + ":gpt-4",
					Provider:    fantasyopenai.Name,
					Model:       "gpt-4",
					DisplayName: "gpt-4",
				}},
			}}},
		},
		{
			name: "PolicyAvailableMarksProviderAvailable",
			configuredProviders: []chatprovider.ConfiguredProvider{
				configuredProvider(fantasyanthropic.Name, "sk-central"),
			},
			configuredModels: []chatprovider.ConfiguredModel{{
				Provider: fantasyanthropic.Name,
				Model:    "claude-3-5-sonnet",
			}},
			availabilityByProvider: map[string]chatprovider.ProviderAvailability{
				fantasyanthropic.Name: {Available: true},
			},
			enabledProviders: enabledProviders(fantasyanthropic.Name),
			want: codersdk.ChatModelsResponse{Providers: []codersdk.ChatModelProvider{{
				Provider:  fantasyanthropic.Name,
				Available: true,
				Models: []codersdk.ChatModel{{
					ID:          fantasyanthropic.Name + ":claude-3-5-sonnet",
					Provider:    fantasyanthropic.Name,
					Model:       "claude-3-5-sonnet",
					DisplayName: "claude-3-5-sonnet",
				}},
			}}},
		},
		{
			name: "DisabledProviderOmitted",
			configuredProviders: []chatprovider.ConfiguredProvider{
				configuredProvider(fantasyanthropic.Name, "sk-anthropic"),
				configuredProvider(fantasyopenai.Name, "sk-openai"),
			},
			configuredModels: []chatprovider.ConfiguredModel{
				{Provider: fantasyanthropic.Name, Model: "claude-3-5-sonnet"},
				{Provider: fantasyopenai.Name, Model: "gpt-4"},
			},
			availabilityByProvider: map[string]chatprovider.ProviderAvailability{
				fantasyanthropic.Name: {Available: true},
				fantasyopenai.Name:    {Available: true},
			},
			enabledProviders: enabledProviders(fantasyopenai.Name),
			want: codersdk.ChatModelsResponse{Providers: []codersdk.ChatModelProvider{{
				Provider:  fantasyopenai.Name,
				Available: true,
				Models: []codersdk.ChatModel{{
					ID:          fantasyopenai.Name + ":gpt-4",
					Provider:    fantasyopenai.Name,
					Model:       "gpt-4",
					DisplayName: "gpt-4",
				}},
			}}},
		},
		{
			name: "MissingAvailabilityDefaultsToMissingAPIKey",
			configuredProviders: []chatprovider.ConfiguredProvider{
				configuredProvider(fantasyopenai.Name, "sk-central"),
			},
			configuredModels: []chatprovider.ConfiguredModel{{
				Provider: fantasyopenai.Name,
				Model:    "gpt-4o",
			}},
			enabledProviders: enabledProviders(fantasyopenai.Name),
			want: codersdk.ChatModelsResponse{Providers: []codersdk.ChatModelProvider{{
				Provider:          fantasyopenai.Name,
				Available:         false,
				UnavailableReason: codersdk.ChatModelProviderUnavailableMissingAPIKey,
				Models: []codersdk.ChatModel{{
					ID:          fantasyopenai.Name + ":gpt-4o",
					Provider:    fantasyopenai.Name,
					Model:       "gpt-4o",
					DisplayName: "gpt-4o",
				}},
			}}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := catalog.ListConfiguredModels(
				tt.configuredProviders,
				tt.configuredModels,
				tt.availabilityByProvider,
				tt.enabledProviders,
			)
			require.True(t, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestListConfiguredProviderAvailability_PolicyAwareFiltering(t *testing.T) {
	t.Parallel()

	enabledProviders := func(providers ...string) map[string]struct{} {
		result := make(map[string]struct{}, len(providers))
		for _, provider := range providers {
			result[chatprovider.NormalizeProvider(provider)] = struct{}{}
		}
		return result
	}

	catalog := chatprovider.NewModelCatalog()
	tests := []struct {
		name                   string
		availabilityByProvider map[string]chatprovider.ProviderAvailability
		enabledProviders       map[string]struct{}
		want                   codersdk.ChatModelsResponse
	}{
		{
			name: "EnabledProvidersUsePolicyAvailability",
			availabilityByProvider: map[string]chatprovider.ProviderAvailability{
				fantasyanthropic.Name: {
					Available:         false,
					UnavailableReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired,
				},
				fantasyopenai.Name: {Available: true},
			},
			enabledProviders: enabledProviders(fantasyanthropic.Name, fantasyopenai.Name),
			want: codersdk.ChatModelsResponse{Providers: []codersdk.ChatModelProvider{
				{
					Provider:          fantasyanthropic.Name,
					Available:         false,
					UnavailableReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired,
					Models:            []codersdk.ChatModel{},
				},
				{
					Provider:  fantasyopenai.Name,
					Available: true,
					Models:    []codersdk.ChatModel{},
				},
			}},
		},
		{
			name: "DisabledSupportedProviderOmitted",
			availabilityByProvider: map[string]chatprovider.ProviderAvailability{
				fantasyanthropic.Name: {Available: true},
				fantasyopenai.Name:    {Available: true},
			},
			enabledProviders: enabledProviders(fantasyopenai.Name),
			want: codersdk.ChatModelsResponse{Providers: []codersdk.ChatModelProvider{{
				Provider:  fantasyopenai.Name,
				Available: true,
				Models:    []codersdk.ChatModel{},
			}}},
		},
		{
			name:             "MissingAvailabilityDefaultsToMissingAPIKey",
			enabledProviders: enabledProviders(fantasyopenai.Name),
			want: codersdk.ChatModelsResponse{Providers: []codersdk.ChatModelProvider{{
				Provider:          fantasyopenai.Name,
				Available:         false,
				UnavailableReason: codersdk.ChatModelProviderUnavailableMissingAPIKey,
				Models:            []codersdk.ChatModel{},
			}}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := catalog.ListConfiguredProviderAvailability(
				tt.availabilityByProvider,
				tt.enabledProviders,
			)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestPruneDisabledProviderKeys(t *testing.T) {
	t.Parallel()

	enabledProviders := func(providers ...string) map[string]struct{} {
		result := make(map[string]struct{}, len(providers))
		for _, provider := range providers {
			result[chatprovider.NormalizeProvider(provider)] = struct{}{}
		}
		return result
	}

	tests := []struct {
		name             string
		keys             chatprovider.ProviderAPIKeys
		enabledProviders map[string]struct{}
		want             chatprovider.ProviderAPIKeys
	}{
		{
			name: "DisabledProviderEntriesRemoved",
			keys: chatprovider.ProviderAPIKeys{
				ByProvider: map[string]string{
					fantasyanthropic.Name: "sk-anthropic",
					fantasyopenai.Name:    "sk-openai",
				},
				BaseURLByProvider: map[string]string{
					fantasyanthropic.Name: "https://anthropic.example.com",
					fantasyopenai.Name:    "https://openai.example.com",
				},
			},
			enabledProviders: enabledProviders(fantasyopenai.Name),
			want: chatprovider.ProviderAPIKeys{
				ByProvider: map[string]string{
					fantasyopenai.Name: "sk-openai",
				},
				BaseURLByProvider: map[string]string{
					fantasyopenai.Name: "https://openai.example.com",
				},
			},
		},
		{
			name: "OpenAIDisabledClearsLegacyField",
			keys: chatprovider.ProviderAPIKeys{
				OpenAI:    "sk-openai",
				Anthropic: "sk-anthropic",
				ByProvider: map[string]string{
					fantasyopenai.Name:    "sk-openai",
					fantasyanthropic.Name: "sk-anthropic",
				},
				BaseURLByProvider: map[string]string{
					fantasyopenai.Name:    "https://openai.example.com",
					fantasyanthropic.Name: "https://anthropic.example.com",
				},
			},
			enabledProviders: enabledProviders(fantasyanthropic.Name),
			want: chatprovider.ProviderAPIKeys{
				Anthropic: "sk-anthropic",
				ByProvider: map[string]string{
					fantasyanthropic.Name: "sk-anthropic",
				},
				BaseURLByProvider: map[string]string{
					fantasyanthropic.Name: "https://anthropic.example.com",
				},
			},
		},
		{
			name: "AnthropicDisabledClearsLegacyField",
			keys: chatprovider.ProviderAPIKeys{
				OpenAI:    "sk-openai",
				Anthropic: "sk-anthropic",
				ByProvider: map[string]string{
					fantasyopenai.Name:    "sk-openai",
					fantasyanthropic.Name: "sk-anthropic",
				},
				BaseURLByProvider: map[string]string{
					fantasyopenai.Name:    "https://openai.example.com",
					fantasyanthropic.Name: "https://anthropic.example.com",
				},
			},
			enabledProviders: enabledProviders(fantasyopenai.Name),
			want: chatprovider.ProviderAPIKeys{
				OpenAI: "sk-openai",
				ByProvider: map[string]string{
					fantasyopenai.Name: "sk-openai",
				},
				BaseURLByProvider: map[string]string{
					fantasyopenai.Name: "https://openai.example.com",
				},
			},
		},
		{
			name: "AllEnabledLeavesKeysUnchanged",
			keys: chatprovider.ProviderAPIKeys{
				OpenAI:    "sk-openai",
				Anthropic: "sk-anthropic",
				ByProvider: map[string]string{
					fantasyopenai.Name:    "sk-openai",
					fantasyanthropic.Name: "sk-anthropic",
				},
				BaseURLByProvider: map[string]string{
					fantasyopenai.Name:    "https://openai.example.com",
					fantasyanthropic.Name: "https://anthropic.example.com",
				},
			},
			enabledProviders: enabledProviders(fantasyopenai.Name, fantasyanthropic.Name),
			want: chatprovider.ProviderAPIKeys{
				OpenAI:    "sk-openai",
				Anthropic: "sk-anthropic",
				ByProvider: map[string]string{
					fantasyopenai.Name:    "sk-openai",
					fantasyanthropic.Name: "sk-anthropic",
				},
				BaseURLByProvider: map[string]string{
					fantasyopenai.Name:    "https://openai.example.com",
					fantasyanthropic.Name: "https://anthropic.example.com",
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			keys := tt.keys
			chatprovider.PruneDisabledProviderKeys(&keys, tt.enabledProviders)
			require.Equal(t, tt.want, keys)
		})
	}
}

func TestCoderHeaders(t *testing.T) {
	t.Parallel()

	t.Run("RootChatNoWorkspace", func(t *testing.T) {
		t.Parallel()
		chatID := uuid.New()
		ownerID := uuid.New()
		chat := database.Chat{
			ID:      chatID,
			OwnerID: ownerID,
		}
		h := chatprovider.CoderHeaders(chat)
		require.Equal(t, ownerID.String(), h[chatprovider.HeaderCoderOwnerID])
		require.Equal(t, chatID.String(), h[chatprovider.HeaderCoderChatID])
		require.NotContains(t, h, chatprovider.HeaderCoderSubchatID)
		require.NotContains(t, h, chatprovider.HeaderCoderWorkspaceID)
	})

	t.Run("RootChatWithWorkspace", func(t *testing.T) {
		t.Parallel()
		chatID := uuid.New()
		ownerID := uuid.New()
		workspaceID := uuid.New()
		chat := database.Chat{
			ID:          chatID,
			OwnerID:     ownerID,
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
		}
		h := chatprovider.CoderHeaders(chat)
		require.Equal(t, ownerID.String(), h[chatprovider.HeaderCoderOwnerID])
		require.Equal(t, chatID.String(), h[chatprovider.HeaderCoderChatID])
		require.NotContains(t, h, chatprovider.HeaderCoderSubchatID)
		require.Equal(t, workspaceID.String(), h[chatprovider.HeaderCoderWorkspaceID])
	})

	t.Run("SubchatWithWorkspace", func(t *testing.T) {
		t.Parallel()
		parentID := uuid.New()
		subchatID := uuid.New()
		ownerID := uuid.New()
		workspaceID := uuid.New()
		chat := database.Chat{
			ID:           subchatID,
			OwnerID:      ownerID,
			ParentChatID: uuid.NullUUID{UUID: parentID, Valid: true},
			WorkspaceID:  uuid.NullUUID{UUID: workspaceID, Valid: true},
		}
		h := chatprovider.CoderHeaders(chat)
		require.Equal(t, ownerID.String(), h[chatprovider.HeaderCoderOwnerID])
		require.Equal(t, parentID.String(), h[chatprovider.HeaderCoderChatID])
		require.Equal(t, subchatID.String(), h[chatprovider.HeaderCoderSubchatID])
		require.Equal(t, workspaceID.String(), h[chatprovider.HeaderCoderWorkspaceID])
	})

	t.Run("SubchatNoWorkspace", func(t *testing.T) {
		t.Parallel()
		parentID := uuid.New()
		subchatID := uuid.New()
		ownerID := uuid.New()
		chat := database.Chat{
			ID:           subchatID,
			OwnerID:      ownerID,
			ParentChatID: uuid.NullUUID{UUID: parentID, Valid: true},
		}
		h := chatprovider.CoderHeaders(chat)
		require.Equal(t, ownerID.String(), h[chatprovider.HeaderCoderOwnerID])
		require.Equal(t, parentID.String(), h[chatprovider.HeaderCoderChatID])
		require.Equal(t, subchatID.String(), h[chatprovider.HeaderCoderSubchatID])
		require.NotContains(t, h, chatprovider.HeaderCoderWorkspaceID)
	})
}

// TestModelFromConfig_ExtraHeaders verifies that extra headers passed
// to ModelFromConfig are sent on outgoing LLM API requests. Only the
// OpenAI and Anthropic providers are tested end-to-end because the
// WithHeaders injection is the same mechanical pattern across all
// eight provider cases, and these are the only two providers with
// chattest test servers. CoderHeaders construction is tested
// separately in TestCoderHeaders.
func TestModelFromConfig_ExtraHeaders(t *testing.T) {
	t.Parallel()

	parentID := uuid.New()
	subchatID := uuid.New()
	ownerID := uuid.New()
	workspaceID := uuid.New()

	chat := database.Chat{
		ID:           subchatID,
		OwnerID:      ownerID,
		ParentChatID: uuid.NullUUID{UUID: parentID, Valid: true},
		WorkspaceID:  uuid.NullUUID{UUID: workspaceID, Valid: true},
	}
	headers := chatprovider.CoderHeaders(chat)

	assertCoderHeaders := func(t *testing.T, got http.Header) {
		t.Helper()
		assert.Equal(t, ownerID.String(), got.Get(chatprovider.HeaderCoderOwnerID))
		assert.Equal(t, parentID.String(), got.Get(chatprovider.HeaderCoderChatID))
		assert.Equal(t, subchatID.String(), got.Get(chatprovider.HeaderCoderSubchatID))
		assert.Equal(t, workspaceID.String(), got.Get(chatprovider.HeaderCoderWorkspaceID))
	}

	t.Run("OpenAI", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		called := make(chan struct{})
		serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			assertCoderHeaders(t, req.Header)
			close(called)
			return chattest.OpenAINonStreamingResponse("hello")
		})

		keys := chatprovider.ProviderAPIKeys{
			ByProvider:        map[string]string{"openai": "test-key"},
			BaseURLByProvider: map[string]string{"openai": serverURL},
		}

		model, err := chatprovider.ModelFromConfig("openai", "gpt-4", keys, chatprovider.UserAgent(), headers)
		require.NoError(t, err)

		_, err = model.Generate(ctx, fantasy.Call{
			Prompt: []fantasy.Message{
				{
					Role:    fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
				},
			},
		})
		require.NoError(t, err)
		_ = testutil.TryReceive(ctx, t, called)
	})

	t.Run("Anthropic", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		called := make(chan struct{})
		serverURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			assertCoderHeaders(t, req.Header)
			close(called)
			return chattest.AnthropicNonStreamingResponse("hello")
		})

		keys := chatprovider.ProviderAPIKeys{
			ByProvider:        map[string]string{"anthropic": "test-key"},
			BaseURLByProvider: map[string]string{"anthropic": serverURL},
		}

		model, err := chatprovider.ModelFromConfig("anthropic", "claude-sonnet-4-20250514", keys, chatprovider.UserAgent(), headers)
		require.NoError(t, err)

		_, err = model.Generate(ctx, fantasy.Call{
			Prompt: []fantasy.Message{
				{
					Role:    fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
				},
			},
		})
		require.NoError(t, err)
		_ = testutil.TryReceive(ctx, t, called)
	})
}

func TestModelFromConfig_NilExtraHeaders(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	called := make(chan struct{})
	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		// Coder headers must be absent when nil is passed.
		assert.Empty(t, req.Header.Get(chatprovider.HeaderCoderOwnerID))
		assert.Empty(t, req.Header.Get(chatprovider.HeaderCoderChatID))
		assert.Empty(t, req.Header.Get(chatprovider.HeaderCoderSubchatID))
		assert.Empty(t, req.Header.Get(chatprovider.HeaderCoderWorkspaceID))
		close(called)
		return chattest.OpenAINonStreamingResponse("hello")
	})

	keys := chatprovider.ProviderAPIKeys{
		ByProvider:        map[string]string{"openai": "test-key"},
		BaseURLByProvider: map[string]string{"openai": serverURL},
	}

	model, err := chatprovider.ModelFromConfig("openai", "gpt-4", keys, chatprovider.UserAgent(), nil)
	require.NoError(t, err)

	_, err = model.Generate(ctx, fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role:    fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
			},
		},
	})
	require.NoError(t, err)
	_ = testutil.TryReceive(ctx, t, called)
func TestSupportedProviders(t *testing.T) {
	t.Parallel()

	providers := chatprovider.SupportedProviders()

	// Must contain all known providers.
	require.Contains(t, providers, "openai")
	require.Contains(t, providers, "anthropic")
	require.Contains(t, providers, "azure")
	require.Contains(t, providers, "bedrock")
	require.Contains(t, providers, "google")
	require.Contains(t, providers, "openai-compat")
	require.Contains(t, providers, "openrouter")
	require.Contains(t, providers, "vercel")
	require.Len(t, providers, 8)

	// Returns a copy, not the original slice.
	providers[0] = "mutated"
	fresh := chatprovider.SupportedProviders()
	require.NotEqual(t, "mutated", fresh[0],
		"SupportedProviders must return a defensive copy")
}

func TestIsEnvPresetProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "OpenAI", input: "openai", expected: true},
		{name: "Anthropic", input: "anthropic", expected: true},
		{name: "OpenAIUpperCase", input: "OPENAI", expected: true},
		{name: "AnthropicMixedCase", input: "Anthropic", expected: true},
		{name: "OpenAIWithSpaces", input: "  openai  ", expected: true},
		{name: "Google", input: "google", expected: false},
		{name: "OpenRouter", input: "openrouter", expected: false},
		{name: "Azure", input: "azure", expected: false},
		{name: "Unknown", input: "unknown", expected: false},
		{name: "Empty", input: "", expected: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := chatprovider.IsEnvPresetProvider(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestProviderDisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "OpenAI", input: "openai", expected: "OpenAI"},
		{name: "Anthropic", input: "anthropic", expected: "Anthropic"},
		{name: "Azure", input: "azure", expected: "Azure OpenAI"},
		{name: "Bedrock", input: "bedrock", expected: "AWS Bedrock"},
		{name: "Google", input: "google", expected: "Google"},
		{name: "OpenAICompat", input: "openai-compat", expected: "OpenAI Compatible"},
		{name: "OpenRouter", input: "openrouter", expected: "OpenRouter"},
		{name: "Vercel", input: "vercel", expected: "Vercel AI Gateway"},
		{name: "UpperCase", input: "OPENAI", expected: "OpenAI"},
		{name: "UnknownReturnsEmpty", input: "unknown", expected: ""},
		{name: "EmptyReturnsEmpty", input: "", expected: ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := chatprovider.ProviderDisplayName(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestNormalizeProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "OpenAI", input: "openai", expected: "openai"},
		{name: "Anthropic", input: "anthropic", expected: "anthropic"},
		{name: "Azure", input: "azure", expected: "azure"},
		{name: "Bedrock", input: "bedrock", expected: "bedrock"},
		{name: "Google", input: "google", expected: "google"},
		{name: "OpenAICompat", input: "openai-compat", expected: "openai-compat"},
		{name: "OpenRouter", input: "openrouter", expected: "openrouter"},
		{name: "Vercel", input: "vercel", expected: "vercel"},
		{name: "UpperCase", input: "OPENAI", expected: "openai"},
		{name: "MixedCase", input: "OpenRouter", expected: "openrouter"},
		{name: "LeadingTrailingSpaces", input: "  anthropic  ", expected: "anthropic"},
		{name: "UnknownReturnsEmpty", input: "unknown", expected: ""},
		{name: "EmptyReturnsEmpty", input: "", expected: ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := chatprovider.NormalizeProvider(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestResolveModelWithProviderHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		model        string
		providerHint string
		wantProvider string
		wantModel    string
		wantErr      bool
	}{
		// Canonical provider:model references.
		{
			name:         "CanonicalColonSeparator",
			model:        "openai:gpt-4o",
			providerHint: "",
			wantProvider: "openai",
			wantModel:    "gpt-4o",
		},
		{
			name:         "CanonicalSlashSeparator",
			model:        "anthropic/claude-sonnet-4-20250514",
			providerHint: "",
			wantProvider: "anthropic",
			wantModel:    "claude-sonnet-4-20250514",
		},
		{
			name:         "CanonicalOverridesHint",
			model:        "google:gemini-2.5-flash",
			providerHint: "openai",
			wantProvider: "google",
			wantModel:    "gemini-2.5-flash",
		},
		// Provider hint used when no canonical prefix.
		{
			name:         "HintUsedForBareModel",
			model:        "my-custom-model",
			providerHint: "openai-compat",
			wantProvider: "openai-compat",
			wantModel:    "my-custom-model",
		},
		// Well-known model shortcuts.
		{
			name:         "WellKnownClaudeOpus",
			model:        "claude-opus-4-6",
			providerHint: "",
			wantProvider: "anthropic",
			wantModel:    "claude-opus-4-6",
		},
		{
			name:         "WellKnownGPT",
			model:        "gpt-5.2",
			providerHint: "",
			wantProvider: "openai",
			wantModel:    "gpt-5.2",
		},
		{
			name:         "WellKnownGemini",
			model:        "gemini-2.5-flash",
			providerHint: "",
			wantProvider: "google",
			wantModel:    "gemini-2.5-flash",
		},
		// Prefix-based inference.
		{
			name:         "ClaudePrefixInfersAnthropic",
			model:        "claude-sonnet-4-20250514",
			providerHint: "",
			wantProvider: "anthropic",
			wantModel:    "claude-sonnet-4-20250514",
		},
		{
			name:         "GPTPrefixInfersOpenAI",
			model:        "gpt-4o-mini",
			providerHint: "",
			wantProvider: "openai",
			wantModel:    "gpt-4o-mini",
		},
		// Error cases.
		{
			name:    "EmptyModelErrors",
			model:   "",
			wantErr: true,
		},
		{
			name:    "WhitespaceOnlyModelErrors",
			model:   "   ",
			wantErr: true,
		},
		{
			name:    "UnknownModelNoHintErrors",
			model:   "totally-unknown-model",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			provider, model, err := chatprovider.ResolveModelWithProviderHint(tt.model, tt.providerHint)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantProvider, provider)
			assert.Equal(t, tt.wantModel, model)
		})
	}
}

func TestOpenAITextVerbosityFromChat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input *string
		want  *fantasyopenai.TextVerbosity
	}{
		{name: "NilReturnsNil", input: nil, want: nil},
		{name: "EmptyReturnsNil", input: ptr.Ref(""), want: nil},
		{name: "WhitespaceReturnsNil", input: ptr.Ref("  "), want: nil},
		{name: "InvalidReturnsNil", input: ptr.Ref("extreme"), want: nil},
		{name: "Low", input: ptr.Ref("low"), want: ptr.Ref(fantasyopenai.TextVerbosityLow)},
		{name: "Medium", input: ptr.Ref("medium"), want: ptr.Ref(fantasyopenai.TextVerbosityMedium)},
		{name: "High", input: ptr.Ref("high"), want: ptr.Ref(fantasyopenai.TextVerbosityHigh)},
		{name: "CaseInsensitive", input: ptr.Ref("HIGH"), want: ptr.Ref(fantasyopenai.TextVerbosityHigh)},
		{name: "TrimmedAndLowered", input: ptr.Ref("  Medium  "), want: ptr.Ref(fantasyopenai.TextVerbosityMedium)},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := chatprovider.OpenAITextVerbosityFromChat(tt.input)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, *tt.want, *got)
			}
		})
	}
}

func TestIsResponsesStoreEnabled(t *testing.T) {
	t.Parallel()

	storeTrue := true
	storeFalse := false

	tests := []struct {
		name     string
		opts     fantasy.ProviderOptions
		expected bool
	}{
		{name: "NilOptions", opts: nil, expected: false},
		{name: "EmptyOptions", opts: fantasy.ProviderOptions{}, expected: false},
		{name: "WrongProviderKey", opts: fantasy.ProviderOptions{"anthropic": &fantasyopenai.ResponsesProviderOptions{Store: &storeTrue}}, expected: false},
		{name: "WrongType", opts: fantasy.ProviderOptions{"openai": &fantasyopenai.ProviderOptions{}}, expected: false},
		{name: "NilStore", opts: fantasy.ProviderOptions{"openai": &fantasyopenai.ResponsesProviderOptions{}}, expected: false},
		{name: "StoreFalse", opts: fantasy.ProviderOptions{"openai": &fantasyopenai.ResponsesProviderOptions{Store: &storeFalse}}, expected: false},
		{name: "StoreTrue", opts: fantasy.ProviderOptions{"openai": &fantasyopenai.ResponsesProviderOptions{Store: &storeTrue}}, expected: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := chatprovider.IsResponsesStoreEnabled(tt.opts)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestCloneWithPreviousResponseID(t *testing.T) {
	t.Parallel()

	t.Run("SetsResponseIDOnClone", func(t *testing.T) {
		t.Parallel()

		storeTrue := true
		originalOpts := &fantasyopenai.ResponsesProviderOptions{
			Store: &storeTrue,
		}
		opts := fantasy.ProviderOptions{
			"openai":    originalOpts,
			"anthropic": &fantasyanthropic.ProviderOptions{},
		}

		cloned := chatprovider.CloneWithPreviousResponseID(opts, "resp_abc123")

		// Cloned map must have the response ID set.
		respOpts, ok := cloned["openai"].(*fantasyopenai.ResponsesProviderOptions)
		require.True(t, ok)
		require.NotNil(t, respOpts.PreviousResponseID)
		assert.Equal(t, "resp_abc123", *respOpts.PreviousResponseID)

		// Store value must be preserved in clone.
		require.NotNil(t, respOpts.Store)
		assert.True(t, *respOpts.Store)

		// Original must not be mutated.
		assert.Nil(t, originalOpts.PreviousResponseID,
			"original options must not be mutated")

		// Non-openai entries are preserved.
		_, hasAnthropic := cloned["anthropic"]
		assert.True(t, hasAnthropic)
	})

	t.Run("NoOpenAIEntryIsNoop", func(t *testing.T) {
		t.Parallel()

		opts := fantasy.ProviderOptions{
			"anthropic": &fantasyanthropic.ProviderOptions{},
		}

		cloned := chatprovider.CloneWithPreviousResponseID(opts, "resp_xyz")

		// Should still have anthropic, no openai added.
		assert.Len(t, cloned, 1)
		_, hasAnthropic := cloned["anthropic"]
		assert.True(t, hasAnthropic)
	})

	t.Run("NilOptionsReturnsEmptyMap", func(t *testing.T) {
		t.Parallel()

		cloned := chatprovider.CloneWithPreviousResponseID(nil, "resp_nil")
		assert.NotNil(t, cloned)
		assert.Empty(t, cloned)
	})
}

func TestMergeMissingProviderOptions_OpenRouterNested(t *testing.T) {
	t.Parallel()

	options := &codersdk.ChatModelProviderOptions{
		OpenRouter: &codersdk.ChatModelOpenRouterProviderOptions{
			Reasoning: &codersdk.ChatModelReasoningOptions{
				Enabled: ptr.Ref(true),
			},
			Provider: &codersdk.ChatModelOpenRouterProvider{
				Order: []string{"openai"},
			},
		},
	}
	defaults := &codersdk.ChatModelProviderOptions{
		OpenRouter: &codersdk.ChatModelOpenRouterProviderOptions{
			Reasoning: &codersdk.ChatModelReasoningOptions{
				Enabled:   ptr.Ref(false),
				Exclude:   ptr.Ref(true),
				MaxTokens: ptr.Ref[int64](123),
				Effort:    ptr.Ref("high"),
			},
			IncludeUsage: ptr.Ref(true),
			Provider: &codersdk.ChatModelOpenRouterProvider{
				Order:             []string{"anthropic"},
				AllowFallbacks:    ptr.Ref(true),
				RequireParameters: ptr.Ref(false),
				DataCollection:    ptr.Ref("allow"),
				Only:              []string{"openai"},
				Ignore:            []string{"foo"},
				Quantizations:     []string{"int8"},
				Sort:              ptr.Ref("latency"),
			},
		},
	}

	chatprovider.MergeMissingProviderOptions(&options, defaults)

	require.NotNil(t, options)
	require.NotNil(t, options.OpenRouter)
	require.NotNil(t, options.OpenRouter.Reasoning)
	require.True(t, *options.OpenRouter.Reasoning.Enabled)
	require.Equal(t, true, *options.OpenRouter.Reasoning.Exclude)
	require.EqualValues(t, 123, *options.OpenRouter.Reasoning.MaxTokens)
	require.Equal(t, "high", *options.OpenRouter.Reasoning.Effort)
	require.NotNil(t, options.OpenRouter.IncludeUsage)
	require.True(t, *options.OpenRouter.IncludeUsage)

	require.NotNil(t, options.OpenRouter.Provider)
	require.Equal(t, []string{"openai"}, options.OpenRouter.Provider.Order)
	require.NotNil(t, options.OpenRouter.Provider.AllowFallbacks)
	require.True(t, *options.OpenRouter.Provider.AllowFallbacks)
	require.NotNil(t, options.OpenRouter.Provider.RequireParameters)
	require.False(t, *options.OpenRouter.Provider.RequireParameters)
	require.Equal(t, "allow", *options.OpenRouter.Provider.DataCollection)
	require.Equal(t, []string{"openai"}, options.OpenRouter.Provider.Only)
	require.Equal(t, []string{"foo"}, options.OpenRouter.Provider.Ignore)
	require.Equal(t, []string{"int8"}, options.OpenRouter.Provider.Quantizations)
	require.Equal(t, "latency", *options.OpenRouter.Provider.Sort)
}

func TestProviderAPIKeys_APIKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		keys     chatprovider.ProviderAPIKeys
		provider string
		want     string
	}{
		{
			name:     "EmptyProvider",
			keys:     chatprovider.ProviderAPIKeys{},
			provider: "",
			want:     "",
		},
		{
			name:     "UnknownProvider",
			keys:     chatprovider.ProviderAPIKeys{},
			provider: "unknown-provider",
			want:     "",
		},
		{
			name: "FallbackOpenAI",
			keys: chatprovider.ProviderAPIKeys{
				OpenAI: "sk-openai",
			},
			provider: "openai",
			want:     "sk-openai",
		},
		{
			name: "FallbackAnthropic",
			keys: chatprovider.ProviderAPIKeys{
				Anthropic: "sk-anthropic",
			},
			provider: "anthropic",
			want:     "sk-anthropic",
		},
		{
			name: "ByProviderTakesPriority",
			keys: chatprovider.ProviderAPIKeys{
				OpenAI:     "sk-fallback",
				ByProvider: map[string]string{"openai": "sk-override"},
			},
			provider: "openai",
			want:     "sk-override",
		},
		{
			name: "ProviderNormalization",
			keys: chatprovider.ProviderAPIKeys{
				ByProvider: map[string]string{"anthropic": "sk-key"},
			},
			provider: " Anthropic ",
			want:     "sk-key",
		},
		{
			name: "WhitespaceKeyTrimmed",
			keys: chatprovider.ProviderAPIKeys{
				OpenAI: "  sk-padded  ",
			},
			provider: "openai",
			want:     "sk-padded",
		},
		{
			name: "WhitespaceOnlyKeyIsEmpty",
			keys: chatprovider.ProviderAPIKeys{
				ByProvider: map[string]string{"openai": "   "},
				OpenAI:     "sk-fallback",
			},
			provider: "openai",
			want:     "sk-fallback",
		},
		{
			name: "NilByProviderMap",
			keys: chatprovider.ProviderAPIKeys{
				Anthropic: "sk-ant",
			},
			provider: "anthropic",
			want:     "sk-ant",
		},
		{
			name: "GoogleKeyFromByProvider",
			keys: chatprovider.ProviderAPIKeys{
				ByProvider: map[string]string{"google": "goog-key"},
			},
			provider: "google",
			want:     "goog-key",
		},
		{
			name: "GoogleNoFallback",
			keys: chatprovider.ProviderAPIKeys{
				OpenAI:    "sk-openai",
				Anthropic: "sk-anthropic",
			},
			provider: "google",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.keys.APIKey(tt.provider)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestProviderAPIKeys_BaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		keys     chatprovider.ProviderAPIKeys
		provider string
		want     string
	}{
		{
			name:     "EmptyProvider",
			keys:     chatprovider.ProviderAPIKeys{},
			provider: "",
			want:     "",
		},
		{
			name:     "NilMap",
			keys:     chatprovider.ProviderAPIKeys{},
			provider: "openai",
			want:     "",
		},
		{
			name: "ReturnsURL",
			keys: chatprovider.ProviderAPIKeys{
				BaseURLByProvider: map[string]string{
					"openai": "https://custom.openai.example.com",
				},
			},
			provider: "openai",
			want:     "https://custom.openai.example.com",
		},
		{
			name: "TrimsWhitespace",
			keys: chatprovider.ProviderAPIKeys{
				BaseURLByProvider: map[string]string{
					"anthropic": "  https://proxy.example.com  ",
				},
			},
			provider: "anthropic",
			want:     "https://proxy.example.com",
		},
		{
			name: "NormalizesProvider",
			keys: chatprovider.ProviderAPIKeys{
				BaseURLByProvider: map[string]string{
					"google": "https://google.proxy",
				},
			},
			provider: " Google ",
			want:     "https://google.proxy",
		},
		{
			name: "MissingProviderEntry",
			keys: chatprovider.ProviderAPIKeys{
				BaseURLByProvider: map[string]string{
					"openai": "https://openai.proxy",
				},
			},
			provider: "anthropic",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.keys.BaseURL(tt.provider)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestMergeProviderAPIKeys(t *testing.T) {
	t.Parallel()

	t.Run("EmptyInputs", func(t *testing.T) {
		t.Parallel()
		merged := chatprovider.MergeProviderAPIKeys(
			chatprovider.ProviderAPIKeys{},
			nil,
		)
		require.Empty(t, merged.OpenAI)
		require.Empty(t, merged.Anthropic)
		require.NotNil(t, merged.ByProvider)
		require.NotNil(t, merged.BaseURLByProvider)
	})

	t.Run("StaticKeysPreserved", func(t *testing.T) {
		t.Parallel()
		static := chatprovider.ProviderAPIKeys{
			OpenAI:    "sk-openai",
			Anthropic: "sk-anthropic",
			ByProvider: map[string]string{
				"google": "goog-key",
			},
			BaseURLByProvider: map[string]string{
				"google": "https://google.proxy",
			},
		}
		merged := chatprovider.MergeProviderAPIKeys(static, nil)

		require.Equal(t, "sk-openai", merged.OpenAI)
		require.Equal(t, "sk-anthropic", merged.Anthropic)
		// Static OpenAI/Anthropic also get promoted into ByProvider.
		require.Equal(t, "sk-openai", merged.ByProvider["openai"])
		require.Equal(t, "sk-anthropic", merged.ByProvider["anthropic"])
		require.Equal(t, "goog-key", merged.ByProvider["google"])
		require.Equal(t, "https://google.proxy", merged.BaseURLByProvider["google"])
	})

	t.Run("DBProviderOverridesStatic", func(t *testing.T) {
		t.Parallel()
		static := chatprovider.ProviderAPIKeys{
			OpenAI: "sk-old",
		}
		dbProviders := []chatprovider.ConfiguredProvider{
			{Provider: "openai", APIKey: "sk-new", BaseURL: "https://new.openai.com"},
		}
		merged := chatprovider.MergeProviderAPIKeys(static, dbProviders)

		require.Equal(t, "sk-new", merged.OpenAI)
		require.Equal(t, "sk-new", merged.ByProvider["openai"])
		require.Equal(t, "https://new.openai.com", merged.BaseURLByProvider["openai"])
	})

	t.Run("DBAnthropicOverridesStatic", func(t *testing.T) {
		t.Parallel()
		static := chatprovider.ProviderAPIKeys{
			Anthropic: "sk-old-ant",
		}
		dbProviders := []chatprovider.ConfiguredProvider{
			{Provider: "anthropic", APIKey: "sk-new-ant"},
		}
		merged := chatprovider.MergeProviderAPIKeys(static, dbProviders)

		require.Equal(t, "sk-new-ant", merged.Anthropic)
		require.Equal(t, "sk-new-ant", merged.ByProvider["anthropic"])
	})

	t.Run("MixedProviders", func(t *testing.T) {
		t.Parallel()
		static := chatprovider.ProviderAPIKeys{
			OpenAI: "sk-openai",
			ByProvider: map[string]string{
				"google": "goog-static",
			},
		}
		dbProviders := []chatprovider.ConfiguredProvider{
			{Provider: "anthropic", APIKey: "sk-ant-db"},
			{Provider: "google", APIKey: "goog-db", BaseURL: "https://goog.proxy"},
		}
		merged := chatprovider.MergeProviderAPIKeys(static, dbProviders)

		// OpenAI comes from static.
		require.Equal(t, "sk-openai", merged.ByProvider["openai"])
		// DB overrides static for google.
		require.Equal(t, "goog-db", merged.ByProvider["google"])
		require.Equal(t, "https://goog.proxy", merged.BaseURLByProvider["google"])
		// Anthropic from DB.
		require.Equal(t, "sk-ant-db", merged.ByProvider["anthropic"])
		require.Equal(t, "sk-ant-db", merged.Anthropic)
	})

	t.Run("WhitespaceHandling", func(t *testing.T) {
		t.Parallel()
		static := chatprovider.ProviderAPIKeys{
			OpenAI: "  sk-padded  ",
		}
		dbProviders := []chatprovider.ConfiguredProvider{
			{Provider: "  google  ", APIKey: "  goog-key  ", BaseURL: "  https://g.proxy  "},
		}
		merged := chatprovider.MergeProviderAPIKeys(static, dbProviders)

		require.Equal(t, "sk-padded", merged.OpenAI)
		require.Equal(t, "goog-key", merged.ByProvider["google"])
		require.Equal(t, "https://g.proxy", merged.BaseURLByProvider["google"])
	})

	t.Run("InvalidProviderSkipped", func(t *testing.T) {
		t.Parallel()
		static := chatprovider.ProviderAPIKeys{
			ByProvider: map[string]string{
				"not-a-provider": "sk-invalid",
			},
		}
		dbProviders := []chatprovider.ConfiguredProvider{
			{Provider: "also-invalid", APIKey: "sk-bad"},
		}
		merged := chatprovider.MergeProviderAPIKeys(static, dbProviders)

		require.Empty(t, merged.ByProvider)
		require.Empty(t, merged.BaseURLByProvider)
	})
}

func TestModelCatalog_ListConfiguredModels(t *testing.T) {
	t.Parallel()

	t.Run("EmptyModelsReturnsFalse", func(t *testing.T) {
		t.Parallel()
		catalog := chatprovider.NewModelCatalog(chatprovider.ProviderAPIKeys{})
		resp, changed := catalog.ListConfiguredModels(nil, nil)
		require.False(t, changed)
		require.Empty(t, resp.Providers)
	})

	t.Run("SingleModel", func(t *testing.T) {
		t.Parallel()
		keys := chatprovider.ProviderAPIKeys{
			ByProvider: map[string]string{"openai": "sk-test"},
		}
		catalog := chatprovider.NewModelCatalog(keys)
		models := []chatprovider.ConfiguredModel{
			{Provider: "openai", Model: "gpt-4o", DisplayName: "GPT-4o"},
		}
		resp, changed := catalog.ListConfiguredModels(nil, models)

		require.True(t, changed)
		require.Len(t, resp.Providers, 1)
		require.Equal(t, "openai", resp.Providers[0].Provider)
		require.True(t, resp.Providers[0].Available)
		require.Len(t, resp.Providers[0].Models, 1)
		require.Equal(t, "gpt-4o", resp.Providers[0].Models[0].Model)
		require.Equal(t, "GPT-4o", resp.Providers[0].Models[0].DisplayName)
		require.Equal(t, "openai:gpt-4o", resp.Providers[0].Models[0].ID)
	})

	t.Run("DuplicateModelDedup", func(t *testing.T) {
		t.Parallel()
		keys := chatprovider.ProviderAPIKeys{
			ByProvider: map[string]string{"anthropic": "sk-ant"},
		}
		catalog := chatprovider.NewModelCatalog(keys)
		models := []chatprovider.ConfiguredModel{
			{Provider: "anthropic", Model: "claude-sonnet-4-20250514", DisplayName: "Claude Sonnet"},
			{Provider: "anthropic", Model: "claude-sonnet-4-20250514", DisplayName: "Claude Sonnet Dup"},
		}
		resp, changed := catalog.ListConfiguredModels(nil, models)

		require.True(t, changed)
		require.Len(t, resp.Providers, 1)
		// Only one model after dedup.
		require.Len(t, resp.Providers[0].Models, 1)
		require.Equal(t, "Claude Sonnet", resp.Providers[0].Models[0].DisplayName)
	})

	t.Run("ProviderOrdering", func(t *testing.T) {
		t.Parallel()
		keys := chatprovider.ProviderAPIKeys{
			ByProvider: map[string]string{
				"openai":    "sk-openai",
				"anthropic": "sk-ant",
			},
		}
		catalog := chatprovider.NewModelCatalog(keys)
		models := []chatprovider.ConfiguredModel{
			// Anthropic listed first, but openai should appear first
			// in output (supportedProviderNames order: anthropic
			// before openai is wrong, let me check).
			{Provider: "openai", Model: "gpt-4o"},
			{Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
		}
		resp, changed := catalog.ListConfiguredModels(nil, models)

		require.True(t, changed)
		require.Len(t, resp.Providers, 2)
		// supportedProviderNames order: anthropic, azure, bedrock,
		// google, openai, openai-compat, openrouter, vercel.
		require.Equal(t, "anthropic", resp.Providers[0].Provider)
		require.Equal(t, "openai", resp.Providers[1].Provider)
	})

	t.Run("UnavailableWithoutKey", func(t *testing.T) {
		t.Parallel()
		// No API keys at all.
		catalog := chatprovider.NewModelCatalog(chatprovider.ProviderAPIKeys{})
		models := []chatprovider.ConfiguredModel{
			{Provider: "openai", Model: "gpt-4o"},
		}
		resp, changed := catalog.ListConfiguredModels(nil, models)

		require.True(t, changed)
		require.Len(t, resp.Providers, 1)
		require.False(t, resp.Providers[0].Available)
		require.Equal(t,
			codersdk.ChatModelProviderUnavailableMissingAPIKey,
			resp.Providers[0].UnavailableReason,
		)
	})

	t.Run("ModelsWithinProviderSorted", func(t *testing.T) {
		t.Parallel()
		keys := chatprovider.ProviderAPIKeys{
			ByProvider: map[string]string{"openai": "sk-test"},
		}
		catalog := chatprovider.NewModelCatalog(keys)
		models := []chatprovider.ConfiguredModel{
			{Provider: "openai", Model: "gpt-4o"},
			{Provider: "openai", Model: "gpt-3.5-turbo"},
			{Provider: "openai", Model: "gpt-4o-mini"},
		}
		resp, changed := catalog.ListConfiguredModels(nil, models)

		require.True(t, changed)
		require.Len(t, resp.Providers[0].Models, 3)
		require.Equal(t, "gpt-3.5-turbo", resp.Providers[0].Models[0].Model)
		require.Equal(t, "gpt-4o", resp.Providers[0].Models[1].Model)
		require.Equal(t, "gpt-4o-mini", resp.Providers[0].Models[2].Model)
	})

	t.Run("DisplayNameFallsBackToModel", func(t *testing.T) {
		t.Parallel()
		keys := chatprovider.ProviderAPIKeys{
			ByProvider: map[string]string{"openai": "sk-test"},
		}
		catalog := chatprovider.NewModelCatalog(keys)
		models := []chatprovider.ConfiguredModel{
			{Provider: "openai", Model: "gpt-4o", DisplayName: ""},
		}
		resp, changed := catalog.ListConfiguredModels(nil, models)

		require.True(t, changed)
		require.Equal(t, "gpt-4o", resp.Providers[0].Models[0].DisplayName)
	})

	t.Run("ConfiguredProvidersAddedToProviderSet", func(t *testing.T) {
		t.Parallel()
		// Provider has a key via ConfiguredProvider but no models.
		// Models reference the same provider, so it appears.
		keys := chatprovider.ProviderAPIKeys{}
		catalog := chatprovider.NewModelCatalog(keys)
		providers := []chatprovider.ConfiguredProvider{
			{Provider: "openai", APIKey: "sk-test"},
		}
		models := []chatprovider.ConfiguredModel{
			{Provider: "openai", Model: "gpt-4o"},
		}
		resp, changed := catalog.ListConfiguredModels(providers, models)

		require.True(t, changed)
		require.Len(t, resp.Providers, 1)
		require.True(t, resp.Providers[0].Available)
	})

	t.Run("CanonicalModelRef", func(t *testing.T) {
		t.Parallel()
		keys := chatprovider.ProviderAPIKeys{
			ByProvider: map[string]string{"anthropic": "sk-ant"},
		}
		catalog := chatprovider.NewModelCatalog(keys)
		models := []chatprovider.ConfiguredModel{
			// Model with provider:model canonical ref.
			{Model: "anthropic:claude-sonnet-4-20250514"},
		}
		resp, changed := catalog.ListConfiguredModels(nil, models)

		require.True(t, changed)
		require.Len(t, resp.Providers, 1)
		require.Equal(t, "anthropic", resp.Providers[0].Provider)
		require.Equal(t, "claude-sonnet-4-20250514", resp.Providers[0].Models[0].Model)
	})
}

func TestModelCatalog_ListConfiguredProviderAvailability(t *testing.T) {
	t.Parallel()

	t.Run("AllUnavailableWithoutKeys", func(t *testing.T) {
		t.Parallel()
		catalog := chatprovider.NewModelCatalog(chatprovider.ProviderAPIKeys{})
		resp := catalog.ListConfiguredProviderAvailability(nil)

		// All supported providers listed.
		require.Len(t, resp.Providers, len(chatprovider.SupportedProviders()))
		for _, p := range resp.Providers {
			require.False(t, p.Available, "provider %s should be unavailable", p.Provider)
			require.Equal(t,
				codersdk.ChatModelProviderUnavailableMissingAPIKey,
				p.UnavailableReason,
			)
			// Models list is empty but non-nil.
			require.NotNil(t, p.Models)
			require.Empty(t, p.Models)
		}
	})

	t.Run("StaticKeyMakesAvailable", func(t *testing.T) {
		t.Parallel()
		keys := chatprovider.ProviderAPIKeys{
			OpenAI: "sk-openai",
		}
		catalog := chatprovider.NewModelCatalog(keys)
		resp := catalog.ListConfiguredProviderAvailability(nil)

		for _, p := range resp.Providers {
			if p.Provider == "openai" {
				require.True(t, p.Available)
				require.Empty(t, p.UnavailableReason)
			} else {
				require.False(t, p.Available, "provider %s should be unavailable", p.Provider)
			}
		}
	})

	t.Run("DBProviderKeyMakesAvailable", func(t *testing.T) {
		t.Parallel()
		catalog := chatprovider.NewModelCatalog(chatprovider.ProviderAPIKeys{})
		dbProviders := []chatprovider.ConfiguredProvider{
			{Provider: "google", APIKey: "goog-key"},
		}
		resp := catalog.ListConfiguredProviderAvailability(dbProviders)

		for _, p := range resp.Providers {
			if p.Provider == "google" {
				require.True(t, p.Available)
			} else {
				require.False(t, p.Available, "provider %s should be unavailable", p.Provider)
			}
		}
	})

	t.Run("MultipleKeysAvailable", func(t *testing.T) {
		t.Parallel()
		keys := chatprovider.ProviderAPIKeys{
			OpenAI:    "sk-openai",
			Anthropic: "sk-anthropic",
		}
		catalog := chatprovider.NewModelCatalog(keys)
		dbProviders := []chatprovider.ConfiguredProvider{
			{Provider: "google", APIKey: "goog-key"},
		}
		resp := catalog.ListConfiguredProviderAvailability(dbProviders)

		available := map[string]bool{}
		for _, p := range resp.Providers {
			available[p.Provider] = p.Available
		}
		require.True(t, available["openai"])
		require.True(t, available["anthropic"])
		require.True(t, available["google"])
		require.False(t, available["azure"])
		require.False(t, available["bedrock"])
	})
}
