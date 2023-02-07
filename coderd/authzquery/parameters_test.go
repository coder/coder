package authzquery_test

import (
	"github.com/coder/coder/coderd/util/slice"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database/dbgen"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (s *MethodTestSuite) TestParameters() {
	s.Run("Workspace/InsertParameterValue", s.Subtest(func(db database.Store, check *MethodCase) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.InsertParameterValueParams{
			ScopeID:           w.ID,
			Scope:             database.ParameterScopeWorkspace,
			SourceScheme:      database.ParameterSourceSchemeNone,
			DestinationScheme: database.ParameterDestinationSchemeNone,
		}).Asserts(w, rbac.ActionUpdate)
	}))
	s.Run("TemplateVersionNoTemplate/InsertParameterValue", s.Subtest(func(db database.Store, check *MethodCase) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{JobID: j.ID, TemplateID: uuid.NullUUID{Valid: false}})
		check.Args(database.InsertParameterValueParams{
			ScopeID:           j.ID,
			Scope:             database.ParameterScopeImportJob,
			SourceScheme:      database.ParameterSourceSchemeNone,
			DestinationScheme: database.ParameterDestinationSchemeNone,
		}).Asserts(v.RBACObjectNoTemplate(), rbac.ActionUpdate)
	}))
	s.Run("TemplateVersionTemplate/InsertParameterValue", s.Subtest(func(db database.Store, check *MethodCase) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{JobID: j.ID,
			TemplateID: uuid.NullUUID{
				UUID:  tpl.ID,
				Valid: true,
			}},
		)
		check.Args(database.InsertParameterValueParams{
			ScopeID:           j.ID,
			Scope:             database.ParameterScopeImportJob,
			SourceScheme:      database.ParameterSourceSchemeNone,
			DestinationScheme: database.ParameterDestinationSchemeNone,
		}).Asserts(v.RBACObject(tpl), rbac.ActionUpdate)
	}))
	s.Run("Template/InsertParameterValue", s.Subtest(func(db database.Store, check *MethodCase) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.InsertParameterValueParams{
			ScopeID:           tpl.ID,
			Scope:             database.ParameterScopeTemplate,
			SourceScheme:      database.ParameterSourceSchemeNone,
			DestinationScheme: database.ParameterDestinationSchemeNone,
		}).Asserts(tpl, rbac.ActionUpdate)
	}))
	s.Run("Template/ParameterValue", s.Subtest(func(db database.Store, check *MethodCase) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		pv := dbgen.ParameterValue(s.T(), db, database.ParameterValue{
			ScopeID: tpl.ID,
			Scope:   database.ParameterScopeTemplate,
		})
		check.Args(pv.ID).Asserts(tpl, rbac.ActionRead).Returns(pv)
	}))
	s.Run("ParameterValues", s.Subtest(func(db database.Store, check *MethodCase) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		a := dbgen.ParameterValue(s.T(), db, database.ParameterValue{
			ScopeID: tpl.ID,
			Scope:   database.ParameterScopeTemplate,
		})
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		b := dbgen.ParameterValue(s.T(), db, database.ParameterValue{
			ScopeID: w.ID,
			Scope:   database.ParameterScopeWorkspace,
		})
		check.Args(database.ParameterValuesParams{
			IDs: []uuid.UUID{a.ID, b.ID},
		}).Asserts(tpl, rbac.ActionRead, w, rbac.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("GetParameterSchemasByJobID", s.Subtest(func(db database.Store, check *MethodCase) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		tpl := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{JobID: j.ID, TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}})
		a := dbgen.ParameterSchema(s.T(), db, database.ParameterSchema{JobID: j.ID})
		check.Args(j.ID).Asserts(tv.RBACObject(tpl), rbac.ActionRead).
			Returns([]database.ParameterSchema{a})
	}))
	s.Run("Workspace/GetParameterValueByScopeAndName", s.Subtest(func(db database.Store, check *MethodCase) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		v := dbgen.ParameterValue(s.T(), db, database.ParameterValue{
			Scope:   database.ParameterScopeWorkspace,
			ScopeID: w.ID,
		})
		check.Args(database.GetParameterValueByScopeAndNameParams{
			Scope:   v.Scope,
			ScopeID: v.ScopeID,
			Name:    v.Name,
		}).Asserts(w, rbac.ActionRead).Returns(v)
	}))
	s.Run("Workspace/DeleteParameterValueByID", s.Subtest(func(db database.Store, check *MethodCase) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		v := dbgen.ParameterValue(s.T(), db, database.ParameterValue{
			Scope:   database.ParameterScopeWorkspace,
			ScopeID: w.ID,
		})
		check.Args(v.ID).Asserts(w, rbac.ActionUpdate).Returns()
	}))
}
