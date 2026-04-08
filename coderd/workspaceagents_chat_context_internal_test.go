package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

func insertAgentChatTestModelConfig(
	ctx context.Context,
	t testing.TB,
	db database.Store,
	userID uuid.UUID,
) database.ChatModelConfig {
	t.Helper()

	sysCtx := dbauthz.AsSystemRestricted(ctx)
	createdBy := uuid.NullUUID{UUID: userID, Valid: true}

	_, err := db.InsertChatProvider(sysCtx, database.InsertChatProviderParams{
		Provider:             "openai",
		DisplayName:          "OpenAI",
		APIKey:               "test-api-key",
		ApiKeyKeyID:          sql.NullString{},
		CreatedBy:            createdBy,
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})
	require.NoError(t, err)

	model, err := db.InsertChatModelConfig(sysCtx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                "gpt-4o-mini",
		DisplayName:          "Test Model",
		CreatedBy:            createdBy,
		UpdatedBy:            createdBy,
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 70,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	return model
}
