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
