package dbauthz_test

import (
	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func (s *MethodTestSuite) TestWorkspaceExecutionArtifacts() {
	session := database.WorkspaceExecutionSession{ID: uuid.New(), OrganizationID: uuid.New(), OwnerID: uuid.New()}
	object := rbac.ResourceWorkspaceExecution.WithID(session.ID).WithOwner(session.OwnerID.String()).InOrg(session.OrganizationID)
	s.Run("InsertWorkspaceExecutionArtifact", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertWorkspaceExecutionArtifactParams{SessionID: session.ID}
		dbm.EXPECT().GetWorkspaceExecutionSessionByID(gomock.Any(), session.ID).Return(session, nil).AnyTimes()
		dbm.EXPECT().InsertWorkspaceExecutionArtifact(gomock.Any(), arg).Return(int64(1), nil).AnyTimes()
		check.Args(arg).Asserts(object, policy.ActionSSH).Returns(int64(1))
	}))
	s.Run("GetWorkspaceExecutionArtifactsBySessionID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		artifact := database.GetWorkspaceExecutionArtifactsBySessionIDRow{ID: uuid.New(), SessionID: session.ID, OrganizationID: session.OrganizationID, OwnerID: session.OwnerID}
		rows := []database.GetWorkspaceExecutionArtifactsBySessionIDRow{artifact}
		dbm.EXPECT().GetWorkspaceExecutionArtifactsBySessionID(gomock.Any(), session.ID).Return(rows, nil).AnyTimes()
		check.Args(session.ID).Asserts(object, policy.ActionRead).Returns(rows)
	}))
	s.Run("ReadWorkspaceExecutionArtifact", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		artifact := database.ReadWorkspaceExecutionArtifactRow{ID: uuid.New(), SessionID: session.ID, OrganizationID: session.OrganizationID, OwnerID: session.OwnerID}
		arg := database.ReadWorkspaceExecutionArtifactParams{ID: artifact.ID}
		dbm.EXPECT().ReadWorkspaceExecutionArtifact(gomock.Any(), arg).Return(artifact, nil).AnyTimes()
		check.Args(arg).Asserts(object, policy.ActionSSH).Returns(artifact)
	}))
}
