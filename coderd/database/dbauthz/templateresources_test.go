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
	template := database.Foundation{ID: uuid.New(), OrganizationID: uuid.New()}
	state := database.TemplateResourceState{FoundationID: template.ID, OrganizationID: template.OrganizationID}
	revision := database.TemplateResourceRevision{ID: uuid.New(), FoundationID: template.ID, OrganizationID: template.OrganizationID}
	run := database.TemplateResourceRun{Action: "plan", ID: uuid.New(), JobID: uuid.New(), FoundationID: template.ID, OrganizationID: template.OrganizationID}
	for _, tc := range []struct {
		name        string
		arg, result any
		system      bool
		action      policy.Action
	}{
		{name: "LockTemplateResourceTemplate", arg: template.ID, system: true, action: policy.ActionRead},
		{name: "GetTemplateResourceState", arg: template.ID, result: state, action: policy.ActionRead},
		{name: "GetTemplateResourceStateForUpdate", arg: template.ID, result: state, action: policy.ActionRead},
		{name: "InsertTemplateResourceState", arg: database.InsertTemplateResourceStateParams{FoundationID: template.ID}, result: state},
		{name: "InsertTemplateResourceRevision", arg: database.InsertTemplateResourceRevisionParams{FoundationID: template.ID}, result: revision},
		{name: "GetTemplateResourceRevisionByID", arg: revision.ID, result: revision, action: policy.ActionRead},
		{name: "GetTemplateResourceRevisionsByFoundationID", arg: template.ID, result: []database.TemplateResourceRevision{revision}, action: policy.ActionRead},
		{name: "SetTemplateResourceCurrentRevision", arg: database.SetTemplateResourceCurrentRevisionParams{FoundationID: template.ID}, result: state},
		{name: "InsertTemplateResourceRun", arg: database.InsertTemplateResourceRunParams{FoundationID: template.ID, Action: "plan"}, result: run},
		{name: "GetTemplateResourceRunByID", arg: run.ID, result: run, action: policy.ActionRead},
		{name: "GetTemplateResourceRunByJobID", arg: run.JobID, result: run, action: policy.ActionRead},
		{name: "GetTemplateResourceRunsByFoundationID", arg: template.ID, result: []database.TemplateResourceRun{run}, action: policy.ActionRead},
		{name: "StartTemplateResourceRun", arg: database.StartTemplateResourceRunParams{FoundationID: template.ID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}}, result: state},
		{name: "UpdateTemplateResourceState", arg: database.UpdateTemplateResourceStateParams{FoundationID: template.ID}, result: state, system: true},
		{name: "CompleteTemplateResourceRun", arg: database.CompleteTemplateResourceRunParams{FoundationID: template.ID}, result: state, system: true},
		{name: "PinWorkspaceBuildTemplateResources", arg: database.PinWorkspaceBuildTemplateResourcesParams{ID: uuid.New()}, system: true},
	} {
		s.Run(tc.name, s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
			dbm.EXPECT().GetFoundationByID(gomock.Any(), template.ID).Return(template, nil).AnyTimes()
			// This suite invokes wrappers by method name; register the matching mock
			// the same way so every generated Store method has authorization coverage.
			call := reflect.ValueOf(dbm.EXPECT()).MethodByName(tc.name).Call([]reflect.Value{reflect.ValueOf(gomock.Any()), reflect.ValueOf(tc.arg)})[0].Interface().(*gomock.Call)
			var object rbac.Objecter = template
			if tc.system {
				object = rbac.ResourceSystem
			}
			action := tc.action
			if action == "" {
				action = policy.ActionUpdate
			}
			if tc.name == "StartTemplateResourceRun" {
				dbm.EXPECT().GetTemplateResourceRunByID(gomock.Any(), run.ID).Return(run, nil).AnyTimes()
			}
			check.Args(tc.arg).Asserts(object, action)
			if tc.result == nil {
				call.Return(nil).AnyTimes()
			} else {
				call.Return(tc.result, nil).AnyTimes()
				check.Returns(tc.result)
			}
		}))
	}
}
