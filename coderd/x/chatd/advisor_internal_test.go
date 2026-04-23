package chatd //nolint:testpackage // Accesses unexported advisor helpers.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatadvisor"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// TestApplyAdvisorReasoningEffort exercises each provider's mutation branch so
// a typo or incorrect type assertion in any single branch fails a unit test
// rather than shipping silently.
func TestApplyAdvisorReasoningEffort(t *testing.T) {
	t.Parallel()

	t.Run("NilProviderOptionsIsNoOp", func(t *testing.T) {
		t.Parallel()
		// Must not panic.
		applyAdvisorReasoningEffort(nil, "medium")
	})

	t.Run("EmptyEffortIsNoOp", func(t *testing.T) {
		t.Parallel()
		effort := fantasyopenai.ReasoningEffortLow
		opts := &fantasyopenai.ProviderOptions{ReasoningEffort: &effort}
		providerOptions := fantasy.ProviderOptions{fantasyopenai.Name: opts}

		applyAdvisorReasoningEffort(providerOptions, "   ")
		require.NotNil(t, opts.ReasoningEffort)
		require.Equal(t, fantasyopenai.ReasoningEffortLow, *opts.ReasoningEffort)
	})

	t.Run("UnrecognizedEffortLeavesOptionsUntouched", func(t *testing.T) {
		t.Parallel()
		opts := &fantasyopenai.ProviderOptions{}
		providerOptions := fantasy.ProviderOptions{fantasyopenai.Name: opts}

		applyAdvisorReasoningEffort(providerOptions, "not-a-real-effort")
		require.Nil(t, opts.ReasoningEffort)
	})

	t.Run("OpenAIProviderOptions", func(t *testing.T) {
		t.Parallel()
		opts := &fantasyopenai.ProviderOptions{}
		providerOptions := fantasy.ProviderOptions{fantasyopenai.Name: opts}

		applyAdvisorReasoningEffort(providerOptions, "medium")
		require.NotNil(t, opts.ReasoningEffort)
		require.Equal(t, fantasyopenai.ReasoningEffortMedium, *opts.ReasoningEffort)
	})

	t.Run("OpenAIResponsesProviderOptions", func(t *testing.T) {
		t.Parallel()
		opts := &fantasyopenai.ResponsesProviderOptions{}
		providerOptions := fantasy.ProviderOptions{fantasyopenai.Name: opts}

		applyAdvisorReasoningEffort(providerOptions, "medium")
		require.NotNil(t, opts.ReasoningEffort)
		require.Equal(t, fantasyopenai.ReasoningEffortMedium, *opts.ReasoningEffort)
	})

	t.Run("OpenAICompatProviderOptions", func(t *testing.T) {
		t.Parallel()
		opts := &fantasyopenaicompat.ProviderOptions{}
		providerOptions := fantasy.ProviderOptions{fantasyopenaicompat.Name: opts}

		applyAdvisorReasoningEffort(providerOptions, "medium")
		require.NotNil(t, opts.ReasoningEffort)
		require.Equal(t, fantasyopenai.ReasoningEffortMedium, *opts.ReasoningEffort)
	})

	t.Run("AnthropicProviderOptions", func(t *testing.T) {
		t.Parallel()
		opts := &fantasyanthropic.ProviderOptions{}
		providerOptions := fantasy.ProviderOptions{fantasyanthropic.Name: opts}

		applyAdvisorReasoningEffort(providerOptions, "high")
		require.NotNil(t, opts.Effort)
		require.Equal(t, fantasyanthropic.EffortHigh, *opts.Effort)
	})

	t.Run("OpenRouterAllocatesReasoningOptions", func(t *testing.T) {
		t.Parallel()
		opts := &fantasyopenrouter.ProviderOptions{}
		providerOptions := fantasy.ProviderOptions{fantasyopenrouter.Name: opts}

		applyAdvisorReasoningEffort(providerOptions, "medium")
		require.NotNil(t, opts.Reasoning, "Reasoning container must be allocated")
		require.NotNil(t, opts.Reasoning.Effort)
		require.Equal(t, fantasyopenrouter.ReasoningEffort("medium"), *opts.Reasoning.Effort)
	})

	t.Run("OpenRouterPreservesExistingReasoningContainer", func(t *testing.T) {
		t.Parallel()
		enabled := true
		opts := &fantasyopenrouter.ProviderOptions{
			Reasoning: &fantasyopenrouter.ReasoningOptions{Enabled: &enabled},
		}
		providerOptions := fantasy.ProviderOptions{fantasyopenrouter.Name: opts}

		applyAdvisorReasoningEffort(providerOptions, "high")
		require.NotNil(t, opts.Reasoning.Enabled)
		require.True(t, *opts.Reasoning.Enabled)
		require.NotNil(t, opts.Reasoning.Effort)
		require.Equal(t, fantasyopenrouter.ReasoningEffort("high"), *opts.Reasoning.Effort)
	})

	t.Run("VercelAllocatesReasoningOptions", func(t *testing.T) {
		t.Parallel()
		opts := &fantasyvercel.ProviderOptions{}
		providerOptions := fantasy.ProviderOptions{fantasyvercel.Name: opts}

		applyAdvisorReasoningEffort(providerOptions, "minimal")
		require.NotNil(t, opts.Reasoning)
		require.NotNil(t, opts.Reasoning.Effort)
		require.Equal(t, fantasyvercel.ReasoningEffortMinimal, *opts.Reasoning.Effort)
	})

	t.Run("MultipleProvidersReceiveMutations", func(t *testing.T) {
		t.Parallel()
		openaiOpts := &fantasyopenai.ProviderOptions{}
		anthropicOpts := &fantasyanthropic.ProviderOptions{}
		providerOptions := fantasy.ProviderOptions{
			fantasyopenai.Name:    openaiOpts,
			fantasyanthropic.Name: anthropicOpts,
		}

		applyAdvisorReasoningEffort(providerOptions, "high")
		require.NotNil(t, openaiOpts.ReasoningEffort)
		require.Equal(t, fantasyopenai.ReasoningEffortHigh, *openaiOpts.ReasoningEffort)
		require.NotNil(t, anthropicOpts.Effort)
		require.Equal(t, fantasyanthropic.EffortHigh, *anthropicOpts.Effort)
	})
}

// advisorOverrideStubStore stubs only the database methods that
// resolveAdvisorModelOverride exercises via the chatConfigCache.
type advisorOverrideStubStore struct {
	database.Store

	getChatModelConfigByID func(context.Context, uuid.UUID) (database.ChatModelConfig, error)
}

func (s *advisorOverrideStubStore) GetChatModelConfigByID(
	ctx context.Context,
	id uuid.UUID,
) (database.ChatModelConfig, error) {
	if s.getChatModelConfigByID == nil {
		return database.ChatModelConfig{}, xerrors.New("unexpected GetChatModelConfigByID call")
	}
	return s.getChatModelConfigByID(ctx, id)
}

func newAdvisorTestServer(
	ctx context.Context,
	t *testing.T,
	store database.Store,
) *Server {
	t.Helper()
	clock := quartz.NewMock(t)
	return &Server{configCache: newChatConfigCache(ctx, store, clock)}
}

// TestResolveAdvisorModelOverride covers the early-return, each fallback
// branch, and the success path. Prior tests only hit the ModelConfigID ==
// uuid.Nil early return, so the override body never executed.
func TestResolveAdvisorModelOverride(t *testing.T) {
	t.Parallel()

	fallbackModel := &chattest.FakeModel{ProviderName: "stub", ModelName: "stub"}
	fallbackCallConfig := codersdk.ChatModelCallConfig{}
	logger := slog.Make()

	t.Run("NilModelConfigReturnsFallback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		// Panic if the cache is consulted; the early return must skip it.
		store := &advisorOverrideStubStore{}
		p := newAdvisorTestServer(ctx, t, store)

		gotModel, gotCfg := p.resolveAdvisorModelOverride(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{},
			fallbackModel,
			fallbackCallConfig,
			chatprovider.ProviderAPIKeys{},
			logger,
		)
		require.Equal(t, fallbackModel, gotModel)
		require.Equal(t, fallbackCallConfig, gotCfg)
	})

	t.Run("ConfigLookupErrorReturnsFallback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &advisorOverrideStubStore{
			getChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{}, xerrors.New("lookup failed")
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		gotModel, gotCfg := p.resolveAdvisorModelOverride(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{ModelConfigID: uuid.New()},
			fallbackModel,
			fallbackCallConfig,
			chatprovider.ProviderAPIKeys{OpenAI: "sk-test"},
			logger,
		)
		require.Equal(t, fallbackModel, gotModel)
		require.Equal(t, fallbackCallConfig, gotCfg)
	})

	t.Run("InvalidOptionsJSONReturnsFallback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		configID := uuid.New()
		store := &advisorOverrideStubStore{
			getChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{
					ID:          configID,
					Provider:    "openai",
					Model:       "gpt-5.2",
					Enabled:     true,
					CreatedAt:   time.Unix(0, 0).UTC(),
					UpdatedAt:   time.Unix(0, 0).UTC(),
					Options:     []byte("not valid json"),
					DisplayName: "gpt-5.2",
				}, nil
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		gotModel, gotCfg := p.resolveAdvisorModelOverride(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{ModelConfigID: configID},
			fallbackModel,
			fallbackCallConfig,
			chatprovider.ProviderAPIKeys{OpenAI: "sk-test"},
			logger,
		)
		require.Equal(t, fallbackModel, gotModel)
		require.Equal(t, fallbackCallConfig, gotCfg)
	})

	t.Run("MissingProviderKeyReturnsFallback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		configID := uuid.New()
		store := &advisorOverrideStubStore{
			getChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{
					ID:          configID,
					Provider:    "openai",
					Model:       "gpt-5.2",
					Enabled:     true,
					CreatedAt:   time.Unix(0, 0).UTC(),
					UpdatedAt:   time.Unix(0, 0).UTC(),
					DisplayName: "gpt-5.2",
				}, nil
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		gotModel, gotCfg := p.resolveAdvisorModelOverride(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{ModelConfigID: configID},
			fallbackModel,
			fallbackCallConfig,
			chatprovider.ProviderAPIKeys{},
			logger,
		)
		require.Equal(t, fallbackModel, gotModel)
		require.Equal(t, fallbackCallConfig, gotCfg)
	})

	t.Run("SuccessReturnsOverrideModelAndConfig", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		configID := uuid.New()
		rawOptions, err := json.Marshal(codersdk.ChatModelCallConfig{
			Temperature: func() *float64 { v := 0.42; return &v }(),
		})
		require.NoError(t, err)
		store := &advisorOverrideStubStore{
			getChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{
					ID:          configID,
					Provider:    "openai",
					Model:       "gpt-5.2",
					Enabled:     true,
					CreatedAt:   time.Unix(0, 0).UTC(),
					UpdatedAt:   time.Unix(0, 0).UTC(),
					Options:     rawOptions,
					DisplayName: "gpt-5.2",
				}, nil
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		gotModel, gotCfg := p.resolveAdvisorModelOverride(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{ModelConfigID: configID},
			fallbackModel,
			fallbackCallConfig,
			chatprovider.ProviderAPIKeys{OpenAI: "sk-test"},
			logger,
		)
		require.NotEqual(t, fantasy.LanguageModel(fallbackModel), gotModel,
			"success path must return the override model, not the fallback")
		require.NotNil(t, gotModel)
		require.Equal(t, "openai", gotModel.Provider())
		require.NotNil(t, gotCfg.Temperature)
		require.InDelta(t, 0.42, *gotCfg.Temperature, 1e-9)
	})
}

// TestStripAdvisorGuidanceBlock exercises the filter that keeps the advisor
// from receiving the parent-facing advisor-guidance instruction in its nested
// context. The block references a tool the advisor cannot use, so forwarding
// it wastes context tokens and risks steering the advisor's reply.
func TestStripAdvisorGuidanceBlock(t *testing.T) {
	t.Parallel()

	t.Run("RemovesGuidanceSystemMessage", func(t *testing.T) {
		t.Parallel()
		msgs := []fantasy.Message{
			{
				Role: fantasy.MessageRoleSystem,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "You are a helpful assistant."},
				},
			},
			{
				Role: fantasy.MessageRoleSystem,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: chatadvisor.ParentGuidanceBlock},
				},
			},
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "Help me plan."},
				},
			},
		}

		filtered := stripAdvisorGuidanceBlock(msgs)
		require.Len(t, filtered, 2)
		for _, msg := range filtered {
			for _, part := range msg.Content {
				if text, ok := part.(fantasy.TextPart); ok {
					require.NotEqual(t, chatadvisor.ParentGuidanceBlock, text.Text,
						"guidance block must not survive the filter")
				}
			}
		}
	})

	t.Run("LeavesOtherSystemMessagesIntact", func(t *testing.T) {
		t.Parallel()
		msgs := []fantasy.Message{
			{
				Role: fantasy.MessageRoleSystem,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "instruction file"},
				},
			},
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "hi"},
				},
			},
		}

		filtered := stripAdvisorGuidanceBlock(msgs)
		require.Len(t, filtered, 2)
	})

	t.Run("IgnoresNonSystemRoleWithMatchingText", func(t *testing.T) {
		t.Parallel()
		// A user message echoing the guidance block must not be stripped:
		// the filter only targets the system-role injection.
		msgs := []fantasy.Message{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: chatadvisor.ParentGuidanceBlock},
				},
			},
		}

		filtered := stripAdvisorGuidanceBlock(msgs)
		require.Len(t, filtered, 1)
	})
}

// TestEnsureAdvisorProviderOptions covers the seeding path that prevents
// admin-configured reasoning_effort from being silently dropped when a
// model config carries no provider_options block.
func TestEnsureAdvisorProviderOptions(t *testing.T) {
	t.Parallel()

	t.Run("EmptyEffortReturnsInputUnchanged", func(t *testing.T) {
		t.Parallel()
		model := &chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "gpt-4"}
		got := ensureAdvisorProviderOptions(nil, model, "")
		require.Nil(t, got)
	})

	t.Run("NilModelReturnsInputUnchanged", func(t *testing.T) {
		t.Parallel()
		got := ensureAdvisorProviderOptions(nil, nil, "medium")
		require.Nil(t, got)
	})

	t.Run("UnknownProviderReturnsInputUnchanged", func(t *testing.T) {
		t.Parallel()
		model := &chattest.FakeModel{ProviderName: "unknown", ModelName: "x"}
		got := ensureAdvisorProviderOptions(nil, model, "medium")
		require.Nil(t, got)
	})

	t.Run("SeedsOpenAICompletionsProviderOptions", func(t *testing.T) {
		t.Parallel()
		// A model name absent from the Responses allowlist must seed
		// the completions options struct.
		model := &chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "not-a-real-openai-model"}
		got := ensureAdvisorProviderOptions(nil, model, "medium")
		require.NotNil(t, got)
		raw, ok := got[fantasyopenai.Name]
		require.True(t, ok)
		_, ok = raw.(*fantasyopenai.ProviderOptions)
		require.True(t, ok, "expected *ProviderOptions for non-Responses model, got %T", raw)
	})

	t.Run("SeedsOpenAIResponsesProviderOptions", func(t *testing.T) {
		t.Parallel()
		// A model name in the Responses allowlist must seed the
		// Responses-specific options struct so the OpenAI provider
		// routes to the Responses endpoint.
		model := &chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "gpt-4"}
		got := ensureAdvisorProviderOptions(nil, model, "medium")
		require.NotNil(t, got)
		raw, ok := got[fantasyopenai.Name]
		require.True(t, ok)
		_, ok = raw.(*fantasyopenai.ResponsesProviderOptions)
		require.True(t, ok, "expected *ResponsesProviderOptions for Responses model, got %T", raw)
	})

	t.Run("SeedsAnthropicProviderOptions", func(t *testing.T) {
		t.Parallel()
		model := &chattest.FakeModel{ProviderName: fantasyanthropic.Name, ModelName: "claude-3-5"}
		got := ensureAdvisorProviderOptions(nil, model, "high")
		require.NotNil(t, got)
		raw, ok := got[fantasyanthropic.Name]
		require.True(t, ok)
		_, ok = raw.(*fantasyanthropic.ProviderOptions)
		require.True(t, ok)
	})

	t.Run("SeedsOpenRouterProviderOptions", func(t *testing.T) {
		t.Parallel()
		model := &chattest.FakeModel{ProviderName: fantasyopenrouter.Name, ModelName: "openrouter-x"}
		got := ensureAdvisorProviderOptions(nil, model, "low")
		require.NotNil(t, got)
		raw, ok := got[fantasyopenrouter.Name]
		require.True(t, ok)
		_, ok = raw.(*fantasyopenrouter.ProviderOptions)
		require.True(t, ok)
	})

	t.Run("PreservesExistingProviderEntry", func(t *testing.T) {
		t.Parallel()
		existing := &fantasyopenai.ProviderOptions{}
		existingEffort := fantasyopenai.ReasoningEffortLow
		existing.ReasoningEffort = &existingEffort
		providerOptions := fantasy.ProviderOptions{fantasyopenai.Name: existing}

		model := &chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "gpt-4"}
		got := ensureAdvisorProviderOptions(providerOptions, model, "medium")
		require.Same(t, existing, got[fantasyopenai.Name],
			"existing provider entry must not be replaced")
	})

	t.Run("SeedSurvivesApplyReasoningEffort", func(t *testing.T) {
		t.Parallel()
		// Integration-style check: seed + apply together must produce
		// a populated effort field even when the input is nil.
		model := &chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "not-a-real-openai-model"}
		providerOptions := ensureAdvisorProviderOptions(nil, model, "medium")
		applyAdvisorReasoningEffort(providerOptions, "medium")

		opts, ok := providerOptions[fantasyopenai.Name].(*fantasyopenai.ProviderOptions)
		require.True(t, ok)
		require.NotNil(t, opts.ReasoningEffort)
		require.Equal(t, fantasyopenai.ReasoningEffortMedium, *opts.ReasoningEffort)
	})
}

// TestNewAdvisorRuntime covers the three defensive branches in
// newAdvisorRuntime that gate whether the runtime is created and with what
// bounds. Without this coverage a regression in any branch ships silently.
func TestNewAdvisorRuntime(t *testing.T) {
	t.Parallel()

	logger := slog.Make()
	fallbackModel := &chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "gpt-4"}
	fallbackCallConfig := codersdk.ChatModelCallConfig{}

	t.Run("ZeroMaxUsesDefaultsToMaxChatSteps", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &advisorOverrideStubStore{}
		p := newAdvisorTestServer(ctx, t, store)

		rt := p.newAdvisorRuntime(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{
				Enabled:         true,
				MaxUsesPerRun:   0,
				MaxOutputTokens: 16384,
			},
			fallbackModel,
			fallbackCallConfig,
			chatprovider.ProviderAPIKeys{},
			logger,
		)
		require.NotNil(t, rt, "zero max uses must default rather than bail out")
		require.Equal(t, maxChatSteps, rt.RemainingUses(),
			"zero max uses must be replaced with maxChatSteps")
	})

	t.Run("NegativeMaxUsesReturnsNil", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &advisorOverrideStubStore{}
		p := newAdvisorTestServer(ctx, t, store)

		rt := p.newAdvisorRuntime(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{
				Enabled:         true,
				MaxUsesPerRun:   -1,
				MaxOutputTokens: 16384,
			},
			fallbackModel,
			fallbackCallConfig,
			chatprovider.ProviderAPIKeys{},
			logger,
		)
		require.Nil(t, rt, "negative max uses must disable the advisor")
	})

	t.Run("ZeroMaxOutputTokensDefaults", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &advisorOverrideStubStore{}
		p := newAdvisorTestServer(ctx, t, store)

		rt := p.newAdvisorRuntime(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{
				Enabled:         true,
				MaxUsesPerRun:   3,
				MaxOutputTokens: 0,
			},
			fallbackModel,
			fallbackCallConfig,
			chatprovider.ProviderAPIKeys{},
			logger,
		)
		require.NotNil(t, rt,
			"zero max output tokens must default to defaultAdvisorMaxOutputTokens, not disable the advisor")
		require.Equal(t, 3, rt.RemainingUses())
	})
}
