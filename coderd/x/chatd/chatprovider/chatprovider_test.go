package chatprovider_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	fantasygoogle "charm.land/fantasy/providers/google"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream/eventstreamapi"
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

func TestProviderBaseURLHostname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "URL", baseURL: "https://openrouter.ai/api/v1", want: "openrouter.ai"},
		{name: "BareHost", baseURL: "openrouter.ai", want: "openrouter.ai"},
		{name: "HostWithPort", baseURL: "https://openrouter.ai:443/api/v1", want: "openrouter.ai"},
		{name: "Empty", baseURL: "", want: ""},
		{name: "Invalid", baseURL: "://", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chatprovider.ProviderBaseURLHostname(tt.baseURL))
		})
	}
}

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

func TestProviderAPIKeysEmpty_RegionCountsAsConfigured(t *testing.T) {
	t.Parallel()

	require.True(t, (chatprovider.ProviderAPIKeys{}).Empty())
	require.False(t, (chatprovider.ProviderAPIKeys{
		RegionByProvider: map[string]string{
			fantasybedrock.Name: "us-east-1",
		},
	}).Empty())
}

func TestMergeProviderAPIKeys_PreservesProviderRegions(t *testing.T) {
	t.Parallel()

	merged := chatprovider.MergeProviderAPIKeys(
		chatprovider.ProviderAPIKeys{
			RegionByProvider: map[string]string{
				"BEDROCK": "us-east-1",
			},
		},
		[]chatprovider.ConfiguredProvider{{
			ProviderID: uuid.New(),
			Provider:   fantasybedrock.Name,
			Region:     "eu-central-1",
		}},
	)

	require.Equal(t, "eu-central-1", merged.Region(fantasybedrock.Name))
	require.False(t, merged.Empty())
}

func TestResolveUserProviderKeys_PreservesProviderRegions(t *testing.T) {
	t.Parallel()

	keys, availability := chatprovider.ResolveUserProviderKeys(
		chatprovider.ProviderAPIKeys{
			RegionByProvider: map[string]string{
				"BEDROCK": "us-east-1",
			},
		},
		[]chatprovider.ConfiguredProvider{{
			ProviderID:           uuid.New(),
			Provider:             fantasybedrock.Name,
			Region:               "eu-central-1",
			CentralAPIKeyEnabled: true,
		}},
		nil,
	)

	require.Equal(t, "eu-central-1", keys.Region(fantasybedrock.Name))
	require.True(t, keys.HasProvider(fantasybedrock.Name))
	require.Equal(t, chatprovider.ProviderAvailability{Available: true}, availability[fantasybedrock.Name])
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
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
			name:     "AnthropicXHighEffort",
			provider: "anthropic",
			input:    ptr.Ref("xhigh"),
			want:     ptr.Ref(string(fantasyanthropic.EffortXHigh)),
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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatprovider.ReasoningEffortFromChat(tt.provider, tt.input)
			require.Equal(t, tt.want, got)
		})
	}
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
			input: ptr.Ref(" SUMMARIZED "),
			want:  ptr.Ref(fantasyanthropic.ThinkingDisplaySummarized),
		},
		{
			name:  "Omitted",
			input: ptr.Ref("omitted"),
			want:  ptr.Ref(fantasyanthropic.ThinkingDisplayOmitted),
		},
		{
			name:  "InvalidReturnsNil",
			input: ptr.Ref("summary"),
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

func TestProviderOptionsFromChatModelConfig_AnthropicThinkingDisplay(t *testing.T) {
	t.Parallel()

	providerOptions := chatprovider.ProviderOptionsFromChatModelConfig(nil, &codersdk.ChatModelProviderOptions{
		Anthropic: &codersdk.ChatModelAnthropicProviderOptions{
			ThinkingDisplay: ptr.Ref(" SUMMARIZED "),
		},
	})

	require.NotNil(t, providerOptions)
	anthropicOptions, ok := providerOptions[fantasyanthropic.Name].(*fantasyanthropic.ProviderOptions)
	require.True(t, ok)
	require.NotNil(t, anthropicOptions.ThinkingDisplay)
	require.Equal(t, fantasyanthropic.ThinkingDisplaySummarized, *anthropicOptions.ThinkingDisplay)
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
			name: "DisabledProviderRegionsRemoved",
			keys: chatprovider.ProviderAPIKeys{
				RegionByProvider: map[string]string{
					fantasybedrock.Name: "us-east-2",
					fantasyopenai.Name:  "us-east-1",
				},
			},
			enabledProviders: enabledProviders(fantasybedrock.Name),
			want: chatprovider.ProviderAPIKeys{
				RegionByProvider: map[string]string{
					fantasybedrock.Name: "us-east-2",
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

func TestModelFromConfig_Bedrock(t *testing.T) {
	t.Parallel()

	const modelID = "us.anthropic.claude-sonnet-4-20250514-v1:0"

	// This verifies the policy gate that permits an empty Bedrock key.
	// End-to-end ambient credential auth would need a real AWS
	// environment or a more complete mock, which is outside this scope.
	t.Run("AllowsEmptyAPIKeyForAmbientCredentials", func(t *testing.T) {
		t.Parallel()

		model, err := chatprovider.ModelFromConfig(
			fantasybedrock.Name,
			modelID,
			chatprovider.ProviderAPIKeys{
				ByProvider: map[string]string{
					fantasybedrock.Name: "",
				},
			},
			chatprovider.UserAgent(),
			nil,
			nil,
		)
		require.NoError(t, err)
		require.NotNil(t, model)
		require.Equal(t, fantasybedrock.Name, model.Provider())
	})

	t.Run("RequiresResolvedProviderForAmbientCredentials", func(t *testing.T) {
		t.Parallel()

		model, err := chatprovider.ModelFromConfig(
			fantasybedrock.Name,
			modelID,
			chatprovider.ProviderAPIKeys{},
			chatprovider.UserAgent(),
			nil,
			nil,
		)
		require.Nil(t, model)
		require.EqualError(t, err, "API key for provider \"bedrock\" is not set")
	})

	t.Run("ForwardsBaseURLAndExplicitAPIKey", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		type requestCapture struct {
			Path          string
			Authorization string
			UserAgent     string
		}

		requests := make(chan requestCapture, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests <- requestCapture{
				Path:          r.URL.Path,
				Authorization: r.Header.Get("Authorization"),
				UserAgent:     r.Header.Get("User-Agent"),
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(bedrockNonStreamingResponse())
		}))
		defer server.Close()

		model, err := chatprovider.ModelFromConfig(
			fantasybedrock.Name,
			modelID,
			chatprovider.ProviderAPIKeys{
				ByProvider: map[string]string{
					fantasybedrock.Name: "test-key",
				},
				BaseURLByProvider: map[string]string{
					fantasybedrock.Name: server.URL,
				},
			},
			chatprovider.UserAgent(),
			nil,
			nil,
		)
		require.NoError(t, err)
		require.NotNil(t, model)

		_, err = model.Generate(ctx, fantasy.Call{
			Prompt: []fantasy.Message{
				{
					Role: fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{
						fantasy.TextPart{Text: "hello"},
					},
				},
			},
		})
		require.NoError(t, err)

		got := testutil.TryReceive(ctx, t, requests)
		require.Equal(t, "/model/"+modelID+"/invoke", got.Path)
		require.Equal(t, "Bearer test-key", got.Authorization)
		require.Equal(t, chatprovider.UserAgent(), got.UserAgent)
	})

	t.Run("NonBedrockStillRequiresAPIKey", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			provider string
			model    string
			wantErr  string
		}{
			{
				name:     "OpenAI",
				provider: fantasyopenai.Name,
				model:    "gpt-4",
				wantErr:  "OPENAI_API_KEY is not set",
			},
			{
				name:     "Anthropic",
				provider: fantasyanthropic.Name,
				model:    "claude-sonnet-4-20250514",
				wantErr:  "ANTHROPIC_API_KEY is not set",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				model, err := chatprovider.ModelFromConfig(
					tt.provider,
					tt.model,
					chatprovider.ProviderAPIKeys{},
					chatprovider.UserAgent(),
					nil,
					nil,
				)
				require.Nil(t, model)
				require.EqualError(t, err, tt.wantErr)
			})
		}
	})
}

// TestModelFromConfig_BedrockStripsAnthropicHeaders is a regression test
// for a bug where the Anthropic SDK reads ANTHROPIC_API_KEY from the
// process environment and adds X-Api-Key and Anthropic-Version headers to
// every request. On Bedrock, these headers conflict with SigV4 signing and
// cause auth failures. The SDK's Bedrock middleware strips them before
// signing. This test verifies the outgoing request shape with both
// Anthropic and AWS credentials present.
func TestModelFromConfig_BedrockStripsAnthropicHeaders(t *testing.T) {
	ctx := testutil.Context(t, testutil.WaitShort)

	t.Setenv("ANTHROPIC_API_KEY", "anthropic-env-key")
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("AWS_SESSION_TOKEN", "test-session-token")

	type requestCapture struct {
		Path             string
		Authorization    string
		AnthropicVersion string
		XAPIKey          string
		Body             string
		ReadError        error
	}

	requests := make(chan requestCapture, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)

		requests <- requestCapture{
			Path:             r.URL.Path,
			Authorization:    r.Header.Get("Authorization"),
			AnthropicVersion: r.Header.Get("Anthropic-Version"),
			XAPIKey:          r.Header.Get("X-Api-Key"),
			Body:             string(body),
			ReadError:        err,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(bedrockNonStreamingResponse())
	}))
	defer server.Close()

	model, err := chatprovider.ModelFromConfig(
		fantasybedrock.Name,
		"anthropic.claude-opus-4-6-v1",
		chatprovider.ProviderAPIKeys{
			ByProvider: map[string]string{
				fantasybedrock.Name: "",
			},
			BaseURLByProvider: map[string]string{
				fantasybedrock.Name: server.URL,
			},
			RegionByProvider: map[string]string{
				fantasybedrock.Name: "us-east-2",
			},
		},
		chatprovider.UserAgent(),
		nil,
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, model)

	_, err = model.Generate(ctx, fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "hello"},
				},
			},
		},
	})
	require.NoError(t, err)

	got := testutil.TryReceive(ctx, t, requests)
	require.NoError(t, got.ReadError)
	require.Equal(t, "/model/us.anthropic.claude-opus-4-6-v1/invoke", got.Path)
	require.Empty(t, got.AnthropicVersion)
	require.Empty(t, got.XAPIKey)
	require.Contains(t, got.Authorization, "AWS4-HMAC-SHA256")
	require.NotContains(t, got.Authorization, "anthropic-version")
	require.NotContains(t, got.Authorization, "x-api-key")
	require.Contains(t, got.Body, `"anthropic_version":"bedrock-2023-05-31"`)
}

func TestModelFromConfig_BedrockStreamingHeaders(t *testing.T) {
	ctx := testutil.Context(t, testutil.WaitShort)

	t.Setenv("ANTHROPIC_API_KEY", "anthropic-env-key")
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("AWS_SESSION_TOKEN", "test-session-token")

	type requestCapture struct {
		Path          string
		Accept        string
		BedrockAccept string
		Authorization string
		Body          string
		ReadError     error
	}

	requests := make(chan requestCapture, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)

		requests <- requestCapture{
			Path:          r.URL.Path,
			Accept:        r.Header.Get("Accept"),
			BedrockAccept: r.Header.Get("X-Amzn-Bedrock-Accept"),
			Authorization: r.Header.Get("Authorization"),
			Body:          string(body),
			ReadError:     err,
		}

		if err := writeBedrockAnthropicStream(w,
			`{"type":"message_start","message":{}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`,
			`{"type":"message_stop"}`,
		); err != nil {
			t.Errorf("write bedrock stream: %v", err)
		}
	}))
	defer server.Close()

	model, err := chatprovider.ModelFromConfig(
		fantasybedrock.Name,
		"global.anthropic.claude-opus-4-6-v1",
		chatprovider.ProviderAPIKeys{
			ByProvider: map[string]string{
				fantasybedrock.Name: "",
			},
			BaseURLByProvider: map[string]string{
				fantasybedrock.Name: server.URL,
			},
			RegionByProvider: map[string]string{
				fantasybedrock.Name: "us-east-2",
			},
		},
		chatprovider.UserAgent(),
		nil,
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, model)

	stream, err := model.Stream(ctx, fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "hello"},
				},
			},
		},
	})
	require.NoError(t, err)

	for part := range stream {
		require.NotEqual(t, fantasy.StreamPartTypeError, part.Type)
	}

	got := testutil.TryReceive(ctx, t, requests)
	require.NoError(t, got.ReadError)
	require.Equal(t, "/model/global.anthropic.claude-opus-4-6-v1/invoke-with-response-stream", got.Path)
	require.Empty(t, got.Accept)
	require.Equal(t, "application/json", got.BedrockAccept)
	require.Contains(t, got.Authorization, "AWS4-HMAC-SHA256")
	require.Contains(t, got.Authorization, "x-amzn-bedrock-accept")
	require.Contains(t, got.Body, `"anthropic_version":"bedrock-2023-05-31"`)
}

func writeBedrockAnthropicStream(w http.ResponseWriter, events ...string) error {
	w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
	w.WriteHeader(http.StatusOK)

	encoder := eventstream.NewEncoder()
	for _, event := range events {
		payload, err := json.Marshal(map[string]string{
			"bytes": base64.StdEncoding.EncodeToString([]byte(event)),
		})
		if err != nil {
			return err
		}

		err = encoder.Encode(w, eventstream.Message{
			Headers: eventstream.Headers{
				{
					Name:  eventstreamapi.MessageTypeHeader,
					Value: eventstream.StringValue(eventstreamapi.EventMessageType),
				},
				{
					Name:  eventstreamapi.EventTypeHeader,
					Value: eventstream.StringValue("chunk"),
				},
				{
					Name:  eventstreamapi.ContentTypeHeader,
					Value: eventstream.StringValue("application/json"),
				},
			},
			Payload: payload,
		})
		if err != nil {
			return err
		}
	}

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func bedrockNonStreamingResponse() map[string]any {
	return map[string]any{
		"id":    "msg_01Test",
		"type":  "message",
		"role":  "assistant",
		"model": "claude-sonnet-4-20250514",
		"content": []any{
			map[string]any{
				"type": "text",
				"text": "Hi there",
			},
		},
		"stop_reason":   "end_turn",
		"stop_sequence": "",
		"usage": map[string]any{
			"cache_creation": map[string]any{
				"ephemeral_1h_input_tokens": 0,
				"ephemeral_5m_input_tokens": 0,
			},
			"cache_creation_input_tokens": 0,
			"cache_read_input_tokens":     0,
			"input_tokens":                5,
			"output_tokens":               2,
			"server_tool_use": map[string]any{
				"web_search_requests": 0,
			},
			"service_tier": "standard",
		},
	}
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

		model, err := chatprovider.ModelFromConfig("openai", "gpt-4", keys, chatprovider.UserAgent(), headers, nil)
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

		model, err := chatprovider.ModelFromConfig("anthropic", "claude-sonnet-4-20250514", keys, chatprovider.UserAgent(), headers, nil)
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

	model, err := chatprovider.ModelFromConfig("anthropic", "claude-sonnet-4-20250514", keys, chatprovider.UserAgent(), nil, nil)
	require.NoError(t, err)

	_, err = model.Generate(ctx, fantasy.Call{
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

	model, err := chatprovider.ModelFromConfig("openai", "gpt-4", keys, chatprovider.UserAgent(), nil, nil)
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
	)
	require.NoError(t, err)

	_, err = model.Generate(ctx, fantasy.Call{
		Prompt: []fantasy.Message{{
			Role:    fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
		}},
	})
	require.NoError(t, err)
	_ = testutil.TryReceive(ctx, t, called)
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
			name:         "OpenAIHintStripsCanonicalPrefix",
			modelName:    "anthropic/claude-opus-4.6",
			providerHint: fantasyopenai.Name,
			wantProvider: fantasyanthropic.Name,
			wantModel:    "claude-opus-4.6",
		},
		{
			name:         "OpenAIHintPreservesUnknownSlashNamespace",
			modelName:    "meta-llama/llama-3-70b",
			providerHint: fantasyopenai.Name,
			wantProvider: fantasyopenai.Name,
			wantModel:    "meta-llama/llama-3-70b",
		},
		{
			name:         "AnthropicHintStripsCanonicalPrefix",
			modelName:    "anthropic/claude-4-5-sonnet",
			providerHint: fantasyanthropic.Name,
			wantProvider: fantasyanthropic.Name,
			wantModel:    "claude-4-5-sonnet",
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
