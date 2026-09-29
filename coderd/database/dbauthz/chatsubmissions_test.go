package dbauthz_test

import (
	"encoding/json"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

func (s *MethodTestSuite) TestChatSubmissions() {
	s.Run("InsertChatSubmission", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		arg := testutil.Fake(s.T(), faker, database.InsertChatSubmissionParams{})
		row := database.ChatSubmission{ID: uuid.New()}
		db.EXPECT().InsertChatSubmission(gomock.Any(), arg).Return(row, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceChat.WithOwner(arg.ActorID.String()).InOrg(arg.OrganizationID), policy.ActionCreate).Returns(row)
	}))
	s.Run("GetChatSubmission", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		arg := testutil.Fake(s.T(), faker, database.GetChatSubmissionParams{})
		row := database.ChatSubmission{ID: uuid.New()}
		db.EXPECT().GetChatSubmission(gomock.Any(), arg).Return(row, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceChat.WithOwner(arg.ActorID.String()).InOrg(arg.OrganizationID), policy.ActionRead).Returns(row)
	}))
	s.Run("CompleteChatSubmission", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		arg := testutil.Fake(s.T(), faker, database.CompleteChatSubmissionParams{})
		row := database.ChatSubmission{ID: uuid.New()}
		db.EXPECT().CompleteChatSubmission(gomock.Any(), arg).Return(row, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceChat.WithOwner(arg.ActorID.String()).InOrg(arg.OrganizationID), policy.ActionUpdate).Returns(row)
	}))
	s.Run("FinishChatSubmission", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		arg := testutil.Fake(s.T(), faker, database.FinishChatSubmissionParams{})
		row := database.ChatSubmission{ID: uuid.New()}
		db.EXPECT().FinishChatSubmission(gomock.Any(), arg).Return(row, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceChat.WithOwner(arg.ActorID.String()).InOrg(arg.OrganizationID), policy.ActionUpdate).Returns(row)
	}))
	s.Run("GetLatestChatSubmissionSettings", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		chat := testutil.Fake(s.T(), faker, database.Chat{})
		db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil).AnyTimes()
		db.EXPECT().GetLatestChatSubmissionSettings(gomock.Any(), chat.ID).Return(json.RawMessage("null"), nil).AnyTimes()
		check.Args(chat.ID).Asserts(chat, policy.ActionRead).Returns(json.RawMessage("null"))
	}))
}
