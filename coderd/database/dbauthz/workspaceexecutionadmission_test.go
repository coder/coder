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

func (s *MethodTestSuite) TestWorkspaceExecutionAdmission() {
	workspaceID := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	s.Run("HasClosedWorkspaceExecutionAdmission", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		db.EXPECT().HasClosedWorkspaceExecutionAdmission(gomock.Any(), workspaceID).Return(true, nil).AnyTimes()
		check.Args(workspaceID).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(true)
	}))
	s.Run("HasProtectedWorkspaceExecutionSession", s.Mocked(func(db *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.HasProtectedWorkspaceExecutionSessionParams{WorkspaceID: workspaceID, Now: time.Now()}
		db.EXPECT().HasProtectedWorkspaceExecutionSession(gomock.Any(), arg).Return(true, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(true)
	}))
}
