package dbauthz_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/rbac"
)

// TestWorkspaceExecutionAuthorization uses the production authorizer. Denied
// mutations have no underlying-store expectation, so an unauthorized side
// effect fails the test even if the wrapper also returns an error.
func TestWorkspaceExecutionAuthorization(t *testing.T) {
	t.Parallel()
	owner, org := uuid.New(), uuid.New()
	session := database.WorkspaceExecutionSession{ID: uuid.New(), OrganizationID: org, OwnerID: owner}
	receipt := database.WorkspaceExecutionReceipt{ID: uuid.New(), SessionID: session.ID, OrganizationID: org, OwnerID: owner}
	artifact := database.ReadWorkspaceExecutionArtifactRow{ID: uuid.New(), SessionID: session.ID, OrganizationID: org, OwnerID: owner}
	for _, tc := range []struct {
		name                          string
		user                          uuid.UUID
		roles                         rbac.RoleIdentifiers
		scope                         rbac.ScopeName
		create, read, execute, update bool
	}{
		{name: "OwnerExplicitAll", user: owner, scope: rbac.ScopeAll, create: true, read: true, execute: true, update: true},
		{name: "OwnerReadOnly", user: owner, scope: "workspace_execution:read", read: true},
		{name: "OwnerCreateOnly", user: owner, scope: "workspace_execution:create", create: true},
		{name: "OwnerExecuteOnly", user: owner, scope: "workspace_execution:ssh", execute: true},
		{name: "OwnerUpdateOnly", user: owner, scope: "workspace_execution:update", update: true},
		{name: "OwnerWorkspaceAccessOnly", user: owner, scope: "coder:workspaces.access"},
		{name: "OwnerWithoutAccessToResourceOrg", user: owner, roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgWorkspaceAccess(uuid.New())}, scope: rbac.ScopeAll},
		{name: "AnotherMember", user: uuid.New(), scope: rbac.ScopeAll},
		{name: "AnotherOrganizationAdmin", user: uuid.New(), roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgAdmin(uuid.New())}, scope: rbac.ScopeAll},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := dbmock.NewMockStore(gomock.NewController(t))
			raw.EXPECT().Wrappers().Return([]string{}).AnyTimes()
			raw.EXPECT().GetWorkspaceExecutionSessionByID(gomock.Any(), session.ID).Return(session, nil).AnyTimes()
			raw.EXPECT().GetWorkspaceExecutionReceiptByID(gomock.Any(), receipt.ID).Return(receipt, nil).AnyTimes()
			raw.EXPECT().ReadWorkspaceExecutionArtifact(gomock.Any(), database.ReadWorkspaceExecutionArtifactParams{ID: artifact.ID}).Return(artifact, nil).AnyTimes()
			insertSession := database.InsertWorkspaceExecutionSessionParams{ID: session.ID, OrganizationID: org, OwnerID: owner, ActorID: tc.user}
			insertReceipt := database.InsertWorkspaceExecutionReceiptParams{ID: receipt.ID, SessionID: session.ID}
			updateSession := database.UpdateWorkspaceExecutionSessionParams{ID: session.ID}
			updateReceipt := database.UpdateWorkspaceExecutionReceiptParams{ID: receipt.ID}
			insertArtifact := database.InsertWorkspaceExecutionArtifactParams{ID: artifact.ID, SessionID: session.ID}
			if tc.create {
				raw.EXPECT().InsertWorkspaceExecutionSession(gomock.Any(), insertSession).Return(session, nil)
			}
			if tc.update {
				raw.EXPECT().UpdateWorkspaceExecutionSession(gomock.Any(), updateSession).Return(session, nil)
			}
			if tc.execute {
				raw.EXPECT().InsertWorkspaceExecutionReceipt(gomock.Any(), insertReceipt).Return(receipt, nil)
				raw.EXPECT().UpdateWorkspaceExecutionReceipt(gomock.Any(), updateReceipt).Return(receipt, nil)
				raw.EXPECT().InsertWorkspaceExecutionArtifact(gomock.Any(), insertArtifact).Return(int64(1), nil)
			}
			roles := tc.roles
			if roles == nil {
				roles = rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgWorkspaceAccess(org)}
			}
			ctx := dbauthz.As(t.Context(), rbac.Subject{ID: tc.user.String(), Roles: roles, Scope: tc.scope})
			db := dbauthz.New(raw, rbac.NewAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), coderdtest.AccessControlStorePointer())
			check := func(allowed bool, err error) {
				t.Helper()
				if allowed {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					require.True(t, dbauthz.IsNotAuthorizedError(err), "%v", err)
				}
			}
			_, err := db.GetWorkspaceExecutionSessionByID(ctx, session.ID)
			check(tc.read, err)
			_, err = db.InsertWorkspaceExecutionSession(ctx, insertSession)
			check(tc.create, err)
			_, err = db.UpdateWorkspaceExecutionSession(ctx, updateSession)
			check(tc.update, err)
			_, err = db.GetWorkspaceExecutionReceiptByID(ctx, receipt.ID)
			check(tc.execute, err)
			_, err = db.InsertWorkspaceExecutionReceipt(ctx, insertReceipt)
			check(tc.execute, err)
			_, err = db.UpdateWorkspaceExecutionReceipt(ctx, updateReceipt)
			check(tc.execute, err)
			_, err = db.ReadWorkspaceExecutionArtifact(ctx, database.ReadWorkspaceExecutionArtifactParams{ID: artifact.ID})
			check(tc.execute, err)
			_, err = db.InsertWorkspaceExecutionArtifact(ctx, insertArtifact)
			check(tc.execute, err)
		})
	}
}
