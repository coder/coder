package dbauthz_test

import (
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func (s *MethodTestSuite) TestWorkspaceWideExecutionProtection() {
	session := database.WorkspaceExecutionSession{ID: uuid.New(), OrganizationID: uuid.New(), OwnerID: uuid.New()}
	s.Run("HasPendingWorkspaceExecutionReceiptsByWorkspaceID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		workspace := database.Workspace{ID: uuid.New(), OrganizationID: session.OrganizationID, OwnerID: session.OwnerID}
		id := uuid.NullUUID{UUID: workspace.ID, Valid: true}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), workspace.ID).Return(workspace, nil).AnyTimes()
		dbm.EXPECT().HasPendingWorkspaceExecutionReceiptsByWorkspaceID(gomock.Any(), id).Return(true, nil).AnyTimes()
		check.Args(id).Asserts(workspace, policy.ActionSSH).Returns(true)
	}))
	s.Run("GetOtherWorkspaceExecutionSessionsByWorkspaceID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		workspace := database.Workspace{ID: uuid.New(), OrganizationID: session.OrganizationID, OwnerID: session.OwnerID}
		arg := database.GetOtherWorkspaceExecutionSessionsByWorkspaceIDParams{WorkspaceID: uuid.NullUUID{UUID: workspace.ID, Valid: true}, ID: session.ID}
		rows := []database.WorkspaceExecutionSession{session}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), workspace.ID).Return(workspace, nil).AnyTimes()
		dbm.EXPECT().GetOtherWorkspaceExecutionSessionsByWorkspaceID(gomock.Any(), arg).Return(rows, nil).AnyTimes()
		check.Args(arg).Asserts(workspace, policy.ActionSSH).Returns(rows)
	}))
}

func (s *MethodTestSuite) TestWorkspaceExecutionControllerStore() {
	session := database.WorkspaceExecutionSession{ID: uuid.New(), OrganizationID: uuid.New(), OwnerID: uuid.New()}
	object := rbac.ResourceWorkspaceExecution.WithID(session.ID).InOrg(session.OrganizationID).WithOwner(session.OwnerID.String())
	workspaceID := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	s.Run("UpdateWorkspaceExecutionSession", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.UpdateWorkspaceExecutionSessionParams{ID: session.ID}
		db.EXPECT().GetWorkspaceExecutionSessionByID(gomock.Any(), session.ID).Return(session, nil).AnyTimes()
		db.EXPECT().UpdateWorkspaceExecutionSession(gomock.Any(), arg).Return(session, nil).AnyTimes()
		check.Args(arg).Asserts(object, policy.ActionUpdate).Returns(session)
	}))
	s.Run("GetReconciliableWorkspaceExecutionSessions", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		now := time.Now()
		rows := []database.WorkspaceExecutionSession{session}
		arg := database.GetReconciliableWorkspaceExecutionSessionsParams{Now: now, AfterID: uuid.Nil}
		db.EXPECT().GetReconciliableWorkspaceExecutionSessions(gomock.Any(), arg).Return(rows, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(rows)
	}))
	s.Run("LockWorkspaceExecutionWorkspace", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		workspace := database.Workspace{ID: workspaceID.UUID, OrganizationID: session.OrganizationID, OwnerID: session.OwnerID}
		db.EXPECT().LockWorkspaceExecutionWorkspace(gomock.Any(), workspace.ID).Return(nil).AnyTimes()
		db.EXPECT().GetWorkspaceByID(gomock.Any(), workspace.ID).Return(workspace, nil).AnyTimes()
		check.Args(workspace.ID).Asserts(workspace, policy.ActionRead)
	}))
	s.Run("HasBusyWorkspaceExecutionChats", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		result := true
		db.EXPECT().HasBusyWorkspaceExecutionChats(gomock.Any(), workspaceID).Return(result, nil).AnyTimes()
		check.Args(workspaceID).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(result)
	}))
}
