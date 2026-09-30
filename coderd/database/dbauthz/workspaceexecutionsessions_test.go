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

func (s *MethodTestSuite) TestWorkspaceExecutionSessionStorage() {
	session := database.WorkspaceExecutionSession{ID: uuid.New(), OrganizationID: uuid.New(), OwnerID: uuid.New(), ActorID: uuid.New(), RequestID: uuid.New()}
	object := rbac.ResourceWorkspaceExecution.WithID(session.ID).InOrg(session.OrganizationID).WithOwner(session.OwnerID.String())
	s.Run("GetWorkspaceExecutionSessionByID", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		db.EXPECT().GetWorkspaceExecutionSessionByID(gomock.Any(), session.ID).Return(session, nil).AnyTimes()
		check.Args(session.ID).Asserts(object, policy.ActionRead).Returns(session)
	}))
	s.Run("GetWorkspaceExecutionSessionByRequest", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetWorkspaceExecutionSessionByRequestParams{OrganizationID: session.OrganizationID, ActorID: session.ActorID, RequestID: session.RequestID}
		db.EXPECT().GetWorkspaceExecutionSessionByRequest(gomock.Any(), arg).Return(session, nil).AnyTimes()
		check.Args(arg).Asserts(object, policy.ActionRead).Returns(session)
	}))
	s.Run("InsertWorkspaceExecutionSession", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertWorkspaceExecutionSessionParams{ID: session.ID, OrganizationID: session.OrganizationID, OwnerID: session.OwnerID, ActorID: session.ActorID}
		db.EXPECT().InsertWorkspaceExecutionSession(gomock.Any(), arg).Return(session, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceWorkspaceExecution.InOrg(session.OrganizationID).WithOwner(session.OwnerID.String()), policy.ActionCreate).Returns(session)
	}))
}
