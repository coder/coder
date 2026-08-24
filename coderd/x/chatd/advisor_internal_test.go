package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatadvisor"
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

	getChatOrganizationModelOverride func(context.Context, database.GetChatOrganizationModelOverrideParams) (database.ChatOrganizationModelOverride, error)
	advisorModelConfigID             uuid.UUID
	advisorReasoningEffort           *string
	getEnabledChatModelConfigByID    func(context.Context, uuid.UUID) (database.ChatModelConfig, error)
	getChatModelConfigByID           func(context.Context, uuid.UUID) (database.ChatModelConfig, error)
	getAIProviderByID                func(context.Context, uuid.UUID) (database.AIProvider, error)
	getAIProviders                   func(context.Context, database.GetAIProvidersParams) ([]database.AIProvider, error)
	getAIProviderKeysByProviderID    func(context.Context, uuid.UUID) ([]database.AIProviderKey, error)
	getAIProviderKeysByProviderIDs   func(context.Context, []uuid.UUID) ([]database.AIProviderKey, error)
}

func (s *advisorOverrideStubStore) GetChatOrganizationModelOverride(
	ctx context.Context,
	params database.GetChatOrganizationModelOverrideParams,
) (database.ChatOrganizationModelOverride, error) {
	if s.getChatOrganizationModelOverride != nil {
		return s.getChatOrganizationModelOverride(ctx, params)
	}
	if s.advisorModelConfigID == uuid.Nil {
		return database.ChatOrganizationModelOverride{}, sql.ErrNoRows
	}
	override := database.ChatOrganizationModelOverride{
		OrganizationID: params.OrganizationID,
		Context:        params.Context,
		ModelConfigID:  s.advisorModelConfigID,
	}
	if s.advisorReasoningEffort != nil {
		override.ReasoningEffort = sql.NullString{String: *s.advisorReasoningEffort, Valid: true}
	}
	return override, nil
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

func (s *advisorOverrideStubStore) GetChatModelConfigByID(
	ctx context.Context,
	id uuid.UUID,
) (database.ChatModelConfig, error) {
	if s.getChatModelConfigByID == nil {
		return database.ChatModelConfig{}, xerrors.New("unexpected GetChatModelConfigByID call")
	}
	return s.getChatModelConfigByID(ctx, id)
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

const advisorTestMaxOutputTokens = int64(16384)

func resolveAdvisorModelOverrideForTest(
	ctx context.Context,
	p *Server,
	chat database.Chat,
	modelConfigID uuid.UUID,
	reasoningEffort *string,
	maxOutputTokens int64,
	modelOpts modelBuildOptions,
	logger slog.Logger,
) (resolvedModelCall, bool, error) {
	if store, ok := p.db.(*advisorOverrideStubStore); ok {
		store.advisorModelConfigID = modelConfigID
		store.advisorReasoningEffort = reasoningEffort
	}
	return p.resolveAdvisorModelOverride(ctx, chat, maxOutputTokens, modelOpts, logger)
}

// advisorChatModelFixture wires a chat whose LastModelConfigID resolves
// through the config cache to an enabled, provider-linked model config, so
// the advisor chat-model path can resolve without an override.
func advisorChatModelFixture(t *testing.T, options json.RawMessage) (database.Chat, *advisorOverrideStubStore) {
	t.Helper()
	configID := uuid.New()
	providerID := uuid.New()
	organizationID := uuid.New()
	store := &advisorOverrideStubStore{
		getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
			return database.ChatModelConfig{
				ID:             configID,
				Model:          "gpt-5.2",
				Enabled:        true,
				Options:        options,
				DisplayName:    "gpt-5.2",
				AIProviderID:   uuid.NullUUID{UUID: providerID, Valid: true},
				OrganizationID: organizationID,
			}, nil
		},
		getAIProviderByID: func(context.Context, uuid.UUID) (database.AIProvider, error) {
			return aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil
		},
	}
	return database.Chat{LastModelConfigID: configID, OrganizationID: organizationID}, store
}

func advisorTestTransportFactory() *aibridgeTestFactory {
	return &aibridgeTestFactory{rt: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})}
}

// TestResolveAdvisorModelOverride covers the early-return, each
// use-the-chat-model branch, and the success path.
func TestResolveAdvisorModelOverride(t *testing.T) {
	t.Parallel()

	logger := slog.Make()

	requireChatModel := func(t *testing.T, p *Server, modelConfigID uuid.UUID) {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitShort)
		resolved, ok, err := resolveAdvisorModelOverrideForTest(ctx,
			p,
			database.Chat{},
			modelConfigID,
			nil,
			advisorTestMaxOutputTokens,
			modelBuildOptions{},
			logger,
		)
		require.NoError(t, err)
		require.False(t, ok, "unusable override must fall back to the chat model")
		require.False(t, resolved.model.Valid())
	}

	t.Run("NilModelConfigUsesChatModel", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		// Error if the store is consulted; the early return must skip it.
		store := &advisorOverrideStubStore{}
		p := newAdvisorTestServer(ctx, t, store)

		requireChatModel(t, p, uuid.Nil)
	})

	t.Run("ConfigLookupErrorUsesChatModel", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &advisorOverrideStubStore{
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{}, xerrors.New("lookup failed")
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		requireChatModel(t, p, uuid.New())
	})

	// Covers the sql.ErrNoRows branch separately from the generic-error
	// branch above. GetEnabledChatModelConfigByID returns ErrNoRows when
	// an admin disables the advisor model or its provider, and that case
	// has a distinct log message. Without this test, removing the
	// errors.Is(err, sql.ErrNoRows) check would still pass the sibling
	// test.
	t.Run("DisabledProviderUsesChatModel", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &advisorOverrideStubStore{
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{}, sql.ErrNoRows
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		requireChatModel(t, p, uuid.New())
	})

	t.Run("ForeignOrgConfigUsesChatModel", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		configID := uuid.New()
		foreignOrgID := uuid.New()
		chatOrgID := uuid.New()
		store := &advisorOverrideStubStore{
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{
					ID:             configID,
					OrganizationID: foreignOrgID,
					Enabled:        true,
				}, nil
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		resolved, ok, err := resolveAdvisorModelOverrideForTest(ctx,
			p,
			database.Chat{OrganizationID: chatOrgID},
			configID,
			nil,
			advisorTestMaxOutputTokens,
			modelBuildOptions{},
			logger,
		)
		require.NoError(t, err)
		require.False(t, ok)
		require.False(t, resolved.model.Valid())
	})

	t.Run("InvalidOptionsJSONUsesChatModel", func(t *testing.T) {
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

		requireChatModel(t, p, configID)
	})

	t.Run("InvalidOptionsJSONWithLinkedProviderUsesChatModel", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		configID := uuid.New()
		store := &advisorOverrideStubStore{
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{
					ID:           configID,
					Model:        "gpt-5.2",
					Enabled:      true,
					Options:      []byte("not valid json"),
					DisplayName:  "gpt-5.2",
					AIProviderID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
				}, nil
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		requireChatModel(t, p, configID)
	})

	t.Run("MissingProviderKeyUsesChatModel", func(t *testing.T) {
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

		requireChatModel(t, p, configID)
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
		p.aibridgeTransportFactory = aibridgeTestFactoryPointer(advisorTestTransportFactory())

		resolved, ok, err := resolveAdvisorModelOverrideForTest(ctx,
			p,
			database.Chat{},
			configID,
			ptr.Ref(codersdk.ChatModelReasoningEffortHigh),
			advisorTestMaxOutputTokens,
			modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
			logger,
		)
		require.NoError(t, err)
		require.True(t, ok)
		require.True(t, resolved.model.Valid())
		require.Equal(t, "openai", resolved.model.Provider())
		// Guard against ModelFromConfig silently ignoring the model field
		// and returning a default. The override is only useful if the
		// model name from the config row actually propagates.
		require.Equal(t, "gpt-5.2", resolved.model.ModelID())
		require.NotNil(t, resolved.callConfig.Temperature)
		require.InDelta(t, 0.42, *resolved.callConfig.Temperature, 1e-9)
		// The resolver derives provider options from the advisor's
		// requested effort instead of pinning it into the call config,
		// so the config's own effort bounds stay intact.
		require.NotNil(t, resolved.callConfig.ReasoningEffort)
		require.Equal(t, codersdk.ChatModelReasoningEffortLow, *resolved.callConfig.ReasoningEffort.Default)
		require.Equal(t, codersdk.ChatModelReasoningEffortXHigh, *resolved.callConfig.ReasoningEffort.Max)
		require.Equal(t, ptr.Ref(advisorTestMaxOutputTokens), resolved.callConfig.MaxOutputTokens)
		requireOpenAIReasoningEffort(t, resolved.providerOptions, codersdk.ChatModelReasoningEffortHigh)
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
		p.aibridgeTransportFactory = aibridgeTestFactoryPointer(advisorTestTransportFactory())

		resolved, ok, err := resolveAdvisorModelOverrideForTest(ctx,
			p,
			database.Chat{},
			configID,
			nil,
			advisorTestMaxOutputTokens,
			modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
			logger,
		)
		require.NoError(t, err)
		require.True(t, ok)
		require.True(t, resolved.model.Valid())
		require.Equal(t, "openai", resolved.model.Provider())
		require.Equal(t, "gpt-5.2", resolved.model.ModelID())
		require.Equal(t, codersdk.ChatModelCallConfig{
			MaxOutputTokens: ptr.Ref(advisorTestMaxOutputTokens),
		}, resolved.callConfig)
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
	resolved, ok, err := resolveAdvisorModelOverrideForTest(ctx,
		p,
		database.Chat{ID: uuid.New()},
		configID,
		nil,
		advisorTestMaxOutputTokens,
		modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
		slog.Make(),
	)
	require.ErrorContains(t, err, "AI Gateway transport factory")
	require.False(t, ok)
	require.False(t, resolved.model.Valid())
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

// TestNewAdvisorRuntime covers the defensive branches in newAdvisorRuntime
// that gate whether the runtime is created and with what bounds, plus the
// chat-model path that resolves the advisor call when no override is
// configured. Without this coverage a regression in any branch ships
// silently.
func TestNewAdvisorRuntime(t *testing.T) {
	t.Parallel()

	logger := slog.Make()

	newChatModelRuntime := func(t *testing.T, advisorCfg advisorRuntimeConfig, options json.RawMessage) *chatadvisor.Runtime {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitShort)
		chat, store := advisorChatModelFixture(t, options)
		p := newAdvisorTestServer(ctx, t, store)
		p.aibridgeTransportFactory = aibridgeTestFactoryPointer(advisorTestTransportFactory())

		rt, err := p.newAdvisorRuntime(
			ctx,
			chat,
			advisorCfg,
			modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
			logger,
		)
		require.NoError(t, err)
		return rt
	}

	t.Run("ZeroMaxUsesDefaultsToMaxChatSteps", func(t *testing.T) {
		t.Parallel()

		rt := newChatModelRuntime(t, advisorRuntimeConfig{
			Enabled:         true,
			MaxUsesPerRun:   0,
			MaxOutputTokens: 16384,
		}, nil)
		require.NotNil(t, rt, "zero max uses must default rather than bail out")
		require.Equal(t, maxChatSteps, rt.RemainingUses(),
			"zero max uses must be replaced with maxChatSteps")
	})

	t.Run("NegativeMaxUsesReturnsNil", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		// Error if any resolution is attempted; the bounds check must
		// disable the advisor before model resolution starts.
		store := &advisorOverrideStubStore{}
		p := newAdvisorTestServer(ctx, t, store)

		rt, err := p.newAdvisorRuntime(
			ctx,
			database.Chat{},
			advisorRuntimeConfig{
				Enabled:         true,
				MaxUsesPerRun:   -1,
				MaxOutputTokens: 16384,
			},
			modelBuildOptions{},
			logger,
		)
		require.NoError(t, err)
		require.Nil(t, rt, "negative max uses must disable the advisor")
	})

	t.Run("ZeroMaxOutputTokensDefaults", func(t *testing.T) {
		t.Parallel()

		rt := newChatModelRuntime(t, advisorRuntimeConfig{
			Enabled:         true,
			MaxUsesPerRun:   3,
			MaxOutputTokens: 0,
		}, nil)
		require.NotNil(t, rt,
			"zero max output tokens must default to defaultAdvisorMaxOutputTokens, not disable the advisor")
		require.Equal(t, 3, rt.RemainingUses())
		require.Equal(t, int64(defaultAdvisorMaxOutputTokens), rt.MaxOutputTokens(),
			"zero max output tokens must be replaced with defaultAdvisorMaxOutputTokens")
	})

	t.Run("ChatModelResolutionFailureSkipsAdvisor", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &advisorOverrideStubStore{
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{}, xerrors.New("lookup failed")
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		rt, err := p.newAdvisorRuntime(
			ctx,
			database.Chat{LastModelConfigID: uuid.New()},
			advisorRuntimeConfig{
				Enabled:         true,
				MaxUsesPerRun:   3,
				MaxOutputTokens: 16384,
			},
			modelBuildOptions{},
			logger,
		)
		require.NoError(t, err, "a failed chat-model resolution must skip the advisor, not fail the turn")
		require.Nil(t, rt)
	})

	t.Run("AppliesChatModelEffortToProviderOptions", func(t *testing.T) {
		t.Parallel()

		options, err := json.Marshal(codersdk.ChatModelCallConfig{
			ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{
				Default: ptr.Ref(codersdk.ChatModelReasoningEffortHigh),
				Max:     ptr.Ref(codersdk.ChatModelReasoningEffortXHigh),
			},
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
					User: ptr.Ref("advisor-user"),
				},
			},
		})
		require.NoError(t, err)

		rt := newChatModelRuntime(t, advisorRuntimeConfig{
			Enabled:         true,
			MaxUsesPerRun:   3,
			MaxOutputTokens: 16384,
		}, options)
		require.NotNil(t, rt)
		requireOpenAIUserOption(t, rt.ProviderOptions(), "advisor-user")
		requireOpenAIReasoningEffort(t, rt.ProviderOptions(), codersdk.ChatModelReasoningEffortHigh)
	})
}
