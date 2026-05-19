package chatd //nolint:testpackage // Accesses unexported compaction helpers.

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/testutil"
)

type compactionOverrideStubStore struct {
	database.Store

	getChatCompactionModelOverride func(context.Context) (string, error)
	getEnabledChatModelConfigByID  func(context.Context, uuid.UUID) (database.ChatModelConfig, error)
}

func (s *compactionOverrideStubStore) GetChatCompactionModelOverride(ctx context.Context) (string, error) {
	if s.getChatCompactionModelOverride == nil {
		return "", xerrors.New("unexpected GetChatCompactionModelOverride call")
	}
	return s.getChatCompactionModelOverride(ctx)
}

func (s *compactionOverrideStubStore) GetEnabledChatModelConfigByID(
	ctx context.Context,
	id uuid.UUID,
) (database.ChatModelConfig, error) {
	if s.getEnabledChatModelConfigByID == nil {
		return database.ChatModelConfig{}, xerrors.New("unexpected GetEnabledChatModelConfigByID call")
	}
	return s.getEnabledChatModelConfigByID(ctx, id)
}

func TestResolveCompactionModelOverride(t *testing.T) {
	t.Parallel()

	fallbackConfigID := uuid.New()
	fallbackModel := &chattest.FakeModel{ProviderName: "stub", ModelName: "stub"}
	fallbackConfig := database.ChatModelConfig{
		ID:                   fallbackConfigID,
		Provider:             "openai",
		Model:                "gpt-4.1",
		Enabled:              true,
		CreatedAt:            time.Unix(0, 0).UTC(),
		UpdatedAt:            time.Unix(0, 0).UTC(),
		DisplayName:          "gpt-4.1",
		ContextLimit:         1_000_000,
		CompressionThreshold: 70,
	}
	logger := slog.Make()

	t.Run("UnsetReturnsFallback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &compactionOverrideStubStore{
			getChatCompactionModelOverride: func(context.Context) (string, error) {
				return "", nil
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		got := p.resolveCompactionModelOverride(
			ctx,
			database.Chat{},
			fallbackModel,
			fallbackConfig,
			chatprovider.ProviderAPIKeys{},
			logger,
		)
		require.Equal(t, fantasy.LanguageModel(fallbackModel), got.model)
		require.Equal(t, fallbackConfigID, got.modelConfigID)
		require.Equal(t, "stub", got.provider)
		require.Equal(t, "stub", got.modelName)
	})

	t.Run("MalformedReturnsFallback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &compactionOverrideStubStore{
			getChatCompactionModelOverride: func(context.Context) (string, error) {
				return "not-a-uuid", nil
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		got := p.resolveCompactionModelOverride(
			ctx,
			database.Chat{},
			fallbackModel,
			fallbackConfig,
			chatprovider.ProviderAPIKeys{},
			logger,
		)
		require.Equal(t, fantasy.LanguageModel(fallbackModel), got.model)
		require.Equal(t, fallbackConfigID, got.modelConfigID)
	})

	t.Run("NilUUIDReturnsFallback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		store := &compactionOverrideStubStore{
			getChatCompactionModelOverride: func(context.Context) (string, error) {
				return uuid.Nil.String(), nil
			},
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				require.FailNow(t, "nil UUID should not query a model config")
				return database.ChatModelConfig{}, nil
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		got := p.resolveCompactionModelOverride(
			ctx,
			database.Chat{},
			fallbackModel,
			fallbackConfig,
			chatprovider.ProviderAPIKeys{},
			logger,
		)
		require.Equal(t, fantasy.LanguageModel(fallbackModel), got.model)
		require.Equal(t, fallbackConfigID, got.modelConfigID)
		require.Equal(t, "stub", got.provider)
		require.Equal(t, "stub", got.modelName)
	})

	t.Run("DisabledModelReturnsFallback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		overrideID := uuid.New()
		store := &compactionOverrideStubStore{
			getChatCompactionModelOverride: func(context.Context) (string, error) {
				return overrideID.String(), nil
			},
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{}, sql.ErrNoRows
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		got := p.resolveCompactionModelOverride(
			ctx,
			database.Chat{},
			fallbackModel,
			fallbackConfig,
			chatprovider.ProviderAPIKeys{OpenAI: "sk-test"},
			logger,
		)
		require.Equal(t, fantasy.LanguageModel(fallbackModel), got.model)
		require.Equal(t, fallbackConfigID, got.modelConfigID)
	})

	t.Run("MissingProviderKeyReturnsFallback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		overrideID := uuid.New()
		store := &compactionOverrideStubStore{
			getChatCompactionModelOverride: func(context.Context) (string, error) {
				return overrideID.String(), nil
			},
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{
					ID:          overrideID,
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

		got := p.resolveCompactionModelOverride(
			ctx,
			database.Chat{},
			fallbackModel,
			fallbackConfig,
			chatprovider.ProviderAPIKeys{},
			logger,
		)
		require.Equal(t, fantasy.LanguageModel(fallbackModel), got.model)
		require.Equal(t, fallbackConfigID, got.modelConfigID)
	})

	t.Run("SuccessReturnsOverrideModelAndMetadata", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		overrideID := uuid.New()
		store := &compactionOverrideStubStore{
			getChatCompactionModelOverride: func(context.Context) (string, error) {
				return overrideID.String(), nil
			},
			getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
				return database.ChatModelConfig{
					ID:           overrideID,
					Provider:     "openai",
					Model:        "gpt-5.2",
					Enabled:      true,
					CreatedAt:    time.Unix(0, 0).UTC(),
					UpdatedAt:    time.Unix(0, 0).UTC(),
					DisplayName:  "gpt-5.2",
					ContextLimit: 128_000,
				}, nil
			},
		}
		p := newAdvisorTestServer(ctx, t, store)

		got := p.resolveCompactionModelOverride(
			ctx,
			database.Chat{},
			fallbackModel,
			fallbackConfig,
			chatprovider.ProviderAPIKeys{OpenAI: "sk-test"},
			logger,
		)
		require.NotEqual(t, fantasy.LanguageModel(fallbackModel), got.model)
		require.NotNil(t, got.model)
		require.Equal(t, overrideID, got.modelConfigID)
		require.Equal(t, "openai", got.provider)
		require.Equal(t, "gpt-5.2", got.modelName)
	})
}
