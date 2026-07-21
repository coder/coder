package database_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
)

func TestMCPServerConfigsOrganizationScope(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, _ := dbtestutil.NewDB(t)
	organizationA := dbgen.Organization(t, db, database.Organization{})
	organizationB := dbgen.Organization(t, db, database.Organization{})

	configA := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: organizationA.ID,
		DisplayName:    "A Config",
		Slug:           "shared-slug",
		Availability:   "force_on",
		Enabled:        true,
	})
	configB := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: organizationB.ID,
		DisplayName:    "B Config",
		Slug:           "shared-slug",
		Availability:   "force_on",
		Enabled:        true,
	})
	disabledA := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: organizationA.ID,
		DisplayName:    "Disabled",
	})
	disabledA, err := db.UpdateMCPServerConfig(ctx, database.UpdateMCPServerConfigParams{
		DisplayName:             disabledA.DisplayName,
		Slug:                    disabledA.Slug,
		Description:             disabledA.Description,
		IconURL:                 disabledA.IconURL,
		Transport:               disabledA.Transport,
		Url:                     disabledA.Url,
		AuthType:                disabledA.AuthType,
		OAuth2ClientID:          disabledA.OAuth2ClientID,
		OAuth2ClientSecret:      disabledA.OAuth2ClientSecret,
		OAuth2ClientSecretKeyID: disabledA.OAuth2ClientSecretKeyID,
		OAuth2AuthURL:           disabledA.OAuth2AuthURL,
		OAuth2TokenURL:          disabledA.OAuth2TokenURL,
		OAuth2RevocationURL:     disabledA.OAuth2RevocationURL,
		OAuth2Scopes:            disabledA.OAuth2Scopes,
		APIKeyHeader:            disabledA.APIKeyHeader,
		APIKeyValue:             disabledA.APIKeyValue,
		APIKeyValueKeyID:        disabledA.APIKeyValueKeyID,
		CustomHeaders:           disabledA.CustomHeaders,
		CustomHeadersKeyID:      disabledA.CustomHeadersKeyID,
		ToolAllowList:           disabledA.ToolAllowList,
		ToolDenyList:            disabledA.ToolDenyList,
		Availability:            disabledA.Availability,
		Enabled:                 false,
		ModelIntent:             disabledA.ModelIntent,
		AllowInPlanMode:         disabledA.AllowInPlanMode,
		ForwardCoderHeaders:     disabledA.ForwardCoderHeaders,
		UpdatedBy:               disabledA.UpdatedBy.UUID,
		ID:                      disabledA.ID,
		OrganizationID:          organizationA.ID,
	})
	require.NoError(t, err)

	configs, err := db.GetMCPServerConfigs(ctx, organizationA.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{configA.ID, disabledA.ID}, mcpServerConfigIDs(configs))

	bySlug, err := db.GetMCPServerConfigBySlug(ctx, database.GetMCPServerConfigBySlugParams{
		OrganizationID: organizationA.ID,
		Slug:           "shared-slug",
	})
	require.NoError(t, err)
	require.Equal(t, configA.ID, bySlug.ID)

	byIDs, err := db.GetMCPServerConfigsByIDs(ctx, database.GetMCPServerConfigsByIDsParams{
		OrganizationID: organizationA.ID,
		IDs:            []uuid.UUID{configA.ID, configB.ID},
	})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{configA.ID}, mcpServerConfigIDs(byIDs))

	enabled, err := db.GetEnabledMCPServerConfigs(ctx, organizationA.ID)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{configA.ID}, mcpServerConfigIDs(enabled))

	forced, err := db.GetForcedMCPServerConfigs(ctx, organizationA.ID)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{configA.ID}, mcpServerConfigIDs(forced))

	user := dbgen.User(t, db, database.User{})
	_, err = db.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
		MCPServerConfigID: configA.ID,
		UserID:            user.ID,
		AccessToken:       "a",
		RefreshToken:      "a-refresh",
		TokenType:         "Bearer",
	})
	require.NoError(t, err)
	_, err = db.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
		MCPServerConfigID: configB.ID,
		UserID:            user.ID,
		AccessToken:       "b",
		RefreshToken:      "b-refresh",
		TokenType:         "Bearer",
	})
	require.NoError(t, err)

	tokens, err := db.GetMCPServerUserTokensByUserID(ctx, database.GetMCPServerUserTokensByUserIDParams{
		UserID:             user.ID,
		OrganizationID:     organizationA.ID,
		McpServerConfigIds: []uuid.UUID{configA.ID, configB.ID},
	})
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.Equal(t, configA.ID, tokens[0].MCPServerConfigID)

	duplicate := database.InsertMCPServerConfigParams{
		OrganizationID: organizationA.ID,
		DisplayName:    "Duplicate",
		Slug:           configA.Slug,
		Transport:      "streamable_http",
		Url:            "https://duplicate.example.com",
		AuthType:       "none",
		APIKeyHeader:   "Authorization",
		CustomHeaders:  "{}",
		ToolAllowList:  []string{},
		ToolDenyList:   []string{},
		Availability:   "default_off",
		CreatedBy:      configA.CreatedBy.UUID,
		UpdatedBy:      configA.UpdatedBy.UUID,
	}
	_, err = db.InsertMCPServerConfig(ctx, duplicate)
	require.True(t, database.IsUniqueViolation(err, database.UniqueMcpServerConfigsOrganizationIDSlugKey), err)

	duplicate.OrganizationID = organizationB.ID
	duplicate.CreatedBy = configB.CreatedBy.UUID
	duplicate.UpdatedBy = configB.UpdatedBy.UUID
	_, err = db.InsertMCPServerConfig(ctx, duplicate)
	require.True(t, database.IsUniqueViolation(err, database.UniqueMcpServerConfigsOrganizationIDSlugKey), err)

	_, err = db.GetMCPServerConfigBySlug(ctx, database.GetMCPServerConfigBySlugParams{
		OrganizationID: uuid.New(),
		Slug:           configA.Slug,
	})
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func mcpServerConfigIDs(configs []database.MCPServerConfig) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(configs))
	for _, config := range configs {
		ids = append(ids, config.ID)
	}
	return ids
}
