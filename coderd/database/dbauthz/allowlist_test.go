package dbauthz_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
)

// TestAllowListIDScopedListEndpoints ensures list queries compiled from an
// ID-scoped allow list produce valid SQL. The queries join tables that also
// have an "id" column, so the ID matcher must be table qualified.
func TestAllowListIDScopedListEndpoints(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})
	// Each build creates its own template, so the allow list below admits one
	// workspace and one template and excludes the others.
	ws := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
	}).Do().Workspace
	_ = dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
	}).Do()

	scope, err := rbac.ScopeAll.Expand()
	require.NoError(t, err)
	scope.AllowIDList = []rbac.AllowListElement{
		{Type: rbac.ResourceWorkspace.Type, ID: ws.ID.String()},
		{Type: rbac.ResourceTemplate.Type, ID: ws.TemplateID.String()},
	}

	db := dbauthz.New(store, rbac.NewAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), coderdtest.AccessControlStorePointer())
	subject := rbac.Subject{
		ID:     user.ID.String(),
		Roles:  rbac.Roles(must(rbac.RoleIdentifiers{rbac.RoleOwner()}.Expand())),
		Groups: []string{},
		Scope:  scope,
	}

	t.Run("GetWorkspaces", func(t *testing.T) {
		t.Parallel()
		ctx := dbauthz.As(testutil.Context(t, testutil.WaitMedium), subject)
		rows, err := db.GetWorkspaces(ctx, database.GetWorkspacesParams{})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, ws.ID, rows[0].ID)
	})

	t.Run("GetTemplatesWithFilter", func(t *testing.T) {
		t.Parallel()
		ctx := dbauthz.As(testutil.Context(t, testutil.WaitMedium), subject)
		rows, err := db.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, ws.TemplateID, rows[0].ID)
	})
}
