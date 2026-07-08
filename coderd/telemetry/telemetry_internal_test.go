package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestCollectAgentVirtualDesktop(t *testing.T) {
	t.Parallel()

	collect := func(t *testing.T, opts Options) agentVirtualDesktopTelemetry {
		t.Helper()
		var payload agentVirtualDesktopTelemetry
		require.NoError(t, json.Unmarshal(collectAgentVirtualDesktop(context.Background(), opts), &payload))
		return payload
	}

	t.Run("Default", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatComputerUseProvider(gomock.Any()).Return("", nil)

		payload := collect(t, Options{Database: db, Logger: testutil.Logger(t)})
		require.False(t, payload.Enabled)
		require.EqualValues(t, codersdk.ChatComputerUseProviderAnthropic, payload.ComputerUse.Provider)
		require.Equal(t, "default", payload.ComputerUse.ProviderSource)
	})

	t.Run("Configured", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatComputerUseProvider(gomock.Any()).Return("openai", nil)

		payload := collect(t, Options{
			Database:    db,
			Logger:      testutil.Logger(t),
			Experiments: codersdk.Experiments{codersdk.ExperimentChatVirtualDesktop},
		})
		require.True(t, payload.Enabled)
		require.Equal(t, "openai", payload.ComputerUse.Provider)
		require.Equal(t, "configured", payload.ComputerUse.ProviderSource)
	})

	t.Run("QueryError", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatComputerUseProvider(gomock.Any()).Return("", sql.ErrConnDone)

		payload := collect(t, Options{Database: db, Logger: testutil.Logger(t)})
		require.Equal(t, agentExperimentUnknown, payload.ComputerUse.Provider)
		require.Equal(t, agentExperimentUnknown, payload.ComputerUse.ProviderSource)
	})
}

func TestCollectAgentAdvisor(t *testing.T) {
	t.Parallel()

	collect := func(t *testing.T, opts Options) agentAdvisorTelemetry {
		t.Helper()
		var payload agentAdvisorTelemetry
		require.NoError(t, json.Unmarshal(collectAgentAdvisor(context.Background(), opts), &payload))
		return payload
	}
	marshalConfig := func(t *testing.T, cfg codersdk.AdvisorConfig) string {
		t.Helper()
		raw, err := json.Marshal(cfg)
		require.NoError(t, err)
		return string(raw)
	}

	t.Run("ReuseChatModel", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).
			Return(marshalConfig(t, codersdk.AdvisorConfig{}), nil)

		payload := collect(t, Options{Database: db, Logger: testutil.Logger(t)})
		require.False(t, payload.Enabled)
		require.Zero(t, payload.MaxUsesPerRun)
		require.Zero(t, payload.MaxOutputTokens)
		require.Equal(t, advisorModelReuseChatModel, payload.Provider)
		require.Equal(t, advisorModelReuseChatModel, payload.Model)
	})

	t.Run("ModelOverride", func(t *testing.T) {
		t.Parallel()

		modelID := uuid.New()
		providerID := uuid.New()
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).Return(marshalConfig(t, codersdk.AdvisorConfig{
			Enabled:         true,
			MaxUsesPerRun:   7,
			MaxOutputTokens: 2048,
			ModelConfigID:   modelID,
		}), nil)
		db.EXPECT().GetEnabledChatModelConfigByID(gomock.Any(), modelID).Return(database.ChatModelConfig{
			Model:        "gpt-6-preview",
			AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
		}, nil)
		db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(database.AIProvider{
			Type: database.AIProviderTypeOpenai,
		}, nil)

		payload := collect(t, Options{Database: db, Logger: testutil.Logger(t)})
		// Stored enabled is ignored; the chat-advisor experiment gates it.
		require.False(t, payload.Enabled)
		require.Equal(t, 7, payload.MaxUsesPerRun)
		require.Equal(t, int64(2048), payload.MaxOutputTokens)
		require.Equal(t, string(database.AIProviderTypeOpenai), payload.Provider)
		require.Equal(t, "gpt-6-preview", payload.Model)
	})

	t.Run("ExperimentEnabled", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).
			Return(marshalConfig(t, codersdk.AdvisorConfig{}), nil)

		payload := collect(t, Options{
			Database:    db,
			Logger:      testutil.Logger(t),
			Experiments: codersdk.Experiments{codersdk.ExperimentChatAdvisor},
		})
		require.True(t, payload.Enabled)
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).Return("not-json", nil)

		payload := collect(t, Options{Database: db, Logger: testutil.Logger(t)})
		require.Equal(t, agentExperimentUnknown, payload.Provider)
		require.Equal(t, agentExperimentUnknown, payload.Model)
	})

	t.Run("PartialParse", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).
			Return(`{"max_uses_per_run": 42, "model_config_id": "not-a-uuid"}`, nil)

		payload := collect(t, Options{Database: db, Logger: testutil.Logger(t)})
		require.Zero(t, payload.MaxUsesPerRun)
		require.Zero(t, payload.MaxOutputTokens)
		require.Equal(t, agentExperimentUnknown, payload.Provider)
		require.Equal(t, agentExperimentUnknown, payload.Model)
	})

	t.Run("InactiveModelConfig", func(t *testing.T) {
		t.Parallel()

		modelID := uuid.New()
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).
			Return(marshalConfig(t, codersdk.AdvisorConfig{ModelConfigID: modelID}), nil)
		db.EXPECT().GetEnabledChatModelConfigByID(gomock.Any(), modelID).
			Return(database.ChatModelConfig{}, sql.ErrNoRows)

		payload := collect(t, Options{Database: db, Logger: testutil.Logger(t)})
		require.Equal(t, advisorModelReuseChatModel, payload.Provider)
		require.Equal(t, advisorModelReuseChatModel, payload.Model)
	})

	t.Run("ConfigFetchError", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).Return("", sql.ErrConnDone)

		payload := collect(t, Options{Database: db, Logger: testutil.Logger(t)})
		require.Equal(t, agentExperimentUnknown, payload.Provider)
		require.Equal(t, agentExperimentUnknown, payload.Model)
	})

	t.Run("ModelResolveError", func(t *testing.T) {
		t.Parallel()

		modelID := uuid.New()
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).
			Return(marshalConfig(t, codersdk.AdvisorConfig{ModelConfigID: modelID}), nil)
		db.EXPECT().GetEnabledChatModelConfigByID(gomock.Any(), modelID).
			Return(database.ChatModelConfig{}, sql.ErrConnDone)

		payload := collect(t, Options{Database: db, Logger: testutil.Logger(t)})
		require.Equal(t, agentExperimentUnknown, payload.Provider)
		require.Equal(t, agentExperimentUnknown, payload.Model)
	})

	t.Run("ProviderResolveError", func(t *testing.T) {
		t.Parallel()

		modelID := uuid.New()
		providerID := uuid.New()
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).
			Return(marshalConfig(t, codersdk.AdvisorConfig{ModelConfigID: modelID}), nil)
		db.EXPECT().GetEnabledChatModelConfigByID(gomock.Any(), modelID).Return(database.ChatModelConfig{
			Model:        "gpt-6-preview",
			AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
		}, nil)
		db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).
			Return(database.AIProvider{}, sql.ErrConnDone)

		payload := collect(t, Options{Database: db, Logger: testutil.Logger(t)})
		// The provider is unknown, but the already-resolved model still ships.
		require.Equal(t, agentExperimentUnknown, payload.Provider)
		require.Equal(t, "gpt-6-preview", payload.Model)
	})
}
