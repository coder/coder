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

func (s *MethodTestSuite) TestFoundationPreview() {
	s.Run("GetFoundationConsumerTemplateVersions", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		id := uuid.New()
		rows := []database.GetFoundationConsumerTemplateVersionsRow{{ID: uuid.New(), FileID: uuid.New()}}
		dbm.EXPECT().GetFoundationConsumerTemplateVersions(gomock.Any(), id).Return(rows, nil).AnyTimes()
		check.Args(id).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(rows)
	}))
	s.Run("UpdateTemplateResourceRunPlanPreview", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.UpdateTemplateResourceRunPlanPreviewParams{ID: uuid.New()}
		dbm.EXPECT().UpdateTemplateResourceRunPlanPreview(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
}
