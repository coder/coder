package chatprovider_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	fantasygoogle "charm.land/fantasy/providers/google"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
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
	bedrockProviderID := uuid.MustParse("00000000-0000-0000-0000-000000000003")

	tests := []struct {
		name             string
		fallback         chatprovider.ProviderAPIKeys
		providers        []chatprovider.ConfiguredProvider
		userKeys         []chatprovider.UserProviderKey
		wantAvailability map[string]chatprovider.ProviderAvailability
		wantKeys         map[string]string
		wantKeyPresence  map[string]bool
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
			wantKeyPresence: map[string]bool{
				fantasyopenai.Name: false,
			},
		},
		{
			name:      "BedrockCentralOnlyAmbientCredentialsEnabled",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(bedrockProviderID, fantasybedrock.Name, true, "", false, false)},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasybedrock.Name: {Available: true},
			},
			wantKeys: map[string]string{
				fantasybedrock.Name: "",
			},
			wantKeyPresence: map[string]bool{
				fantasybedrock.Name: true,
			},
		},
		{
			name:      "BedrockFallbackAmbientCredentialsEnabled",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(bedrockProviderID, fantasybedrock.Name, true, "", true, true)},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasybedrock.Name: {Available: true},
			},
			wantKeys: map[string]string{
				fantasybedrock.Name: "",
			},
			wantKeyPresence: map[string]bool{
				fantasybedrock.Name: true,
			},
		},
		{
			name:      "BedrockUserKeyRequiredWithoutFallback",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(bedrockProviderID, fantasybedrock.Name, true, "", true, false)},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasybedrock.Name: {Available: false, UnavailableReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired},
			},
			wantKeys: map[string]string{
				fantasybedrock.Name: "",
			},
			wantKeyPresence: map[string]bool{
				fantasybedrock.Name: false,
			},
		},
		{
			name:      "BedrockCentralDisabledMissingAPIKey",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(bedrockProviderID, fantasybedrock.Name, false, "", false, false)},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasybedrock.Name: {Available: false, UnavailableReason: codersdk.ChatModelProviderUnavailableMissingAPIKey},
			},
			wantKeys: map[string]string{
				fantasybedrock.Name: "",
			},
			wantKeyPresence: map[string]bool{
				fantasybedrock.Name: false,
			},
		},
		{
			name:      "BedrockCentralStoredKeyPresent",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(bedrockProviderID, fantasybedrock.Name, true, "bedrock-token", false, false)},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasybedrock.Name: {Available: true},
			},
			wantKeys: map[string]string{
				fantasybedrock.Name: "bedrock-token",
			},
			wantKeyPresence: map[string]bool{
				fantasybedrock.Name: true,
			},
		},
		{
			name: "AnthropicCentralOnlyAmbientCredentialsEnabled",
			providers: []chatprovider.ConfiguredProvider{{
				ProviderID:                 anthropicProviderID,
				Provider:                   fantasyanthropic.Name,
				CentralAPIKeyEnabled:       true,
				SupportsAmbientCredentials: true,
			}},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasyanthropic.Name: {Available: true},
			},
			wantKeys: map[string]string{
				fantasyanthropic.Name: "",
			},
			wantKeyPresence: map[string]bool{
				fantasyanthropic.Name: true,
			},
		},
		{
			name: "AnthropicAmbientCredentialsUserKeyRequiredWithoutFallback",
			providers: []chatprovider.ConfiguredProvider{{
				ProviderID:                 anthropicProviderID,
				Provider:                   fantasyanthropic.Name,
				CentralAPIKeyEnabled:       true,
				AllowUserAPIKey:            true,
				SupportsAmbientCredentials: true,
			}},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasyanthropic.Name: {Available: false, UnavailableReason: codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired},
			},
			wantKeys: map[string]string{
				fantasyanthropic.Name: "",
			},
			wantKeyPresence: map[string]bool{
				fantasyanthropic.Name: false,
			},
		},
		{
			name:      "AnthropicCentralOnlyKeyMissing",
			providers: []chatprovider.ConfiguredProvider{configuredProvider(anthropicProviderID, fantasyanthropic.Name, true, "", false, false)},
			wantAvailability: map[string]chatprovider.ProviderAvailability{
				fantasyanthropic.Name: {Available: false, UnavailableReason: codersdk.ChatModelProviderUnavailableMissingAPIKey},
			},
			wantKeys: map[string]string{
				fantasyanthropic.Name: "",
			},
			wantKeyPresence: map[string]bool{
				fantasyanthropic.Name: false,
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
			for provider, wantPresent := range tt.wantKeyPresence {
				gotKey, ok := keys.ByProvider[provider]
				require.Equal(t, wantPresent, ok, "unexpected key presence for provider %q", provider)
				require.Equal(t, wantPresent, keys.HasProvider(provider), "unexpected HasProvider result for provider %q", provider)
				if wantPresent {
					require.Equal(t, tt.wantKeys[provider], gotKey)
				}
			}
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestAnthropicThinkingDisplayFromChat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input *string
		want  *fantasyanthropic.ThinkingDisplay
	}{
		{
			name:  "Summarized",
			input: new(" SUMMARIZED "),
			want:  new(fantasyanthropic.ThinkingDisplaySummarized),
		},
		{
			name:  "Omitted",
			input: new("omitted"),
			want:  new(fantasyanthropic.ThinkingDisplayOmitted),
		},
		{
			name:  "InvalidReturnsNil",
			input: new("summary"),
		},
		{
			name:  "NilInputReturnsNil",
			input: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := chatprovider.AnthropicThinkingDisplayFromChat(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestProviderOptionsForCall_AnthropicThinkingDisplay(t *testing.T) {
	t.Parallel()

	providerOptions := chatprovider.ProviderOptionsForCall(chatprovider.Model{}, codersdk.ChatModelCallConfig{
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			Anthropic: &codersdk.ChatModelAnthropicProviderOptions{
				ThinkingDisplay: new(" SUMMARIZED "),
			},
		},
	}, nil)

	require.NotNil(t, providerOptions)
	anthropicOptions, ok := providerOptions[fantasyanthropic.Name].(*fantasyanthropic.ProviderOptions)
	require.True(t, ok)
	require.NotNil(t, anthropicOptions.ThinkingDisplay)
	require.Equal(t, fantasyanthropic.ThinkingDisplaySummarized, *anthropicOptions.ThinkingDisplay)
}

func TestGoogleThinkingLevelFromChat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input *string
		want  *fantasygoogle.ThinkingLevel
	}{
		{
			name:  "Minimal",
			input: new(" MINIMAL "),
			want:  new(fantasygoogle.ThinkingLevelMinimal),
		},
		{
			name:  "Low",
			input: new("low"),
			want:  new(fantasygoogle.ThinkingLevelLow),
		},
		{
			name:  "Medium",
			input: new("Medium"),
			want:  new(fantasygoogle.ThinkingLevelMedium),
		},
		{
			name:  "High",
			input: new("high"),
			want:  new(fantasygoogle.ThinkingLevelHigh),
		},
		{
			name:  "InvalidReturnsNil",
			input: new("ultra"),
		},
		{
			name:  "EmptyReturnsNil",
			input: new(" "),
		},
		{
			name:  "NilInputReturnsNil",
			input: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := chatprovider.GoogleThinkingLevelFromChat(tt.input)
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

func TestModelFromConfig_MissingAPIKey(t *testing.T) {
	t.Parallel()

	for provider, wantErr := range map[string]string{
		fantasyopenai.Name:    "OPENAI_API_KEY is not set",
		fantasyanthropic.Name: "ANTHROPIC_API_KEY is not set",
		fantasybedrock.Name:   `API key for provider "bedrock" is not set`,
	} {
		model, err := chatprovider.ModelFromConfig(provider, "test-model", chatprovider.ProviderAPIKeys{}, chatprovider.UserAgent(), nil, nil, nil)
		require.False(t, model.Valid(), provider)
		require.EqualError(t, err, wantErr, provider)
	}
}

// TestModelFromConfig_ExtraHeaders verifies that extra headers passed
// to ModelFromConfig are sent on outgoing LLM API requests. Only the
// OpenAI and Anthropic providers are tested end-to-end because the
// WithHeaders injection is the same mechanical pattern across all
// provider cases, and these are the only two providers with
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

		model, err := chatprovider.ModelFromConfig("openai", "gpt-4", keys, chatprovider.UserAgent(), headers, nil, nil)
		require.NoError(t, err)

		_, err = model.LanguageModel().Generate(ctx, fantasy.Call{
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

		model, err := chatprovider.ModelFromConfig("anthropic", "claude-sonnet-4-20250514", keys, chatprovider.UserAgent(), headers, nil, nil)
		require.NoError(t, err)

		_, err = model.LanguageModel().Generate(ctx, fantasy.Call{
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

func TestBetaHeadersFromCallConfig(t *testing.T) {
	t.Parallel()

	configWith1M := func(enabled *bool) *codersdk.ChatModelCallConfig {
		return &codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				Anthropic: &codersdk.ChatModelAnthropicProviderOptions{
					Context1MEnabled: enabled,
				},
			},
		}
	}
	beta := map[string]string{
		chatprovider.HeaderAnthropicBeta: chatprovider.AnthropicBetaContext1M,
	}

	tests := []struct {
		name     string
		provider string
		config   *codersdk.ChatModelCallConfig
		want     map[string]string
	}{
		{name: "NilConfig", provider: fantasyanthropic.Name, config: nil, want: nil},
		{name: "NilProviderOptions", provider: fantasyanthropic.Name, config: &codersdk.ChatModelCallConfig{}, want: nil},
		{name: "NilAnthropicOptions", provider: fantasyanthropic.Name, config: &codersdk.ChatModelCallConfig{ProviderOptions: &codersdk.ChatModelProviderOptions{}}, want: nil},
		{name: "Unset", provider: fantasyanthropic.Name, config: configWith1M(nil), want: nil},
		{name: "Disabled", provider: fantasyanthropic.Name, config: configWith1M(new(false)), want: nil},
		{name: "EnabledAnthropic", provider: fantasyanthropic.Name, config: configWith1M(new(true)), want: beta},
		{name: "EnabledOpenAI", provider: fantasyopenai.Name, config: configWith1M(new(true)), want: nil},
		{name: "EnabledUnknownProvider", provider: "does-not-exist", config: configWith1M(new(true)), want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chatprovider.BetaHeadersFromCallConfig(tt.provider, tt.config))
		})
	}
}

func generateHello(ctx context.Context, model chatprovider.Model) error {
	_, err := model.LanguageModel().Generate(ctx, fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role:    fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
			},
		},
	})
	return err
}

func TestModelFromConfig_AnthropicBetaExtraHeader(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	called := make(chan struct{})
	serverURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		assert.Equal(t, chatprovider.AnthropicBetaContext1M, req.Header.Get(chatprovider.HeaderAnthropicBeta))
		close(called)
		return chattest.AnthropicNonStreamingResponse("hello")
	})

	keys := chatprovider.ProviderAPIKeys{
		ByProvider:        map[string]string{fantasyanthropic.Name: "test-key"},
		BaseURLByProvider: map[string]string{fantasyanthropic.Name: serverURL},
	}
	betaHeaders := map[string]string{
		chatprovider.HeaderAnthropicBeta: chatprovider.AnthropicBetaContext1M,
	}
	model, err := chatprovider.ModelFromConfig(fantasyanthropic.Name, "claude-sonnet-4-20250514", keys, chatprovider.UserAgent(), betaHeaders, nil, nil)
	require.NoError(t, err)

	require.NoError(t, generateHello(ctx, model))
	_ = testutil.TryReceive(ctx, t, called)
}

// TestModelFromConfig_AnthropicPDFFilePartReachesProvider pins the end-to-end
// path that lets a user-uploaded PDF actually reach Claude/Bedrock: a
// fantasy.FilePart with MediaType "application/pdf" must be serialized as an
// Anthropic "document" content block with a base64 source carrying the PDF
// bytes and a sanitized filename as the document title. Older fantasy versions
// silently dropped PDF FileParts in the Anthropic provider, so the user
// message ended up empty and the model never saw the document. The underlying
// PDF block support came from coder/fantasy#37, a cherry-pick of upstream
// charmbracelet/fantasy#197. The Generate call would fail outright on the
// regressed code path because the dropped FilePart leaves the request with
// zero messages.
func TestModelFromConfig_AnthropicPDFFilePartReachesProvider(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	pdfData := []byte("%PDF-1.7\nfake pdf bytes for regression test")
	wantData := base64.StdEncoding.EncodeToString(pdfData)

	called := make(chan struct{})
	serverURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		defer close(called)

		require.Len(t, req.Messages, 1, "PDF FilePart should produce one Anthropic message, not be dropped as empty")
		require.Equal(t, "user", req.Messages[0].Role)

		var blocks []struct {
			Type   string `json:"type"`
			Title  string `json:"title"`
			Source struct {
				Type      string `json:"type"`
				MediaType string `json:"media_type"`
				Data      string `json:"data"`
			} `json:"source"`
		}
		require.NoError(t, json.Unmarshal(req.Messages[0].Content, &blocks),
			"user content should be a structured block array, got: %s", string(req.Messages[0].Content))

		var found bool
		for _, block := range blocks {
			if block.Type != "document" {
				continue
			}
			assert.Equal(t, "base64", block.Source.Type, "PDF document block must use a base64 source")
			assert.Equal(t, wantData, block.Source.Data, "PDF bytes must round-trip base64 unchanged")
			assert.Equal(t,
				"quarterly report v1 pdf",
				block.Title,
				"PDF filename must reach Anthropic as a sanitized document title",
			)
			if block.Source.MediaType != "" {
				assert.Equal(t, "application/pdf", block.Source.MediaType)
			}
			found = true
		}
		require.True(t, found, "expected an Anthropic document block carrying the PDF, got: %s", string(req.Messages[0].Content))

		return chattest.AnthropicNonStreamingResponse("ok")
	})

	keys := chatprovider.ProviderAPIKeys{
		ByProvider:        map[string]string{"anthropic": "test-key"},
		BaseURLByProvider: map[string]string{"anthropic": serverURL},
	}

	model, err := chatprovider.ModelFromConfig("anthropic", "claude-sonnet-4-20250514", keys, chatprovider.UserAgent(), nil, nil, nil)
	require.NoError(t, err)

	_, err = model.LanguageModel().Generate(ctx, fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.FilePart{
						Filename:  "quarterly_report.v1.pdf",
						Data:      pdfData,
						MediaType: "application/pdf",
					},
				},
			},
		},
	})
	require.NoError(t, err)
	_ = testutil.TryReceive(ctx, t, called)
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

	model, err := chatprovider.ModelFromConfig("openai", "gpt-4", keys, chatprovider.UserAgent(), nil, nil, nil)
	require.NoError(t, err)

	_, err = model.LanguageModel().Generate(ctx, fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role:    fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
			},
		},
	})
	require.NoError(t, err)
	_ = testutil.TryReceive(ctx, t, called)
}

func TestModelFromConfig_HTTPClient(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	called := make(chan struct{})
	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		assert.Equal(t, "true", req.Header.Get("X-Test-Transport"))
		close(called)
		return chattest.OpenAINonStreamingResponse("hello")
	})

	keys := chatprovider.ProviderAPIKeys{
		ByProvider:        map[string]string{"openai": "test-key"},
		BaseURLByProvider: map[string]string{"openai": serverURL},
	}
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		cloned := req.Clone(req.Context())
		cloned.Header = req.Header.Clone()
		cloned.Header.Set("X-Test-Transport", "true")
		return http.DefaultTransport.RoundTrip(cloned)
	})}

	model, err := chatprovider.ModelFromConfig(
		"openai",
		"gpt-4",
		keys,
		chatprovider.UserAgent(),
		nil,
		client,
		nil,
	)
	require.NoError(t, err)

	_, err = model.LanguageModel().Generate(ctx, fantasy.Call{
		Prompt: []fantasy.Message{{
			Role:    fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
		}},
	})
	require.NoError(t, err)
	_ = testutil.TryReceive(ctx, t, called)
}

func TestIsAnthropicFamilyModelID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		modelID string
		want    bool
	}{
		{modelID: "anthropic/claude-haiku-4.5", want: true},
		{modelID: "anthropic/claude-sonnet-4.5:beta", want: true},
		{modelID: "Anthropic/Claude-Opus-4.6", want: true},
		{modelID: "anthropic.claude-3-5-sonnet-20241022-v2:0", want: true},
		{modelID: "us.anthropic.claude-sonnet-4-20250514-v1:0", want: true},
		{modelID: "claude-haiku-4-5", want: true},
		{modelID: " claude-sonnet-4-20250514 ", want: true},
		{modelID: "openai/gpt-5-mini", want: false},
		{modelID: "google/gemini-2.5-flash", want: false},
		{modelID: "deepseek/deepseek-chat-v3.1", want: false},
		{modelID: "gpt-4o", want: false},
		{modelID: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chatprovider.IsAnthropicFamilyModelID(tt.modelID))
		})
	}
}

func TestResolveModelWithProviderHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		modelName    string
		providerHint string
		wantProvider string
		wantModel    string
		wantErr      bool
	}{
		{
			name:         "VercelHintPreservesPrefixedModelID",
			modelName:    "anthropic/claude-4-5-sonnet",
			providerHint: fantasyvercel.Name,
			wantProvider: fantasyvercel.Name,
			wantModel:    "anthropic/claude-4-5-sonnet",
		},
		{
			name:         "OpenRouterHintPreservesPrefixedModelID",
			modelName:    "anthropic/claude-3.5-haiku",
			providerHint: fantasyopenrouter.Name,
			wantProvider: fantasyopenrouter.Name,
			wantModel:    "anthropic/claude-3.5-haiku",
		},
		{
			name:         "OpenAICompatHintPreservesPrefixedModelID",
			modelName:    "anthropic/claude-4-5-sonnet",
			providerHint: fantasyopenaicompat.Name,
			wantProvider: fantasyopenaicompat.Name,
			wantModel:    "anthropic/claude-4-5-sonnet",
		},
		{
			name:         "OpenRouterHintPreservesOpenRouterModelID",
			modelName:    "anthropic/claude-opus-4.6",
			providerHint: fantasyopenrouter.Name,
			wantProvider: fantasyopenrouter.Name,
			wantModel:    "anthropic/claude-opus-4.6",
		},
		{
			name:         "OpenAICompatHintPreservesOpenRouterModelID",
			modelName:    "anthropic/claude-opus-4.6",
			providerHint: fantasyopenaicompat.Name,
			wantProvider: fantasyopenaicompat.Name,
			wantModel:    "anthropic/claude-opus-4.6",
		},
		{
			name:         "OpenAIHintPreservesCanonicalPrefix",
			modelName:    "anthropic/claude-opus-4.6",
			providerHint: fantasyopenai.Name,
			wantProvider: fantasyopenai.Name,
			wantModel:    "anthropic/claude-opus-4.6",
		},
		{
			name:         "OpenAIHintPreservesUnknownSlashNamespace",
			modelName:    "meta-llama/llama-3-70b",
			providerHint: fantasyopenai.Name,
			wantProvider: fantasyopenai.Name,
			wantModel:    "meta-llama/llama-3-70b",
		},
		{
			name:         "AnthropicHintPreservesCanonicalPrefix",
			modelName:    "anthropic/claude-4-5-sonnet",
			providerHint: fantasyanthropic.Name,
			wantProvider: fantasyanthropic.Name,
			wantModel:    "anthropic/claude-4-5-sonnet",
		},
		{
			name:         "AnthropicHintPreservesBedrockNamespace",
			modelName:    "bedrock/claude-opus-4-8",
			providerHint: fantasyanthropic.Name,
			wantProvider: fantasyanthropic.Name,
			wantModel:    "bedrock/claude-opus-4-8",
		},
		{
			name:         "AnthropicHintPreservesDoubleBedrockNamespace",
			modelName:    "bedrock/bedrock/claude-opus-4-8",
			providerHint: fantasyanthropic.Name,
			wantProvider: fantasyanthropic.Name,
			wantModel:    "bedrock/bedrock/claude-opus-4-8",
		},
		{
			name:         "AnthropicHintPreservesColonCanonicalRef",
			modelName:    "openai:gpt-5.2",
			providerHint: fantasyanthropic.Name,
			wantProvider: fantasyanthropic.Name,
			wantModel:    "openai:gpt-5.2",
		},
		{
			name:         "BedrockHintWithBareModel",
			modelName:    "claude-opus-4-6",
			providerHint: fantasybedrock.Name,
			wantProvider: fantasybedrock.Name,
			wantModel:    "claude-opus-4-6",
		},
		{
			name:         "NoHintUsesCanonicalRef",
			modelName:    "anthropic/claude-4-5-sonnet",
			providerHint: "",
			wantProvider: fantasyanthropic.Name,
			wantModel:    "claude-4-5-sonnet",
		},
		{
			name:         "VercelHintWithoutSlashPasses",
			modelName:    "claude-4-5-sonnet",
			providerHint: fantasyvercel.Name,
			wantProvider: fantasyvercel.Name,
			wantModel:    "claude-4-5-sonnet",
		},
		{
			name:         "BareGeminiModelResolvesToGoogle",
			modelName:    "gemini-3.5-flash",
			providerHint: "",
			wantProvider: fantasygoogle.Name,
			wantModel:    "gemini-3.5-flash",
		},
		{
			name:         "BareGemmaModelResolvesToGoogle",
			modelName:    "gemma-3-27b",
			providerHint: "",
			wantProvider: fantasygoogle.Name,
			wantModel:    "gemma-3-27b",
		},
		{
			name:         "GoogleHintWithGeminiModel",
			modelName:    "gemini-2.5-pro",
			providerHint: fantasygoogle.Name,
			wantProvider: fantasygoogle.Name,
			wantModel:    "gemini-2.5-pro",
		},
		{
			name:         "CanonicalGoogleRefResolvesToGoogle",
			modelName:    "google/gemini-3.5-flash",
			providerHint: "",
			wantProvider: fantasygoogle.Name,
			wantModel:    "gemini-3.5-flash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			provider, model, err := chatprovider.ResolveModelWithProviderHint(tt.modelName, tt.providerHint)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantProvider, provider)
			require.Equal(t, tt.wantModel, model)
		})
	}
}

func TestUnsupportedProviders(t *testing.T) {
	t.Parallel()

	t.Run("copilot only", func(t *testing.T) {
		t.Parallel()
		got := chatprovider.UnsupportedProviders([]chatprovider.ConfiguredProvider{
			{Provider: string(codersdk.AIProviderTypeCopilot)},
		})
		require.Equal(t, []codersdk.ChatUnsupportedProvider{
			{
				Provider:    "copilot",
				DisplayName: "GitHub Copilot",
			},
		}, got)
	})

	t.Run("supported provider omitted", func(t *testing.T) {
		t.Parallel()
		got := chatprovider.UnsupportedProviders([]chatprovider.ConfiguredProvider{
			{Provider: fantasyanthropic.Name},
			{Provider: fantasyopenai.Name},
		})
		require.Empty(t, got)
	})

	t.Run("dedup by type and skip supported", func(t *testing.T) {
		t.Parallel()
		got := chatprovider.UnsupportedProviders([]chatprovider.ConfiguredProvider{
			{Provider: fantasyanthropic.Name},
			{Provider: string(codersdk.AIProviderTypeCopilot)},
			{Provider: "Copilot"},
		})
		require.Len(t, got, 1)
		require.Equal(t, "copilot", got[0].Provider)
	})
}
