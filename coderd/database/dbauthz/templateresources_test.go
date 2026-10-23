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

func (s *MethodTestSuite) TestTemplateResources() {
	template := database.Template{ID: uuid.New(), OrganizationID: uuid.New()}
	state := database.TemplateResourceState{TemplateID: template.ID, OrganizationID: template.OrganizationID}
	revision := database.TemplateResourceRevision{ID: uuid.New(), TemplateID: template.ID, OrganizationID: template.OrganizationID}
	run := database.TemplateResourceRun{ID: uuid.New(), JobID: uuid.New(), TemplateID: template.ID, OrganizationID: template.OrganizationID}
	for _, tc := range []struct {
		name        string
		arg, result any
		system      bool
	}{
		{name: "LockTemplateResourceTemplate", arg: template.ID},
		{name: "GetTemplateResourceState", arg: template.ID, result: state},
		{name: "GetTemplateResourceStateForUpdate", arg: template.ID, result: state},
		{name: "InsertTemplateResourceState", arg: database.InsertTemplateResourceStateParams{TemplateID: template.ID}, result: state},
		{name: "InsertTemplateResourceRevision", arg: database.InsertTemplateResourceRevisionParams{TemplateID: template.ID}, result: revision},
		{name: "GetTemplateResourceRevisionByID", arg: revision.ID, result: revision},
		{name: "GetTemplateResourceRevisionsByTemplateID", arg: template.ID, result: []database.TemplateResourceRevision{revision}},
		{name: "SetTemplateResourceCurrentRevision", arg: database.SetTemplateResourceCurrentRevisionParams{TemplateID: template.ID}, result: state},
		{name: "InsertTemplateResourceRun", arg: database.InsertTemplateResourceRunParams{TemplateID: template.ID}, result: run},
		{name: "GetTemplateResourceRunByID", arg: run.ID, result: run},
		{name: "GetTemplateResourceRunByJobID", arg: run.JobID, result: run},
		{name: "GetTemplateResourceRunsByTemplateID", arg: template.ID, result: []database.TemplateResourceRun{run}},
		{name: "StartTemplateResourceRun", arg: database.StartTemplateResourceRunParams{TemplateID: template.ID}, result: state},
		{name: "UpdateTemplateResourceState", arg: database.UpdateTemplateResourceStateParams{TemplateID: template.ID}, result: state, system: true},
		{name: "CompleteTemplateResourceRun", arg: database.CompleteTemplateResourceRunParams{TemplateID: template.ID}, result: state, system: true},
		{name: "PinWorkspaceBuildTemplateResources", arg: database.PinWorkspaceBuildTemplateResourcesParams{ID: uuid.New()}, system: true},
	} {
		s.Run(tc.name, s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
			dbm.EXPECT().GetTemplateByID(gomock.Any(), template.ID).Return(template, nil).AnyTimes()
			// This suite invokes wrappers by method name; register the matching mock
			// the same way so every generated Store method has authorization coverage.
			call := reflect.ValueOf(dbm.EXPECT()).MethodByName(tc.name).Call([]reflect.Value{reflect.ValueOf(gomock.Any()), reflect.ValueOf(tc.arg)})[0].Interface().(*gomock.Call)
			var object rbac.Objecter = template
			if tc.system {
				object = rbac.ResourceSystem
			}
			check.Args(tc.arg).Asserts(object, policy.ActionUpdate)
			if tc.result == nil {
				call.Return(nil).AnyTimes()
			} else {
				call.Return(tc.result, nil).AnyTimes()
				check.Returns(tc.result)
			}
		}))
	}
}
