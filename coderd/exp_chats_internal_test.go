package coderd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestChatMCPServerConfigs(t *testing.T) {
	t.Parallel()

	newOrgWithConfig := func(t *testing.T, db database.Store, enabled bool) (database.Organization, database.MCPServerConfig) {
		t.Helper()
		org := dbgen.Organization(t, db, database.Organization{})
		cfg := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID: org.ID,
			Enabled:        enabled,
		})
		return org, cfg
	}

	t.Run("DuplicateIDsDeduped", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		defaultOrg, err := db.GetDefaultOrganization(ctx)
		require.NoError(t, err)

		// The query dedupes in SQL: a duplicated ID returns one row. The
		// handlers compare that count against the raw request length, so
		// duplicate arrays are rejected with an empty missing list, the
		// pre-org-scoping behavior preserved by CODAGT-870's predecessor.
		chatOrg, chatOrgCfg := newOrgWithConfig(t, db, true)
		defaultOrgCfg := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID: defaultOrg.ID,
			Enabled:        true,
		})

		configs, err := chatMCPServerConfigs(ctx, db, chatOrg.ID,
			[]uuid.UUID{defaultOrgCfg.ID, chatOrgCfg.ID, defaultOrgCfg.ID, chatOrgCfg.ID})
		require.NoError(t, err)
		require.Len(t, configs, 2)
	})

	t.Run("DisabledDefaultOrgConfigValidates", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		defaultOrg, err := db.GetDefaultOrganization(ctx)
		require.NoError(t, err)

		// A disabled default-org config still validates: the generation
		// path skips it, as it did before org-scoping.
		chatOrg := dbgen.Organization(t, db, database.Organization{})
		user := dbgen.User(t, db, database.User{})
		disabledCfg, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
			OrganizationID: defaultOrg.ID,
			DisplayName:    "Disabled Default Org MCP Server",
			Slug:           testutil.GetRandomName(t),
			Url:            "https://mcp.example.com",
			Transport:      "streamable_http",
			AuthType:       "none",
			ToolAllowList:  []string{},
			ToolDenyList:   []string{},
			Availability:   "default_off",
			Enabled:        false,
			CreatedBy:      user.ID,
			UpdatedBy:      user.ID,
		})
		require.NoError(t, err)

		configs, err := chatMCPServerConfigs(ctx, db, chatOrg.ID, []uuid.UUID{disabledCfg.ID})
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, disabledCfg.ID, configs[0].ID)
	})

	t.Run("ThirdOrgConfigMissing", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// A config outside the chat-org/default-org pair is reported
		// missing even though it exists and is enabled.
		chatOrg, _ := newOrgWithConfig(t, db, true)
		_, thirdOrgCfg := newOrgWithConfig(t, db, true)

		configs, err := chatMCPServerConfigs(ctx, db, chatOrg.ID, []uuid.UUID{thirdOrgCfg.ID})
		require.NoError(t, err)
		require.Empty(t, configs)
	})

	t.Run("EmptyIDs", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		configs, err := chatMCPServerConfigs(ctx, db, uuid.New(), nil)
		require.NoError(t, err)
		require.Empty(t, configs)
	})

	t.Run("DefaultOrgFallbackUnderSystemSubject", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// The handler resolves the default organization as chatd, so the
		// fallback works under the system-restricted subject used by the
		// HTTP validation paths.
		defaultOrg, err := db.GetDefaultOrganization(ctx)
		require.NoError(t, err)

		chatOrg, _ := newOrgWithConfig(t, db, true)
		defaultOrgCfg := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID: defaultOrg.ID,
			Enabled:        true,
		})

		configs, err := chatMCPServerConfigs(dbauthz.AsSystemRestricted(ctx), db, chatOrg.ID, []uuid.UUID{defaultOrgCfg.ID})
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, defaultOrgCfg.ID, configs[0].ID)
	})
}
