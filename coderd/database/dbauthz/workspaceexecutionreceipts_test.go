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

func (s *MethodTestSuite) TestWorkspaceExecutionReceipts() {
	session := database.WorkspaceExecutionSession{ID: uuid.New(), OrganizationID: uuid.New(), OwnerID: uuid.New()}
	object := rbac.ResourceWorkspaceExecution.WithID(session.ID).InOrg(session.OrganizationID).WithOwner(session.OwnerID.String())
	receipt := database.WorkspaceExecutionReceipt{ID: uuid.New(), SessionID: session.ID, OrganizationID: session.OrganizationID, OwnerID: session.OwnerID, ActorID: uuid.New(), RequestID: uuid.New()}
	s.Run("GetWorkspaceExecutionReceiptByID", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		db.EXPECT().GetWorkspaceExecutionReceiptByID(gomock.Any(), receipt.ID).Return(receipt, nil).AnyTimes()
		check.Args(receipt.ID).Asserts(object, policy.ActionSSH).Returns(receipt)
	}))
	s.Run("GetWorkspaceExecutionReceiptByRequest", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetWorkspaceExecutionReceiptByRequestParams{SessionID: receipt.SessionID, ActorID: receipt.ActorID, RequestID: receipt.RequestID}
		db.EXPECT().GetWorkspaceExecutionReceiptByRequest(gomock.Any(), arg).Return(receipt, nil).AnyTimes()
		check.Args(arg).Asserts(object, policy.ActionSSH).Returns(receipt)
	}))
	s.Run("InsertWorkspaceExecutionReceipt", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertWorkspaceExecutionReceiptParams{ID: receipt.ID, SessionID: session.ID}
		db.EXPECT().GetWorkspaceExecutionSessionByID(gomock.Any(), session.ID).Return(session, nil).AnyTimes()
		db.EXPECT().InsertWorkspaceExecutionReceipt(gomock.Any(), arg).Return(receipt, nil).AnyTimes()
		check.Args(arg).Asserts(object, policy.ActionSSH).Returns(receipt)
	}))
	s.Run("UpdateWorkspaceExecutionReceipt", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.UpdateWorkspaceExecutionReceiptParams{ID: receipt.ID}
		db.EXPECT().GetWorkspaceExecutionReceiptByID(gomock.Any(), receipt.ID).Return(receipt, nil).AnyTimes()
		db.EXPECT().UpdateWorkspaceExecutionReceipt(gomock.Any(), arg).Return(receipt, nil).AnyTimes()
		check.Args(arg).Asserts(object, policy.ActionSSH).Returns(receipt)
	}))
	s.Run("HasPendingWorkspaceExecutionReceipts", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		db.EXPECT().GetWorkspaceExecutionSessionByID(gomock.Any(), session.ID).Return(session, nil).AnyTimes()
		db.EXPECT().HasPendingWorkspaceExecutionReceipts(gomock.Any(), session.ID).Return(true, nil).AnyTimes()
		check.Args(session.ID).Asserts(object, policy.ActionSSH).Returns(true)
	}))
	s.Run("GetPendingWorkspaceExecutionReceipts", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		rows := []database.WorkspaceExecutionReceipt{receipt}
		db.EXPECT().GetWorkspaceExecutionSessionByID(gomock.Any(), session.ID).Return(session, nil).AnyTimes()
		db.EXPECT().GetPendingWorkspaceExecutionReceipts(gomock.Any(), session.ID).Return(rows, nil).AnyTimes()
		check.Args(session.ID).Asserts(object, policy.ActionSSH).Returns(rows)
	}))
}
