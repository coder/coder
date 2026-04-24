package chatd //nolint:testpackage // Accesses unexported advisor helpers.

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
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

// advisorOverrideStubStore stubs only the database methods that
// resolveAdvisorModelOverride exercises. The prod code calls
// GetEnabledChatModelConfigByID so the query joins chat_providers and
// filters both enabled flags atomically; tests simulate that by returning
// configs the stub treats as enabled.
type advisorOverrideStubStore struct {
	database.Store

	getEnabledChatModelConfigByID func(context.Context, uuid.UUID) (database.ChatModelConfig, error)
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
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
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
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
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
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
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
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
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
		// Guard against ModelFromConfig silently ignoring the model field
		// and returning a default. The override is only useful if the
		// model name from the config row actually propagates.
		require.Equal(t, "gpt-5.2", gotModel.Model())
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
		require.Equal(t, int64(defaultAdvisorMaxOutputTokens), rt.MaxOutputTokens(),
			"zero max output tokens must be replaced with defaultAdvisorMaxOutputTokens")
	})

	// Guards the wiring from AdvisorConfig.ReasoningEffort through
	// newAdvisorRuntime to ApplyReasoningEffortToOptions. A field swap,
	// typo, or accidental deletion of the apply call would otherwise
	// ship silently because chatprovider_test only covers the helper in
	// isolation.
	t.Run("ReasoningEffortReachesProviderOptions", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &advisorOverrideStubStore{}
		p := newAdvisorTestServer(ctx, t, store)

		openAIModel := &chattest.FakeModel{
			ProviderName: fantasyopenai.Name,
			ModelName:    "gpt-4",
		}

		rt := p.newAdvisorRuntime(
			ctx,
			database.Chat{},
			codersdk.AdvisorConfig{
				Enabled:         true,
				MaxUsesPerRun:   3,
				MaxOutputTokens: 16384,
				ReasoningEffort: "high",
			},
			openAIModel,
			fallbackCallConfig,
			chatprovider.ProviderAPIKeys{},
			logger,
		)
		require.NotNil(t, rt)

		providerOptions := rt.ProviderOptions()
		require.NotNil(t, providerOptions,
			"advisor runtime must seed provider options when reasoning effort is set")
		opts, ok := providerOptions[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
		require.True(t, ok,
			"expected *ResponsesProviderOptions for Responses model, got %T",
			providerOptions[fantasyopenai.Name])
		require.NotNil(t, opts.ReasoningEffort,
			"ReasoningEffort from AdvisorConfig must reach the provider options")
		require.Equal(t, fantasyopenai.ReasoningEffortHigh, *opts.ReasoningEffort)
	})
}
