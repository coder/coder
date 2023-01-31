package authzquery_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/authzquery"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func TestWorkspaceFunctions(t *testing.T) {
	t.Parallel()

	const mainWorkspace = "workspace-one"
	workspaceData := func(t *testing.T, tc *authorizeTest) map[string]interface{} {
		return map[string]interface{}{
			"u-one": database.User{},
			mainWorkspace: database.Workspace{
				Name:       "peter-pan",
				OwnerID:    tc.Lookup("u-one"),
				TemplateID: tc.Lookup("t-one"),
			},
			"t-one": database.Template{},
			"b-one": database.WorkspaceBuild{
				WorkspaceID: tc.Lookup(mainWorkspace),
				//TemplateVersionID: uuid.UUID{},
				BuildNumber: 0,
				Transition:  database.WorkspaceTransitionStart,
				InitiatorID: tc.Lookup("u-one"),
				//JobID:             uuid.UUID{},
			},
		}
	}

	testCases := []struct {
		Name   string
		Config *authorizeTest
	}{
		{
			Name: "GetWorkspaceByID",
			Config: &authorizeTest{
				Data: workspaceData,
				Test: func(ctx context.Context, t *testing.T, tc *authorizeTest, q authzquery.AuthzStore) {
					_, err := q.GetWorkspaceByID(ctx, tc.Lookup(mainWorkspace))
					require.NoError(t, err)
				},
				Asserts: map[string][]rbac.Action{
					mainWorkspace: {rbac.ActionRead},
				},
			},
		},
		{
			Name: "GetWorkspaces",
			Config: &authorizeTest{
				Data: workspaceData,
				Test: func(ctx context.Context, t *testing.T, tc *authorizeTest, q authzquery.AuthzStore) {
					_, err := q.GetWorkspaces(ctx, database.GetWorkspacesParams{})
					require.NoError(t, err)
				},
				Asserts: map[string][]rbac.Action{
					// SQL filter does not generate authz calls
				},
			},
		},
		{
			Name: "GetLatestWorkspaceBuildByWorkspaceID",
			Config: &authorizeTest{
				Data: workspaceData,
				Test: func(ctx context.Context, t *testing.T, tc *authorizeTest, q authzquery.AuthzStore) {
					_, err := q.GetLatestWorkspaceBuildByWorkspaceID(ctx, tc.Lookup(mainWorkspace))
					require.NoError(t, err)
				},
				Asserts: map[string][]rbac.Action{
					mainWorkspace: {rbac.ActionRead},
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
