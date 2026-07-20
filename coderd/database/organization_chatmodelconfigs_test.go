package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

func TestOrganizationChatModelConfigQueries(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	provider := dbgen.AIProvider(t, store, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
		Name: "organization-models-" + uuid.NewString(),
	})
	firstOrganization := dbgen.Organization(t, store, database.Organization{})
	secondOrganization := dbgen.Organization(t, store, database.Organization{})

	first := insertOrganizationChatModelConfig(ctx, t, store, provider.ID, firstOrganization.ID, "first-"+uuid.NewString(), false)
	second := insertOrganizationChatModelConfig(ctx, t, store, provider.ID, secondOrganization.ID, "second-"+uuid.NewString(), true)

	got, err := store.GetOrganizationChatModelConfigByID(ctx, database.GetOrganizationChatModelConfigByIDParams{
		ID:             first.ID,
		OrganizationID: firstOrganization.ID,
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, got.ID)

	_, err = store.GetOrganizationChatModelConfigByID(ctx, database.GetOrganizationChatModelConfigByIDParams{
		ID:             first.ID,
		OrganizationID: secondOrganization.ID,
	})
	require.ErrorIs(t, err, sql.ErrNoRows)

	firstConfigs, err := store.GetOrganizationChatModelConfigs(ctx, firstOrganization.ID)
	require.NoError(t, err)
	require.Contains(t, chatModelConfigIDs(firstConfigs), first.ID)
	require.NotContains(t, chatModelConfigIDs(firstConfigs), second.ID)

	enabled, err := store.GetOrganizationEnabledChatModelConfigs(ctx, firstOrganization.ID)
	require.NoError(t, err)
	require.Contains(t, enabledChatModelConfigIDs(enabled), first.ID)
	require.NotContains(t, enabledChatModelConfigIDs(enabled), second.ID)

	userID := uuid.NewString()
	groupID := uuid.NewString()
	userACL := database.ChatModelConfigACL{userID: {policy.ActionRead}}
	groupACL := database.ChatModelConfigACL{groupID: {policy.ActionRead}}
	err = store.UpdateOrganizationChatModelConfigACL(ctx, database.UpdateOrganizationChatModelConfigACLParams{
		UserACL:        userACL,
		GroupACL:       groupACL,
		ID:             first.ID,
		OrganizationID: firstOrganization.ID,
	})
	require.NoError(t, err)

	acl, err := store.GetOrganizationChatModelConfigACL(ctx, database.GetOrganizationChatModelConfigACLParams{
		ID:             first.ID,
		OrganizationID: firstOrganization.ID,
	})
	require.NoError(t, err)
	require.Equal(t, userACL, acl.UserACL)
	require.Equal(t, groupACL, acl.GroupACL)

	legacy := dbgen.ChatModelConfig(t, store, database.ChatModelConfig{
		Model:     "legacy-" + uuid.NewString(),
		IsDefault: true,
	})
	legacyRows, err := store.GetChatModelConfigs(ctx)
	require.NoError(t, err)
	require.Contains(t, chatModelConfigIDs(legacyRows), legacy.ID)
	require.NotContains(t, chatModelConfigIDs(legacyRows), first.ID)
	require.NotContains(t, chatModelConfigIDs(legacyRows), second.ID)

	_, err = store.GetChatModelConfigByID(ctx, first.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestAuthorizedOrganizationChatModelConfigQueries(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	provider := dbgen.AIProvider(t, store, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
		Name: "authorized-organization-models-" + uuid.NewString(),
	})
	organization := dbgen.Organization(t, store, database.Organization{})
	otherOrganization := dbgen.Organization(t, store, database.Organization{})
	admin := dbgen.User(t, store, database.User{})
	dbgen.OrganizationMember(t, store, database.OrganizationMember{
		UserID:         admin.ID,
		OrganizationID: organization.ID,
		Roles:          []string{rbac.RoleOrgAdmin()},
	})

	model := insertOrganizationChatModelConfig(ctx, t, store, provider.ID, organization.ID, "authorized-"+uuid.NewString(), false)
	otherModel := insertOrganizationChatModelConfig(ctx, t, store, provider.ID, otherOrganization.ID, "other-"+uuid.NewString(), false)

	subject, _, err := httpmw.UserRBACSubject(ctx, store, admin.ID, rbac.ExpandableScope(rbac.ScopeAll))
	require.NoError(t, err)
	prepared, err := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()).Prepare(ctx, subject, policy.ActionRead, rbac.ResourceChatModelConfig.Type)
	require.NoError(t, err)

	configs, err := store.GetAuthorizedOrganizationChatModelConfigs(ctx, organization.ID, prepared)
	require.NoError(t, err)
	require.Contains(t, chatModelConfigIDs(configs), model.ID)
	require.NotContains(t, chatModelConfigIDs(configs), otherModel.ID)

	enabled, err := store.GetAuthorizedOrganizationEnabledChatModelConfigs(ctx, organization.ID, prepared)
	require.NoError(t, err)
	require.Contains(t, enabledChatModelConfigIDs(enabled), model.ID)
	require.NotContains(t, enabledChatModelConfigIDs(enabled), otherModel.ID)
}

func TestOrganizationChatModelConfigInheritanceQueries(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	firstOrganization := dbgen.Organization(t, store, database.Organization{})
	secondOrganization := dbgen.Organization(t, store, database.Organization{})
	legacy := dbgen.ChatModelConfig(t, store, database.ChatModelConfig{
		Model:       "legacy-" + uuid.NewString(),
		DisplayName: "Legacy model",
		IsDefault:   true,
	})

	err := store.InsertInheritedOrganizationChatModelConfigs(ctx, legacy.ID)
	require.NoError(t, err)

	firstCopy, err := store.GetOrganizationChatModelConfigByLegacyID(ctx, database.GetOrganizationChatModelConfigByLegacyIDParams{
		OrganizationID:      firstOrganization.ID,
		LegacyModelConfigID: legacy.ID,
	})
	require.NoError(t, err)
	secondCopy, err := store.GetOrganizationChatModelConfigByLegacyID(ctx, database.GetOrganizationChatModelConfigByLegacyIDParams{
		OrganizationID:      secondOrganization.ID,
		LegacyModelConfigID: legacy.ID,
	})
	require.NoError(t, err)

	require.True(t, firstCopy.InheritsLegacyConfig)
	require.Equal(t, database.ChatModelConfigACL{}, firstCopy.UserACL)
	require.Equal(t, database.ChatModelConfigACL{
		firstOrganization.ID.String(): {policy.ActionRead},
	}, firstCopy.GroupACL)
	require.NotEqual(t, firstCopy.ID, secondCopy.ID)

	legacy.DisplayName = "Synchronized model"
	legacy = updateLegacyChatModelConfig(ctx, t, store, legacy)
	err = store.SynchronizeInheritedOrganizationChatModelConfigs(ctx, legacy.ID)
	require.NoError(t, err)

	firstCopy, err = store.GetOrganizationChatModelConfigByID(ctx, database.GetOrganizationChatModelConfigByIDParams{
		ID:             firstCopy.ID,
		OrganizationID: firstOrganization.ID,
	})
	require.NoError(t, err)
	require.Equal(t, legacy.DisplayName, firstCopy.DisplayName)

	err = store.DetachOrganizationChatModelConfig(ctx, database.DetachOrganizationChatModelConfigParams{
		ID:             firstCopy.ID,
		OrganizationID: firstOrganization.ID,
	})
	require.NoError(t, err)

	legacy.DisplayName = "Second synchronization"
	legacy = updateLegacyChatModelConfig(ctx, t, store, legacy)
	err = store.SynchronizeInheritedOrganizationChatModelConfigs(ctx, legacy.ID)
	require.NoError(t, err)

	firstCopy, err = store.GetOrganizationChatModelConfigByID(ctx, database.GetOrganizationChatModelConfigByIDParams{
		ID:             firstCopy.ID,
		OrganizationID: firstOrganization.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "Synchronized model", firstCopy.DisplayName)
	require.False(t, firstCopy.InheritsLegacyConfig)

	secondCopy, err = store.GetOrganizationChatModelConfigByID(ctx, database.GetOrganizationChatModelConfigByIDParams{
		ID:             secondCopy.ID,
		OrganizationID: secondOrganization.ID,
	})
	require.NoError(t, err)
	require.Equal(t, legacy.DisplayName, secondCopy.DisplayName)

	err = store.SoftDeleteInheritedOrganizationChatModelConfigs(ctx, legacy.ID)
	require.NoError(t, err)

	_, err = store.GetOrganizationChatModelConfigByID(ctx, database.GetOrganizationChatModelConfigByIDParams{
		ID:             secondCopy.ID,
		OrganizationID: secondOrganization.ID,
	})
	require.ErrorIs(t, err, sql.ErrNoRows)
	lineage, err := store.GetChatModelConfigLineageByID(ctx, secondCopy.ID)
	require.NoError(t, err)
	require.False(t, lineage.InheritsLegacyConfig)

	err = store.InsertInheritedOrganizationChatModelConfigs(ctx, legacy.ID)
	require.NoError(t, err)
	_, err = store.GetOrganizationChatModelConfigByLegacyID(ctx, database.GetOrganizationChatModelConfigByLegacyIDParams{
		OrganizationID:      secondOrganization.ID,
		LegacyModelConfigID: legacy.ID,
	})
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestOrganizationChatModelConfigDefaults(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	firstOrganization := dbgen.Organization(t, store, database.Organization{})
	secondOrganization := dbgen.Organization(t, store, database.Organization{})
	firstLegacy := dbgen.ChatModelConfig(t, store, database.ChatModelConfig{
		Model:     "first-default-" + uuid.NewString(),
		IsDefault: true,
	})
	secondLegacy := dbgen.ChatModelConfig(t, store, database.ChatModelConfig{
		Model: "second-default-" + uuid.NewString(),
	})

	require.NoError(t, store.InsertInheritedOrganizationChatModelConfigs(ctx, firstLegacy.ID))
	require.NoError(t, store.InsertInheritedOrganizationChatModelConfigs(ctx, secondLegacy.ID))
	require.NoError(t, store.SetOrganizationChatModelConfigDefaultInheritance(ctx, database.SetOrganizationChatModelConfigDefaultInheritanceParams{
		OrganizationID:        secondOrganization.ID,
		InheritsLegacyDefault: false,
	}))

	require.NoError(t, store.UnsetDefaultChatModelConfigs(ctx))
	secondLegacy.IsDefault = true
	secondLegacy = updateLegacyChatModelConfig(ctx, t, store, secondLegacy)
	require.NoError(t, store.SynchronizeInheritedOrganizationChatModelConfigDefaults(ctx, secondLegacy.ID))

	firstDefault, err := store.GetOrganizationDefaultChatModelConfig(ctx, firstOrganization.ID)
	require.NoError(t, err)
	require.Equal(t, secondLegacy.ID, firstDefault.LegacyModelConfigID.UUID)
	secondDefault, err := store.GetOrganizationDefaultChatModelConfig(ctx, secondOrganization.ID)
	require.NoError(t, err)
	require.Equal(t, firstLegacy.ID, secondDefault.LegacyModelConfigID.UUID)
}

func TestElectOrganizationDefaultChatModelConfig(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	organization := dbgen.Organization(t, store, database.Organization{})
	disabledProvider := dbgen.AIProvider(t, store, database.AIProvider{
		Type: database.AIProviderTypeAnthropic,
		Name: "disabled-election-" + uuid.NewString(),
	}, func(params *database.InsertAIProviderParams) {
		params.Enabled = false
	})
	enabledProvider := dbgen.AIProvider(t, store, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
		Name: "enabled-election-" + uuid.NewString(),
	})
	disabled := insertOrganizationChatModelConfig(ctx, t, store, disabledProvider.ID, organization.ID, "a-disabled-"+uuid.NewString(), false)
	enabled := insertOrganizationChatModelConfig(ctx, t, store, enabledProvider.ID, organization.ID, "z-enabled-"+uuid.NewString(), false)

	elected, err := store.ElectOrganizationDefaultChatModelConfig(ctx, organization.ID)
	require.NoError(t, err)
	require.Equal(t, enabled.ID, elected.ID)
	require.NotEqual(t, disabled.ID, elected.ID)
}

func insertOrganizationChatModelConfig(
	ctx context.Context,
	t *testing.T,
	store database.Store,
	providerID uuid.UUID,
	organizationID uuid.UUID,
	model string,
	isDefault bool,
) database.ChatModelConfig {
	t.Helper()

	config, err := store.InsertOrganizationChatModelConfig(ctx, database.InsertOrganizationChatModelConfigParams{
		Model:                model,
		DisplayName:          model,
		Enabled:              true,
		IsDefault:            isDefault,
		ContextLimit:         128000,
		CompressionThreshold: 70,
		Options:              json.RawMessage(`{}`),
		AIProviderID:         uuid.NullUUID{UUID: providerID, Valid: true},
		OrganizationID:       organizationID,
		UserACL:              database.ChatModelConfigACL{},
		GroupACL: database.ChatModelConfigACL{
			organizationID.String(): {policy.ActionRead},
		},
	})
	require.NoError(t, err)
	return config
}

func updateLegacyChatModelConfig(ctx context.Context, t *testing.T, store database.Store, config database.ChatModelConfig) database.ChatModelConfig {
	t.Helper()

	updated, err := store.UpdateChatModelConfig(ctx, database.UpdateChatModelConfigParams{
		Model:                config.Model,
		DisplayName:          config.DisplayName,
		UpdatedBy:            config.UpdatedBy,
		Enabled:              config.Enabled,
		IsDefault:            config.IsDefault,
		ContextLimit:         config.ContextLimit,
		CompressionThreshold: config.CompressionThreshold,
		Options:              config.Options,
		AIProviderID:         config.AIProviderID,
		ID:                   config.ID,
	})
	require.NoError(t, err)
	return updated
}

func chatModelConfigIDs(configs []database.ChatModelConfig) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(configs))
	for _, config := range configs {
		ids = append(ids, config.ID)
	}
	return ids
}

func enabledChatModelConfigIDs(configs []database.GetOrganizationEnabledChatModelConfigsRow) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(configs))
	for _, config := range configs {
		ids = append(ids, config.ChatModelConfig.ID)
	}
	return ids
}
