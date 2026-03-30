package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
)

// TestDeleteProvider_CascadesUnboundModelConfigs verifies that
// deleting a provider cascade-deletes model configs that reference
// it by provider family, even when provider_config_id is NULL.
//
// Migration 000456 drops the old text-based FK and adds a new
// UUID-based FK on provider_config_id, but does not backfill
// existing rows. Without a backfill, rows with NULL
// provider_config_id lose cascade protection entirely.
func TestDeleteProvider_CascadesUnboundModelConfigs(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Requires a real Postgres database")
	}

	store, _ := dbtestutil.NewDB(t)
	ctx := context.Background()

	user := dbgen.User(t, store, database.User{})

	provider, err := store.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:    "openai",
		DisplayName: "OpenAI",
		APIKey:      "test-key",
		BaseUrl:     "",
		ApiKeyKeyID: sql.NullString{},
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:     true,
	})
	require.NoError(t, err)

	// Insert a model config with matching provider family text but
	// provider_config_id left as NULL. This simulates a row that
	// existed before migration 456 (no backfill) or one created
	// through the API without an explicit provider config binding.
	unboundModel, err := store.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                "gpt-4o-mini",
		DisplayName:          "Unbound Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
		ProviderConfigID:     uuid.NullUUID{}, // NULL — no binding
	})
	require.NoError(t, err)

	// Sanity check.
	_, err = store.GetChatModelConfigByID(ctx, unboundModel.ID)
	require.NoError(t, err, "model config should exist before provider deletion")

	// Delete the provider.
	err = store.DeleteChatProviderByID(ctx, provider.ID)
	require.NoError(t, err)

	// The model config should be gone. Before the old text-based FK
	// was dropped this would have cascaded. After migration 456 the
	// only cascade path is through provider_config_id, which is NULL
	// here, so this assertion will FAIL until a backfill is added.
	_, err = store.GetChatModelConfigByID(ctx, unboundModel.ID)
	require.Error(t, err, "unbound model config should be cascade-deleted when its provider is removed")
}

// TestDeleteProvider_CascadesBoundModelConfigs is the control case:
// when provider_config_id IS set, the new FK cascade works correctly.
func TestDeleteProvider_CascadesBoundModelConfigs(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Requires a real Postgres database")
	}

	store, _ := dbtestutil.NewDB(t)
	ctx := context.Background()

	user := dbgen.User(t, store, database.User{})

	provider, err := store.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:    "openai",
		DisplayName: "OpenAI",
		APIKey:      "test-key",
		BaseUrl:     "",
		ApiKeyKeyID: sql.NullString{},
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:     true,
	})
	require.NoError(t, err)

	boundModel, err := store.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                "gpt-4o-mini",
		DisplayName:          "Bound Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
		ProviderConfigID:     uuid.NullUUID{UUID: provider.ID, Valid: true},
	})
	require.NoError(t, err)

	_, err = store.GetChatModelConfigByID(ctx, boundModel.ID)
	require.NoError(t, err, "bound model config should exist before provider deletion")

	err = store.DeleteChatProviderByID(ctx, provider.ID)
	require.NoError(t, err)

	_, err = store.GetChatModelConfigByID(ctx, boundModel.ID)
	require.Error(t, err, "bound model config should be cascade-deleted when provider is deleted")
}
