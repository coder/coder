package authzquery_test

import (
	"context"
	"testing"
	"time"

	"github.com/coder/coder/coderd/rbac"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"

	"github.com/coder/coder/coderd/authzquery"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database/databasefake"
)

func TestWorkspace(t *testing.T) {
	// GetWorkspaceByID
	var (
		db = databasefake.New()
		// TODO: Recorder should record all authz calls
		rec   = &coderdtest.RecordingAuthorizer{}
		q     = authzquery.NewAuthzQuerier(db, rec)
		ctx   = context.Background()
		actor = authzquery.WithAuthorizeContext(ctx,
			uuid.New(),
			rbac.RoleNames{rbac.RoleOwner()},
			[]string{},
			rbac.ScopeAll,
		)
	)

	// Seed db
	workspace, err := db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
		ID:             uuid.New(),
		CreatedAt:      time.Time{},
		UpdatedAt:      time.Time{},
		OwnerID:        uuid.New(),
		OrganizationID: uuid.New(),
		TemplateID:     uuid.New(),
		Name:           "fake-workspace",
	})
	require.NoError(t, err)

	// Test
	// NoAuth
	_, err = q.GetWorkspaceByID(ctx, workspace.ID)
	require.Error(t, err, "no actor in context")

	// Test recorder
	_, err = q.GetWorkspaceByID(actor, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, rec.Called.Object, workspace.RBACObject())
}
