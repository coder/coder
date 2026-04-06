package chatprovider

import (
	"testing"

	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasygoogle "charm.land/fantasy/providers/google"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func ptr[T any](v T) *T {
	return &v
}

func TestIsOpenAIReasoningModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		modelID string
		want    bool
	}{
		{name: "o1", modelID: "o1", want: true},
		{name: "o3", modelID: "o3", want: true},
		{name: "o4", modelID: "o4", want: true},
		{name: "o1-mini", modelID: "o1-mini", want: true},
		{name: "o3-mini", modelID: "o3-mini", want: true},
		{name: "o4-mini", modelID: "o4-mini", want: true},
		{name: "o1.5", modelID: "o1.5", want: true},
		{name: "o12", modelID: "o12", want: true},
		{name: "empty", modelID: "", want: false},
		{name: "single_char_o", modelID: "o", want: false},
		{name: "gpt-4", modelID: "gpt-4", want: false},
		{name: "claude-3", modelID: "claude-3", want: false},
		{name: "oops", modelID: "oops", want: false},
		{name: "o_no_digit", modelID: "oa", want: false},
		{name: "o1x_non_sep", modelID: "o1x", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isOpenAIReasoningModel(tt.modelID)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsChatModelForProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
		modelID  string
		want     bool
	}{
		// OpenAI matches.
		{name: "openai_gpt", provider: "openai", modelID: "gpt-4o", want: true},
		{name: "openai_chatgpt", provider: "openai", modelID: "chatgpt-4o-latest", want: true},
		{name: "openai_reasoning", provider: "openai", modelID: "o3-mini", want: true},
		{name: "openai_no_match", provider: "openai", modelID: "claude-3", want: false},

		// Anthropic matches.
		{name: "anthropic_claude", provider: "anthropic", modelID: "claude-3.5-sonnet", want: true},
		{name: "anthropic_no_match", provider: "anthropic", modelID: "gpt-4", want: false},

		// Google matches.
		{name: "google_gemini", provider: "google", modelID: "gemini-2.5-flash", want: true},
		{name: "google_gemma", provider: "google", modelID: "gemma-7b", want: true},
		{name: "google_no_match", provider: "google", modelID: "gpt-4", want: false},

		// Unknown provider.
		{name: "unknown_provider", provider: "unknown", modelID: "gpt-4", want: false},
		{name: "empty_provider", provider: "", modelID: "gpt-4", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isChatModelForProvider(tt.provider, tt.modelID)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCanonicalModelID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
		modelID  string
		want     string
	}{
		{name: "openai_gpt4", provider: "openai", modelID: "gpt-4", want: "openai:gpt-4"},
		{name: "anthropic_claude", provider: "anthropic", modelID: "claude-3", want: "anthropic:claude-3"},
		{name: "trims_model_whitespace", provider: "openai", modelID: "  gpt-4  ", want: "openai:gpt-4"},
		{name: "unknown_provider_empty", provider: "unknown", modelID: "model", want: ":model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := canonicalModelID(tt.provider, tt.modelID)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMissingProviderAPIKeyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
		wantMsg  string
	}{
		{name: "anthropic", provider: "anthropic", wantMsg: "ANTHROPIC_API_KEY is not set"},
		{name: "azure", provider: "azure", wantMsg: "AZURE_OPENAI_API_KEY is not set"},
		{name: "bedrock", provider: "bedrock", wantMsg: "BEDROCK_API_KEY is not set"},
		{name: "google", provider: "google", wantMsg: "GOOGLE_API_KEY is not set"},
		{name: "openai", provider: "openai", wantMsg: "OPENAI_API_KEY is not set"},
		{name: "openai-compat", provider: "openai-compat", wantMsg: "OPENAI_COMPAT_API_KEY is not set"},
		{name: "openrouter", provider: "openrouter", wantMsg: "OPENROUTER_API_KEY is not set"},
		{name: "vercel", provider: "vercel", wantMsg: "VERCEL_API_KEY is not set"},
		{name: "unknown", provider: "custom", wantMsg: "API key for provider \"custom\" is not set"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := missingProviderAPIKeyError(tt.provider)
			require.Error(t, err)
			assert.Equal(t, tt.wantMsg, err.Error())
		})
	}
}

func TestNormalizedEnumValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		allowed []string
		want    *string
	}{
		{name: "exact_match", value: "low", allowed: []string{"low", "medium", "high"}, want: ptr("low")},
		{name: "case_insensitive_match", value: "low", allowed: []string{"Low", "Medium", "High"}, want: ptr("Low")},
		{name: "no_match", value: "extreme", allowed: []string{"low", "medium", "high"}, want: nil},
		{name: "empty_value", value: "", allowed: []string{"low", "medium", "high"}, want: nil},
		{name: "empty_allowed", value: "low", allowed: []string{}, want: nil},
		{name: "no_allowed", value: "low", allowed: nil, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizedEnumValue(tt.value, tt.allowed...)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, *tt.want, *got)
			}
		})
	}
}

func TestNormalizedStringPointer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input *string
		want  *string
	}{
		{name: "nil", input: nil, want: nil},
		{name: "empty", input: ptr(""), want: nil},
		{name: "whitespace_only", input: ptr("   "), want: nil},
		{name: "value", input: ptr("hello"), want: ptr("hello")},
		{name: "trims_whitespace", input: ptr("  hello  "), want: ptr("hello")},
		{name: "tab_and_newline", input: ptr("\t\n"), want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizedStringPointer(tt.input)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, *tt.want, *got)
			}
		})
	}
}

func TestBoolPtrOrDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  *bool
		def  bool
		want bool
	}{
		{name: "nil_default_true", val: nil, def: true, want: true},
		{name: "nil_default_false", val: nil, def: false, want: false},
		{name: "true_overrides", val: ptr(true), def: false, want: true},
		{name: "false_overrides", val: ptr(false), def: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := boolPtrOrDefault(tt.val, tt.def)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, *got)
		})
	}
}

func TestAnthropicProviderOptionsFromChatConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  *codersdk.ChatModelAnthropicProviderOptions
		assert func(t *testing.T, got *fantasyanthropic.ProviderOptions)
	}{
		{
			name: "all_fields_set",
			input: &codersdk.ChatModelAnthropicProviderOptions{
				SendReasoning:          ptr(true),
				Effort:                 ptr("high"),
				DisableParallelToolUse: ptr(true),
				Thinking: &codersdk.ChatModelAnthropicThinkingOptions{
					BudgetTokens: ptr(int64(10000)),
				},
			},
			assert: func(t *testing.T, got *fantasyanthropic.ProviderOptions) {
				t.Helper()
				require.NotNil(t, got.SendReasoning)
				assert.True(t, *got.SendReasoning)
				require.NotNil(t, got.Effort)
				assert.Equal(t, fantasyanthropic.EffortHigh, *got.Effort)
				require.NotNil(t, got.DisableParallelToolUse)
				assert.True(t, *got.DisableParallelToolUse)
				require.NotNil(t, got.Thinking)
				assert.Equal(t, int64(10000), got.Thinking.BudgetTokens)
			},
		},
		{
			name:  "minimal_fields",
			input: &codersdk.ChatModelAnthropicProviderOptions{},
			assert: func(t *testing.T, got *fantasyanthropic.ProviderOptions) {
				t.Helper()
				assert.Nil(t, got.SendReasoning)
				assert.Nil(t, got.Effort)
				assert.Nil(t, got.DisableParallelToolUse)
				assert.Nil(t, got.Thinking)
			},
		},
		{
			name: "thinking_without_budget",
			input: &codersdk.ChatModelAnthropicProviderOptions{
				Thinking: &codersdk.ChatModelAnthropicThinkingOptions{},
			},
			assert: func(t *testing.T, got *fantasyanthropic.ProviderOptions) {
				t.Helper()
				// BudgetTokens is nil, so Thinking should not be set.
				assert.Nil(t, got.Thinking)
			},
		},
		{
			name: "invalid_effort_returns_nil",
			input: &codersdk.ChatModelAnthropicProviderOptions{
				Effort: ptr("nonexistent"),
			},
			assert: func(t *testing.T, got *fantasyanthropic.ProviderOptions) {
				t.Helper()
				assert.Nil(t, got.Effort)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := anthropicProviderOptionsFromChatConfig(tt.input)
			require.NotNil(t, got)
			tt.assert(t, got)
		})
	}
}

func TestGoogleProviderOptionsFromChatConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  *codersdk.ChatModelGoogleProviderOptions
		assert func(t *testing.T, got *fantasygoogle.ProviderOptions)
	}{
		{
			name: "all_fields_set",
			input: &codersdk.ChatModelGoogleProviderOptions{
				CachedContent: "cachedContents/abc123",
				Threshold:     "BLOCK_NONE",
				ThinkingConfig: &codersdk.ChatModelGoogleThinkingConfig{
					ThinkingBudget:  ptr(int64(8000)),
					IncludeThoughts: ptr(true),
				},
				SafetySettings: []codersdk.ChatModelGoogleSafetySetting{
					{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "BLOCK_LOW_AND_ABOVE"},
					{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "BLOCK_NONE"},
				},
			},
			assert: func(t *testing.T, got *fantasygoogle.ProviderOptions) {
				t.Helper()
				assert.Equal(t, "cachedContents/abc123", got.CachedContent)
				assert.Equal(t, "BLOCK_NONE", got.Threshold)
				require.NotNil(t, got.ThinkingConfig)
				require.NotNil(t, got.ThinkingConfig.ThinkingBudget)
				assert.Equal(t, int64(8000), *got.ThinkingConfig.ThinkingBudget)
				require.NotNil(t, got.ThinkingConfig.IncludeThoughts)
				assert.True(t, *got.ThinkingConfig.IncludeThoughts)
				require.Len(t, got.SafetySettings, 2)
				assert.Equal(t, "HARM_CATEGORY_HATE_SPEECH", got.SafetySettings[0].Category)
				assert.Equal(t, "BLOCK_LOW_AND_ABOVE", got.SafetySettings[0].Threshold)
			},
		},
		{
			name:  "minimal_fields",
			input: &codersdk.ChatModelGoogleProviderOptions{},
			assert: func(t *testing.T, got *fantasygoogle.ProviderOptions) {
				t.Helper()
				assert.Empty(t, got.CachedContent)
				assert.Empty(t, got.Threshold)
				assert.Nil(t, got.ThinkingConfig)
				assert.Nil(t, got.SafetySettings)
			},
		},
		{
			name: "trims_whitespace_in_strings",
			input: &codersdk.ChatModelGoogleProviderOptions{
				CachedContent: "  cachedContents/abc  ",
				Threshold:     "  BLOCK_NONE  ",
				SafetySettings: []codersdk.ChatModelGoogleSafetySetting{
					{Category: "  HARM_CATEGORY_HATE_SPEECH  ", Threshold: "  BLOCK_NONE  "},
				},
			},
			assert: func(t *testing.T, got *fantasygoogle.ProviderOptions) {
				t.Helper()
				assert.Equal(t, "cachedContents/abc", got.CachedContent)
				assert.Equal(t, "BLOCK_NONE", got.Threshold)
				require.Len(t, got.SafetySettings, 1)
				assert.Equal(t, "HARM_CATEGORY_HATE_SPEECH", got.SafetySettings[0].Category)
				assert.Equal(t, "BLOCK_NONE", got.SafetySettings[0].Threshold)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := googleProviderOptionsFromChatConfig(tt.input)
			require.NotNil(t, got)
			tt.assert(t, got)
		})
	}
}

func TestOpenAICompatProviderOptionsFromChatConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  *codersdk.ChatModelOpenAICompatProviderOptions
		assert func(t *testing.T, got *fantasyopenaicompat.ProviderOptions)
	}{
		{
			name: "all_fields_set",
			input: &codersdk.ChatModelOpenAICompatProviderOptions{
				User:            ptr("user-123"),
				ReasoningEffort: ptr("high"),
			},
			assert: func(t *testing.T, got *fantasyopenaicompat.ProviderOptions) {
				t.Helper()
				require.NotNil(t, got.User)
				assert.Equal(t, "user-123", *got.User)
				require.NotNil(t, got.ReasoningEffort)
				assert.Equal(t, fantasyopenai.ReasoningEffortHigh, *got.ReasoningEffort)
			},
		},
		{
			name:  "minimal_fields",
			input: &codersdk.ChatModelOpenAICompatProviderOptions{},
			assert: func(t *testing.T, got *fantasyopenaicompat.ProviderOptions) {
				t.Helper()
				assert.Nil(t, got.User)
				assert.Nil(t, got.ReasoningEffort)
			},
		},
		{
			name: "whitespace_user_returns_nil",
			input: &codersdk.ChatModelOpenAICompatProviderOptions{
				User: ptr("   "),
			},
			assert: func(t *testing.T, got *fantasyopenaicompat.ProviderOptions) {
				t.Helper()
				assert.Nil(t, got.User)
			},
		},
		{
			name: "invalid_effort_returns_nil",
			input: &codersdk.ChatModelOpenAICompatProviderOptions{
				ReasoningEffort: ptr("nonexistent"),
			},
			assert: func(t *testing.T, got *fantasyopenaicompat.ProviderOptions) {
				t.Helper()
				assert.Nil(t, got.ReasoningEffort)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := openAICompatProviderOptionsFromChatConfig(tt.input)
			require.NotNil(t, got)
			tt.assert(t, got)
		})
	}
}

func TestOpenRouterProviderOptionsFromChatConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  *codersdk.ChatModelOpenRouterProviderOptions
		assert func(t *testing.T, got *fantasyopenrouter.ProviderOptions)
	}{
		{
			name: "all_fields_set",
			input: &codersdk.ChatModelOpenRouterProviderOptions{
				ExtraBody:         map[string]any{"key": "value"},
				IncludeUsage:      ptr(true),
				LogitBias:         map[string]int64{"50256": -100},
				LogProbs:          ptr(true),
				ParallelToolCalls: ptr(false),
				User:              ptr("user-456"),
				Reasoning: &codersdk.ChatModelReasoningOptions{
					Enabled:   ptr(true),
					Exclude:   ptr(false),
					MaxTokens: ptr(int64(4096)),
					Effort:    ptr("high"),
				},
				Provider: &codersdk.ChatModelOpenRouterProvider{
					Order:             []string{"anthropic", "openai"},
					AllowFallbacks:    ptr(true),
					RequireParameters: ptr(false),
					DataCollection:    ptr("deny"),
					Only:              []string{"anthropic"},
					Ignore:            []string{"openai"},
					Quantizations:     []string{"int8"},
					Sort:              ptr("price"),
				},
			},
			assert: func(t *testing.T, got *fantasyopenrouter.ProviderOptions) {
				t.Helper()
				assert.Equal(t, map[string]any{"key": "value"}, got.ExtraBody)
				require.NotNil(t, got.IncludeUsage)
				assert.True(t, *got.IncludeUsage)
				assert.Equal(t, map[string]int64{"50256": -100}, got.LogitBias)
				require.NotNil(t, got.LogProbs)
				assert.True(t, *got.LogProbs)
				require.NotNil(t, got.ParallelToolCalls)
				assert.False(t, *got.ParallelToolCalls)
				require.NotNil(t, got.User)
				assert.Equal(t, "user-456", *got.User)

				require.NotNil(t, got.Reasoning)
				require.NotNil(t, got.Reasoning.Enabled)
				assert.True(t, *got.Reasoning.Enabled)
				require.NotNil(t, got.Reasoning.Exclude)
				assert.False(t, *got.Reasoning.Exclude)
				require.NotNil(t, got.Reasoning.MaxTokens)
				assert.Equal(t, int64(4096), *got.Reasoning.MaxTokens)
				require.NotNil(t, got.Reasoning.Effort)
				assert.Equal(t, fantasyopenrouter.ReasoningEffortHigh, *got.Reasoning.Effort)

				require.NotNil(t, got.Provider)
				assert.Equal(t, []string{"anthropic", "openai"}, got.Provider.Order)
				require.NotNil(t, got.Provider.AllowFallbacks)
				assert.True(t, *got.Provider.AllowFallbacks)
				require.NotNil(t, got.Provider.RequireParameters)
				assert.False(t, *got.Provider.RequireParameters)
				require.NotNil(t, got.Provider.DataCollection)
				assert.Equal(t, "deny", *got.Provider.DataCollection)
				assert.Equal(t, []string{"anthropic"}, got.Provider.Only)
				assert.Equal(t, []string{"openai"}, got.Provider.Ignore)
				assert.Equal(t, []string{"int8"}, got.Provider.Quantizations)
				require.NotNil(t, got.Provider.Sort)
				assert.Equal(t, "price", *got.Provider.Sort)
			},
		},
		{
			name:  "minimal_fields",
			input: &codersdk.ChatModelOpenRouterProviderOptions{},
			assert: func(t *testing.T, got *fantasyopenrouter.ProviderOptions) {
				t.Helper()
				assert.Nil(t, got.ExtraBody)
				assert.Nil(t, got.IncludeUsage)
				assert.Nil(t, got.LogitBias)
				assert.Nil(t, got.LogProbs)
				assert.Nil(t, got.ParallelToolCalls)
				assert.Nil(t, got.User)
				assert.Nil(t, got.Reasoning)
				assert.Nil(t, got.Provider)
			},
		},
		{
			name: "reasoning_without_provider",
			input: &codersdk.ChatModelOpenRouterProviderOptions{
				Reasoning: &codersdk.ChatModelReasoningOptions{
					Enabled: ptr(true),
				},
			},
			assert: func(t *testing.T, got *fantasyopenrouter.ProviderOptions) {
				t.Helper()
				require.NotNil(t, got.Reasoning)
				require.NotNil(t, got.Reasoning.Enabled)
				assert.True(t, *got.Reasoning.Enabled)
				assert.Nil(t, got.Provider)
			},
		},
		{
			name: "provider_without_reasoning",
			input: &codersdk.ChatModelOpenRouterProviderOptions{
				Provider: &codersdk.ChatModelOpenRouterProvider{
					Order: []string{"anthropic"},
				},
			},
			assert: func(t *testing.T, got *fantasyopenrouter.ProviderOptions) {
				t.Helper()
				assert.Nil(t, got.Reasoning)
				require.NotNil(t, got.Provider)
				assert.Equal(t, []string{"anthropic"}, got.Provider.Order)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := openRouterProviderOptionsFromChatConfig(tt.input)
			require.NotNil(t, got)
			tt.assert(t, got)
		})
	}
}

func TestVercelProviderOptionsFromChatConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  *codersdk.ChatModelVercelProviderOptions
		assert func(t *testing.T, got *fantasyvercel.ProviderOptions)
	}{
		{
			name: "all_fields_set",
			input: &codersdk.ChatModelVercelProviderOptions{
				User:              ptr("user-789"),
				LogitBias:         map[string]int64{"50256": 50},
				LogProbs:          ptr(true),
				TopLogProbs:       ptr(int64(5)),
				ParallelToolCalls: ptr(true),
				ExtraBody:         map[string]any{"custom": true},
				Reasoning: &codersdk.ChatModelReasoningOptions{
					Enabled:   ptr(true),
					MaxTokens: ptr(int64(2048)),
					Effort:    ptr("high"),
					Exclude:   ptr(false),
				},
				ProviderOptions: &codersdk.ChatModelVercelGatewayProviderOptions{
					Order:  []string{"vertex", "anthropic"},
					Models: []string{"claude-3", "gemini-2"},
				},
			},
			assert: func(t *testing.T, got *fantasyvercel.ProviderOptions) {
				t.Helper()
				require.NotNil(t, got.User)
				assert.Equal(t, "user-789", *got.User)
				assert.Equal(t, map[string]int64{"50256": 50}, got.LogitBias)
				require.NotNil(t, got.LogProbs)
				assert.True(t, *got.LogProbs)
				require.NotNil(t, got.TopLogProbs)
				assert.Equal(t, int64(5), *got.TopLogProbs)
				require.NotNil(t, got.ParallelToolCalls)
				assert.True(t, *got.ParallelToolCalls)
				assert.Equal(t, map[string]any{"custom": true}, got.ExtraBody)

				require.NotNil(t, got.Reasoning)
				require.NotNil(t, got.Reasoning.Enabled)
				assert.True(t, *got.Reasoning.Enabled)
				require.NotNil(t, got.Reasoning.MaxTokens)
				assert.Equal(t, int64(2048), *got.Reasoning.MaxTokens)
				require.NotNil(t, got.Reasoning.Effort)
				assert.Equal(t, fantasyvercel.ReasoningEffortHigh, *got.Reasoning.Effort)
				require.NotNil(t, got.Reasoning.Exclude)
				assert.False(t, *got.Reasoning.Exclude)

				require.NotNil(t, got.ProviderOptions)
				assert.Equal(t, []string{"vertex", "anthropic"}, got.ProviderOptions.Order)
				assert.Equal(t, []string{"claude-3", "gemini-2"}, got.ProviderOptions.Models)
			},
		},
		{
			name:  "minimal_fields",
			input: &codersdk.ChatModelVercelProviderOptions{},
			assert: func(t *testing.T, got *fantasyvercel.ProviderOptions) {
				t.Helper()
				assert.Nil(t, got.User)
				assert.Nil(t, got.LogitBias)
				assert.Nil(t, got.LogProbs)
				assert.Nil(t, got.TopLogProbs)
				assert.Nil(t, got.ParallelToolCalls)
				assert.Nil(t, got.ExtraBody)
				assert.Nil(t, got.Reasoning)
				assert.Nil(t, got.ProviderOptions)
			},
		},
		{
			name: "reasoning_only",
			input: &codersdk.ChatModelVercelProviderOptions{
				Reasoning: &codersdk.ChatModelReasoningOptions{
					Enabled: ptr(true),
				},
			},
			assert: func(t *testing.T, got *fantasyvercel.ProviderOptions) {
				t.Helper()
				require.NotNil(t, got.Reasoning)
				require.NotNil(t, got.Reasoning.Enabled)
				assert.True(t, *got.Reasoning.Enabled)
				assert.Nil(t, got.ProviderOptions)
			},
		},
		{
			name: "provider_options_only",
			input: &codersdk.ChatModelVercelProviderOptions{
				ProviderOptions: &codersdk.ChatModelVercelGatewayProviderOptions{
					Order: []string{"vertex"},
				},
			},
			assert: func(t *testing.T, got *fantasyvercel.ProviderOptions) {
				t.Helper()
				assert.Nil(t, got.Reasoning)
				require.NotNil(t, got.ProviderOptions)
				assert.Equal(t, []string{"vertex"}, got.ProviderOptions.Order)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := vercelProviderOptionsFromChatConfig(tt.input)
			require.NotNil(t, got)
			tt.assert(t, got)
		})
	}
}
