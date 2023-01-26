package authzquery_test

import (
	"context"
	"testing"
	"time"

	"github.com/moby/moby/pkg/namesgenerator"

	"github.com/coder/coder/coderd/rbac"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"

	"github.com/coder/coder/coderd/authzquery"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database/databasefake"
)

func TestWorkspaceFunctions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name   string
		Config *authorizeTest
	}{
		{
			Name: "GetByID",
			Config: &authorizeTest{
				Data: func(t *testing.T, tc *authorizeTest) map[string]interface{} {
					return map[string]interface{}{
						"u-one": database.User{},
						"w-one": database.Workspace{
							Name:       "peter-pan",
							OwnerID:    tc.Lookup("u-one"),
							TemplateID: tc.Lookup("t-one"),
						},
						"t-one": database.Template{},
					}
				},
				Test: func(ctx context.Context, t *testing.T, tc *authorizeTest, q authzquery.AuthzStore) {
					wrk, err := q.GetWorkspaceByID(ctx, tc.Lookup("w-one"))
					require.NoError(t, err)

					wrk, err = q.GetWorkspaceByID(ctx, tc.Lookup("w-one"))
					require.NoError(t, err)

					_, err = q.GetTemplateByID(ctx, wrk.TemplateID)
					require.NoError(t, err)
				},
				Asserts: map[string][]rbac.Action{
					"w-one": {rbac.ActionRead, rbac.ActionRead},
					"t-one": {rbac.ActionRead},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			testAuthorizeFunction(t, tc.Config)
		})
	}

}

func TestWorkspace(t *testing.T) {
	// GetWorkspaceByID
	var (
		db = databasefake.New()
		// TODO: Recorder should record all authz calls
		rec   = &coderdtest.RecordingAuthorizer{}
		q     = authzquery.NewAuthzQuerier(db, rec)
		actor = rbac.Subject{
			ID:     uuid.New().String(),
			Roles:  rbac.RoleNames{rbac.RoleOwner()},
			Groups: []string{},
			Scope:  rbac.ScopeAll,
		}
		ctx = authzquery.WithAuthorizeContext(context.Background(), actor)
	)

	workspace := insertRandomWorkspace(t, db)

	// Test recorder
	_, err := q.GetWorkspaceByID(ctx, workspace.ID)
	require.NoError(t, err)

	_, err = q.UpdateWorkspace(ctx, database.UpdateWorkspaceParams{
		ID:   workspace.ID,
		Name: "new-name",
	})
	require.NoError(t, err)

	rec.AssertActor(t, actor,
		rec.Pair(rbac.ActionRead, workspace),
		rec.Pair(rbac.ActionUpdate, workspace),
	)
	require.NoError(t, rec.AllAsserted())
}

func insertRandomWorkspace(t *testing.T, db database.Store, opts ...func(w *database.Workspace)) database.Workspace {
	workspace := &database.Workspace{
		ID:             uuid.New(),
		CreatedAt:      time.Now().Add(time.Hour * -1),
		UpdatedAt:      time.Now(),
		OwnerID:        uuid.New(),
		OrganizationID: uuid.New(),
		TemplateID:     uuid.New(),
		Deleted:        false,
		Name:           namesgenerator.GetRandomName(1),
		LastUsedAt:     time.Now(),
	}
	for _, opt := range opts {
		opt(workspace)
	}

	newWorkspace, err := db.InsertWorkspace(context.Background(), database.InsertWorkspaceParams{
		ID:                workspace.ID,
		CreatedAt:         workspace.CreatedAt,
		UpdatedAt:         workspace.UpdatedAt,
		OwnerID:           workspace.OwnerID,
		OrganizationID:    workspace.OrganizationID,
		TemplateID:        workspace.TemplateID,
		Name:              workspace.Name,
		AutostartSchedule: workspace.AutostartSchedule,
		Ttl:               workspace.Ttl,
	})
	require.NoError(t, err, "insert workspace")
	return newWorkspace
}
