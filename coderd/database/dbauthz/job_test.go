package dbauthz_test

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

func (s *MethodTestSuite) TestProvsionerJob() {
	s.Run("Build/GetProvisionerJobByID", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		check.Args(j.ID).Asserts(w, rbac.ActionRead).Returns(j)
	}))
	s.Run("TemplateVersion/GetProvisionerJobByID", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionImport,
		})
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
			JobID:      j.ID,
		})
		check.Args(j.ID).Asserts(v.RBACObject(tpl), rbac.ActionRead).Returns(j)
	}))
	s.Run("TemplateVersionDryRun/GetProvisionerJobByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
		})
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionDryRun,
			Input: must(json.Marshal(struct {
				TemplateVersionID uuid.UUID `json:"template_version_id"`
			}{TemplateVersionID: v.ID})),
		})
		check.Args(j.ID).Asserts(v.RBACObject(tpl), rbac.ActionRead).Returns(j)
	}))
	s.Run("Build/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{AllowUserCancelWorkspaceJobs: true})
		w := dbgen.Workspace(s.T(), db, database.Workspace{TemplateID: tpl.ID})
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).Asserts(w, rbac.ActionUpdate).Returns()
	}))
	s.Run("TemplateVersion/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionImport,
		})
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
			JobID:      j.ID,
		})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).
			Asserts(v.RBACObject(tpl), []rbac.Action{rbac.ActionRead, rbac.ActionUpdate}).Returns()
	}))
	s.Run("TemplateVersionDryRun/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
		})
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionDryRun,
			Input: must(json.Marshal(struct {
				TemplateVersionID uuid.UUID `json:"template_version_id"`
			}{TemplateVersionID: v.ID})),
		})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).
			Asserts(v.RBACObject(tpl), []rbac.Action{rbac.ActionRead, rbac.ActionUpdate}).Returns()
	}))
	s.Run("GetProvisionerJobsByIDs", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		b := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		check.Args([]uuid.UUID{a.ID, b.ID}).Asserts().Returns(slice.New(a, b))
	}))
	s.Run("GetProvisionerLogsByIDBetween", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		check.Args(database.GetProvisionerLogsByIDBetweenParams{
			JobID: j.ID,
		}).Asserts(w, rbac.ActionRead).Returns([]database.ProvisionerJobLog{})
	}))
}
