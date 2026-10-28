package dbauthz_test

import (
	"reflect"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func (s *MethodTestSuite) TestFoundations() {
	f := database.Foundation{ID: uuid.New(), OrganizationID: uuid.New()}
	template := database.Template{ID: uuid.New(), OrganizationID: f.OrganizationID}
	for _, tc := range []struct {
		name        string
		arg, result any
		object      rbac.Objecter
		action      policy.Action
	}{
		{"InsertFoundation", database.InsertFoundationParams{OrganizationID: f.OrganizationID}, f, rbac.ResourceFoundation.InOrg(f.OrganizationID), policy.ActionCreate},
		{"GetFoundationByID", f.ID, f, f, policy.ActionRead},
		{"GetFoundationByTemplateID", uuid.NullUUID{UUID: template.ID, Valid: true}, f, f, policy.ActionRead},
		{"GetFoundationsByOrganizationID", f.OrganizationID, []database.Foundation{f}, f, policy.ActionRead},
		{"UpdateFoundation", database.UpdateFoundationParams{ID: f.ID}, f, f, policy.ActionUpdate},
		{"DeleteFoundation", database.DeleteFoundationParams{ID: f.ID}, f, f, policy.ActionDelete},
		{"LockFoundation", f.ID, nil, f, policy.ActionRead},
		{"GetFoundationWorkspaceCount", f.ID, int64(1), rbac.ResourceSystem, policy.ActionRead},
		{"GetFoundationConsumerCounts", f.ID, database.GetFoundationConsumerCountsRow{Total: 1, Unknown: 1}, rbac.ResourceSystem, policy.ActionRead},
		{"GetFileFoundations", uuid.New(), []database.Foundation{f}, rbac.ResourceSystem, policy.ActionRead},
	} {
		s.Run(tc.name, s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
			dbm.EXPECT().GetFoundationByID(gomock.Any(), f.ID).Return(f, nil).AnyTimes()
			call := reflect.ValueOf(dbm.EXPECT()).MethodByName(tc.name).Call([]reflect.Value{reflect.ValueOf(gomock.Any()), reflect.ValueOf(tc.arg)})[0].Interface().(*gomock.Call)
			check.Args(tc.arg).Asserts(tc.object, tc.action)
			if tc.result == nil {
				call.Return(nil).AnyTimes()
			} else {
				call.Return(tc.result, nil).AnyTimes()
				check.Returns(tc.result)
			}
		}))
	}
	s.Run("BindFoundation", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.BindFoundationParams{ID: f.ID, TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true}}
		dbm.EXPECT().GetFoundationByID(gomock.Any(), f.ID).Return(f, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), template.ID).Return(template, nil).AnyTimes()
		dbm.EXPECT().BindFoundation(gomock.Any(), arg).Return(f, nil).AnyTimes()
		check.Args(arg).Asserts(f, policy.ActionUse, template, policy.ActionUpdate).Returns(f)
	}))
}
