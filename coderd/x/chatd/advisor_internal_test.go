package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatadvisor"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// advisorOverrideStubStore stubs only the database methods that
// resolveAdvisorModelOverride exercises. The prod code calls
// GetEnabledChatModelConfigByID so the query joins ai_providers and
// filters both enabled flags atomically. Tests simulate that by returning
// configs the stub treats as enabled.
type advisorOverrideStubStore struct {
	database.Store

	getEnabledChatModelConfigByID  func(context.Context, uuid.UUID) (database.ChatModelConfig, error)
	getAIProviderByID              func(context.Context, uuid.UUID) (database.AIProvider, error)
	getAIProviders                 func(context.Context, database.GetAIProvidersParams) ([]database.AIProvider, error)
	getAIProviderKeysByProviderID  func(context.Context, uuid.UUID) ([]database.AIProviderKey, error)
	getAIProviderKeysByProviderIDs func(context.Context, []uuid.UUID) ([]database.AIProviderKey, error)
}

func (s *advisorOverrideStubStore) GetEnabledChatModelConfigByID(
	ctx context.Context,
	id uuid.UUID,
) (database.ChatModelConfig, error) {
	if s.getEnabledChatModelConfigByID == nil {
		return database.ChatModelConfig{}, xerrors.New("unexpected GetEnabledChatModelConfigByID call")
	}
	return s.getEnabledChatModelConfigByID(ctx, id)
}

func (s *advisorOverrideStubStore) GetAIProviderByID(
	ctx context.Context,
	id uuid.UUID,
) (database.AIProvider, error) {
	if s.getAIProviderByID == nil {
		return database.AIProvider{}, xerrors.New("unexpected GetAIProviderByID call")
	}
	return s.getAIProviderByID(ctx, id)
}

func (s *advisorOverrideStubStore) GetAIProviders(
	ctx context.Context,
	params database.GetAIProvidersParams,
) ([]database.AIProvider, error) {
	if s.getAIProviders == nil {
		return nil, xerrors.New("unexpected GetAIProviders call")
	}
	return s.getAIProviders(ctx, params)
}

func (s *advisorOverrideStubStore) GetAIProviderKeysByProviderID(
	ctx context.Context,
	providerID uuid.UUID,
) ([]database.AIProviderKey, error) {
	if s.getAIProviderKeysByProviderID == nil {
		return nil, xerrors.New("unexpected GetAIProviderKeysByProviderID call")
	}
	return s.getAIProviderKeysByProviderID(ctx, providerID)
}

func (s *advisorOverrideStubStore) GetAIProviderKeysByProviderIDs(
	ctx context.Context,
	providerIDs []uuid.UUID,
) ([]database.AIProviderKey, error) {
	if s.getAIProviderKeysByProviderIDs == nil {
		return nil, xerrors.New("unexpected GetAIProviderKeysByProviderIDs call")
	}
	return s.getAIProviderKeysByProviderIDs(ctx, providerIDs)
}

func newAdvisorTestServer(
	ctx context.Context,
	t *testing.T,
	store database.Store,
) *Server {
	t.Helper()
	clock := quartz.NewMock(t)
	return &Server{
		db:          store,
		configCache: newChatConfigCache(ctx, store, clock),
	}
}

func (p *Server) resolveAdvisorModelOverrideOrFallback(
	ctx context.Context,
	chat database.Chat,
	advisorCfg codersdk.AdvisorConfig,
	fallbackModel fantasy.LanguageModel,
	fallbackCallConfig codersdk.ChatModelCallConfig,
	modelOpts modelBuildOptions,
	logger slog.Logger,
) (fantasy.LanguageModel, codersdk.ChatModelCallConfig) {
	model, cfg, err := p.resolveAdvisorModelOverride(
		ctx,
		chat,
		advisorCfg,
		fallbackModel,
		fallbackCallConfig,
		modelOpts,
		logger,
	)
	if err != nil {
		logger.Warn(ctx, "failed to resolve advisor model override, continuing with chat model", slog.Error(err))
		return fallbackModel, fallbackCallConfig
	}
	return model, cfg
}

func (p *Server) newAdvisorRuntimeOrFallback(
	ctx context.Context,
	chat database.Chat,
	advisorCfg codersdk.AdvisorConfig,
	fallbackModel fantasy.LanguageModel,
	fallbackCallConfig codersdk.ChatModelCallConfig,
	modelOpts modelBuildOptions,
	logger slog.Logger,
) *chatadvisor.Runtime {
	rt, err := p.newAdvisorRuntime(
		ctx,
		chat,
		advisorCfg,
		fallbackModel,
		fallbackCallConfig,
		modelOpts,
		logger,
	)
	if err != nil {
		logger.Warn(ctx, "failed to create advisor runtime, continuing without advisor", slog.Error(err))
		return nil
	}
	return rt
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

		gotModel, gotCfg := p.resolveAdvisorModelOverrideOrFallback(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{},
			fallbackModel,
			fallbackCallConfig,
			modelBuildOptions{},
			logger,
		)
		require.Equal(t, fallbackModel, gotModel)
		require.Equal(t, fallbackCallConfig, gotCfg)
	})

	t.Run("ConfigLookupErrorReturnsFallback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &advisorOverrideStubStore{
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{}, xerrors.New("lookup failed")
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		gotModel, gotCfg := p.resolveAdvisorModelOverrideOrFallback(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{ModelConfigID: uuid.New()},
			fallbackModel,
			fallbackCallConfig,
			modelBuildOptions{},
			logger,
		)
		require.Equal(t, fallbackModel, gotModel)
		require.Equal(t, fallbackCallConfig, gotCfg)
	})

	// Covers the sql.ErrNoRows branch separately from the generic-error
	// branch above. GetEnabledChatModelConfigByID returns ErrNoRows when
	// an admin disables the advisor model or its provider, and that case
	// has a distinct log message. Without this test, removing the
	// errors.Is(err, sql.ErrNoRows) check would still pass the sibling
	// test.
	t.Run("DisabledProviderReturnsFallback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &advisorOverrideStubStore{
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{}, sql.ErrNoRows
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		gotModel, gotCfg := p.resolveAdvisorModelOverrideOrFallback(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{ModelConfigID: uuid.New()},
			fallbackModel,
			fallbackCallConfig,
			modelBuildOptions{},
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
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{
					ID:          configID,
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

		gotModel, gotCfg := p.resolveAdvisorModelOverrideOrFallback(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{ModelConfigID: configID},
			fallbackModel,
			fallbackCallConfig,
			modelBuildOptions{},
			logger,
		)
		require.Equal(t, fallbackModel, gotModel)
		require.Equal(t, fallbackCallConfig, gotCfg)
	})

	t.Run("MissingProviderKeyReturnsFallback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		configID := uuid.New()
		providerID := uuid.New()
		store := &advisorOverrideStubStore{
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{
					ID:          configID,
					Model:       "gpt-5.2",
					Enabled:     true,
					CreatedAt:   time.Unix(0, 0).UTC(),
					UpdatedAt:   time.Unix(0, 0).UTC(),
					DisplayName: "gpt-5.2",
				}, nil
			},
			getAIProviders: func(context.Context, database.GetAIProvidersParams) ([]database.AIProvider, error) {
				return []database.AIProvider{{
					ID:      providerID,
					Type:    database.AIProviderTypeOpenai,
					Enabled: true,
				}}, nil
			},
			getAIProviderKeysByProviderIDs: func(context.Context, []uuid.UUID) ([]database.AIProviderKey, error) {
				return nil, nil
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		gotModel, gotCfg := p.resolveAdvisorModelOverrideOrFallback(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{ModelConfigID: configID},
			fallbackModel,
			fallbackCallConfig,
			modelBuildOptions{},
			logger,
		)
		require.Equal(t, fallbackModel, gotModel)
		require.Equal(t, fallbackCallConfig, gotCfg)
	})

	t.Run("SuccessReturnsOverrideModelAndConfig", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		configID := uuid.New()
		providerID := uuid.New()
		rawOptions, err := json.Marshal(codersdk.ChatModelCallConfig{
			Temperature: func() *float64 { v := 0.42; return &v }(),
			ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{
				Default: ptr.Ref(codersdk.ChatModelReasoningEffortLow),
				Max:     ptr.Ref(codersdk.ChatModelReasoningEffortXHigh),
			},
		})
		require.NoError(t, err)
		store := &advisorOverrideStubStore{
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{
					ID:           configID,
					Model:        "gpt-5.2",
					Enabled:      true,
					CreatedAt:    time.Unix(0, 0).UTC(),
					UpdatedAt:    time.Unix(0, 0).UTC(),
					Options:      rawOptions,
					DisplayName:  "gpt-5.2",
					AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
				}, nil
			},
			getAIProviderByID: func(context.Context, uuid.UUID) (database.AIProvider, error) {
				return aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil
			},
		}
		p := newAdvisorTestServer(ctx, t, store)
		p.aibridgeTransportFactory = aibridgeTestFactoryPointer(&aibridgeTestFactory{rt: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		})})

		gotModel, gotCfg := p.resolveAdvisorModelOverrideOrFallback(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{
				ModelConfigID:   configID,
				ReasoningEffort: ptr.Ref(codersdk.ChatModelReasoningEffortHigh),
			},
			fallbackModel,
			fallbackCallConfig,
			modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
			logger,
		)
		require.NotEqual(t, fantasy.LanguageModel(fallbackModel), gotModel,
			"success path must return the override model, not the fallback")
		require.NotNil(t, gotModel)
		require.Equal(t, "openai", gotModel.Provider())
		// Guard against ModelFromConfig silently ignoring the model field
		// and returning a default. The override is only useful if the
		// model name from the config row actually propagates.
		require.Equal(t, "gpt-5.2", gotModel.Model())
		require.NotNil(t, gotCfg.Temperature)
		require.InDelta(t, 0.42, *gotCfg.Temperature, 1e-9)
		require.NotNil(t, gotCfg.ReasoningEffort)
		require.Equal(t, codersdk.ChatModelReasoningEffortHigh, *gotCfg.ReasoningEffort.Default)
		require.Equal(t, codersdk.ChatModelReasoningEffortHigh, *gotCfg.ReasoningEffort.Max)
	})
	t.Run("AIProviderIDResolvesOverrideProviderKeys", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		configID := uuid.New()
		providerID := uuid.New()
		store := &advisorOverrideStubStore{
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{
					ID:           configID,
					Model:        "gpt-5.2",
					Enabled:      true,
					CreatedAt:    time.Unix(0, 0).UTC(),
					UpdatedAt:    time.Unix(0, 0).UTC(),
					DisplayName:  "gpt-5.2",
					AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
				}, nil
			},
			getAIProviderByID: func(context.Context, uuid.UUID) (database.AIProvider, error) {
				return aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil
			},
			getAIProviderKeysByProviderID: func(context.Context, uuid.UUID) ([]database.AIProviderKey, error) {
				return []database.AIProviderKey{{
					ProviderID: providerID,
					APIKey:     "sk-selected",
				}}, nil
			},
		}
		p := newAdvisorTestServer(ctx, t, store)
		p.aibridgeTransportFactory = aibridgeTestFactoryPointer(&aibridgeTestFactory{rt: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		})})

		gotModel, gotCfg := p.resolveAdvisorModelOverrideOrFallback(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{ModelConfigID: configID},
			fallbackModel,
			fallbackCallConfig,
			modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
			logger,
		)
		require.NotEqual(t, fantasy.LanguageModel(fallbackModel), gotModel)
		require.NotNil(t, gotModel)
		require.Equal(t, "openai", gotModel.Provider())
		require.Equal(t, "gpt-5.2", gotModel.Model())
		require.Equal(t, fallbackCallConfig, gotCfg)
	})
}

func TestResolveAdvisorModelOverridePromotesAIBridgeErrors(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	configID := uuid.New()
	providerID := uuid.New()
	store := &advisorOverrideStubStore{
		getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
			return database.ChatModelConfig{
				ID:           configID,
				Model:        "gpt-5.2",
				Enabled:      true,
				DisplayName:  "gpt-5.2",
				AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
			}, nil
		},
		getAIProviderByID: func(context.Context, uuid.UUID) (database.AIProvider, error) {
			return database.AIProvider{ID: providerID, Type: database.AIProviderTypeOpenai, Name: "primary-openai", Enabled: true}, nil
		},
		getAIProviderKeysByProviderID: func(context.Context, uuid.UUID) ([]database.AIProviderKey, error) {
			return []database.AIProviderKey{{ProviderID: providerID, APIKey: "sk-selected"}}, nil
		},
	}
	p := newAdvisorTestServer(ctx, t, store)

	ctx = aibridge.WithDelegatedAPIKeyID(ctx, uuid.NewString())
	model, _, err := p.resolveAdvisorModelOverride(
		ctx,
		database.Chat{ID: uuid.New(), OwnerID: uuid.New()},
		codersdk.AdvisorConfig{ModelConfigID: configID},
		&chattest.FakeModel{ProviderName: "stub", ModelName: "stub"},
		codersdk.ChatModelCallConfig{},
		modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
		slog.Make(),
	)
	require.ErrorContains(t, err, "AI Gateway transport factory")
	require.Nil(t, model)
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

// TestNewAdvisorRuntime covers the three defensive branches in
// newAdvisorRuntime that gate whether the runtime is created and with what
// bounds. Without this coverage a regression in any branch ships silently.
func TestNewAdvisorRuntime(t *testing.T) {
	t.Parallel()

	logger := slog.Make()
	fallbackModel := &chattest.FakeModel{ProviderName: "openai", ModelName: "gpt-4"}
	fallbackCallConfig := codersdk.ChatModelCallConfig{}

	t.Run("ZeroMaxUsesDefaultsToMaxChatSteps", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &advisorOverrideStubStore{}
		p := newAdvisorTestServer(ctx, t, store)

		rt := p.newAdvisorRuntimeOrFallback(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{
				Enabled:         true,
				MaxUsesPerRun:   0,
				MaxOutputTokens: 16384,
			},
			fallbackModel,
			fallbackCallConfig,
			modelBuildOptions{},
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

		rt := p.newAdvisorRuntimeOrFallback(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{
				Enabled:         true,
				MaxUsesPerRun:   -1,
				MaxOutputTokens: 16384,
			},
			fallbackModel,
			fallbackCallConfig,
			modelBuildOptions{},
			logger,
		)
		require.Nil(t, rt, "negative max uses must disable the advisor")
	})

	t.Run("ZeroMaxOutputTokensDefaults", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &advisorOverrideStubStore{}
		p := newAdvisorTestServer(ctx, t, store)

		rt := p.newAdvisorRuntimeOrFallback(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{
				Enabled:         true,
				MaxUsesPerRun:   3,
				MaxOutputTokens: 0,
			},
			fallbackModel,
			fallbackCallConfig,
			modelBuildOptions{},
			logger,
		)
		require.NotNil(t, rt,
			"zero max output tokens must default to defaultAdvisorMaxOutputTokens, not disable the advisor")
		require.Equal(t, 3, rt.RemainingUses())
		require.Equal(t, int64(defaultAdvisorMaxOutputTokens), rt.MaxOutputTokens(),
			"zero max output tokens must be replaced with defaultAdvisorMaxOutputTokens")
	})

	t.Run("AppliesReasoningEffortToProviderOptions", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &advisorOverrideStubStore{}
		p := newAdvisorTestServer(ctx, t, store)

		rt := p.newAdvisorRuntimeOrFallback(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{
				Enabled:         true,
				MaxUsesPerRun:   3,
				MaxOutputTokens: 16384,
			},
			fallbackModel,
			codersdk.ChatModelCallConfig{
				ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{
					Default: ptr.Ref(codersdk.ChatModelReasoningEffortHigh),
					Max:     ptr.Ref(codersdk.ChatModelReasoningEffortXHigh),
				},
				ProviderOptions: &codersdk.ChatModelProviderOptions{
					OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
						User: ptr.Ref("advisor-user"),
					},
				},
			},
			modelBuildOptions{},
			logger,
		)
		require.NotNil(t, rt)
		providerOptions := rt.ProviderOptions()[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
		require.Equal(t, "advisor-user", *providerOptions.User)
		require.Equal(t, fantasyopenai.ReasoningEffortHigh, *providerOptions.ReasoningEffort)
	})
}
