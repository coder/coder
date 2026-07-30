package chatd

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestResolveConfiguredModelOverride_CrossOrgTreatedAsUnavailable pins the
// org-isolation invariant in both failure modes: a stored override naming
// another organization's config resolves exactly like one naming a missing
// row (soft mode falls back, hard mode errors), never silently uses the
// cross-org model.
func TestResolveConfiguredModelOverride_CrossOrgTreatedAsUnavailable(t *testing.T) {
	t.Parallel()

	logger := slog.Make()
	chatOrgID := uuid.New()
	crossOrgConfig := database.ChatModelConfig{
		ID:             uuid.New(),
		Model:          "gpt-5.2",
		Enabled:        true,
		OrganizationID: uuid.New(),
	}

	server := &Server{logger: logger}
	resolveModelConfig := func(_ context.Context, id uuid.UUID) (database.ChatModelConfig, string, error) {
		require.Equal(t, crossOrgConfig.ID, id)
		return crossOrgConfig, "openai", nil
	}
	resolveKeys := func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (chatprovider.ProviderAPIKeys, error) {
		t.Fatal("provider keys must not be resolved for a cross-org override")
		return chatprovider.ProviderAPIKeys{}, nil
	}

	t.Run("SoftModeFallsBack", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		_, _, _, ok, err := server.resolveConfiguredModelOverride(
			ctx,
			"general",
			crossOrgConfig.ID.String(),
			chatOrgID,
			uuid.New(),
			resolveModelConfig,
			resolveKeys,
			modelOverrideFailureModeSoft,
		)
		require.NoError(t, err)
		require.False(t, ok, "cross-org override must not resolve in soft mode")
	})

	t.Run("HardModeErrorsAsUnavailable", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		_, _, _, ok, err := server.resolveConfiguredModelOverride(
			ctx,
			"title generation",
			crossOrgConfig.ID.String(),
			chatOrgID,
			uuid.New(),
			resolveModelConfig,
			resolveKeys,
			modelOverrideFailureModeHard,
		)
		require.ErrorContains(t, err, "is unavailable")
		require.True(t, ok, "hard mode marks configured-but-unusable overrides as set")
	})
}

// TestResolveAdvisorModelOverride_CrossOrgFallsBack pins the advisor side of
// the invariant through the real resolution path: an advisor override naming
// a config outside the chat's org falls back to the chat model. The stub is
// fully resolvable (provider, keys, transport), so without the same-org
// guard the override would succeed: this fails if the guard is removed.
func TestResolveAdvisorModelOverride_CrossOrgFallsBack(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatOrgID := uuid.New()
	configID := uuid.New()
	providerID := uuid.New()
	store := &advisorOverrideStubStore{
		getChatAdvisorModelOverride: func(context.Context, string) (string, error) {
			return configID.String(), nil
		},
		getEnabledChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
			return database.ChatModelConfig{
				ID:             configID,
				Model:          "gpt-5.2",
				Enabled:        true,
				OrganizationID: uuid.New(), // not the chat's org
				AIProviderID:   uuid.NullUUID{UUID: providerID, Valid: true},
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

	fallbackModel := &chattest.FakeModel{ProviderName: "stub", ModelName: "stub"}
	fallbackCallConfig := codersdk.ChatModelCallConfig{}
	gotModel, gotCfg := p.resolveAdvisorModelOverrideOrFallback(
		ctx,
		database.Chat{OrganizationID: chatOrgID},
		fallbackModel,
		fallbackCallConfig,
		modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
		slog.Make(),
	)
	require.Equal(t, fallbackModel, gotModel)
	require.Equal(t, fallbackCallConfig, gotCfg)
}
