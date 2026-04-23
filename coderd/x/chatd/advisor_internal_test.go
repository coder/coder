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
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
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

type stubLanguageModel struct{}

func (stubLanguageModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	return nil, xerrors.New("stub")
}

func (stubLanguageModel) Stream(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
	return nil, xerrors.New("stub")
}

func (stubLanguageModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, xerrors.New("stub")
}

func (stubLanguageModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, xerrors.New("stub")
}

func (stubLanguageModel) Provider() string { return "stub" }
func (stubLanguageModel) Model() string    { return "stub" }

// TestResolveAdvisorModelOverride covers the early-return, each fallback
// branch, and the success path. Prior tests only hit the ModelConfigID ==
// uuid.Nil early return, so the override body never executed.
func TestResolveAdvisorModelOverride(t *testing.T) {
	t.Parallel()

	fallbackModel := stubLanguageModel{}
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
