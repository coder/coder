package authzquery_test

import (
	"testing"

	"github.com/coder/coder/coderd/util/slice"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database/dbgen"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (suite *MethodTestSuite) TestParameters() {
	suite.Run("Workspace/InsertParameterValue", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			w := dbgen.Workspace(t, db, database.Workspace{})
			return methodCase(values(database.InsertParameterValueParams{
				ScopeID:           w.ID,
				Scope:             database.ParameterScopeWorkspace,
				SourceScheme:      database.ParameterSourceSchemeNone,
				DestinationScheme: database.ParameterDestinationSchemeNone,
			}), asserts(w, rbac.ActionUpdate), nil)
		})
	})
	suite.Run("TemplateVersionNoTemplate/InsertParameterValue", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			j := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{})
			v := dbgen.TemplateVersion(t, db, database.TemplateVersion{JobID: j.ID, TemplateID: uuid.NullUUID{Valid: false}})
			return methodCase(values(database.InsertParameterValueParams{
				ScopeID:           j.ID,
				Scope:             database.ParameterScopeImportJob,
				SourceScheme:      database.ParameterSourceSchemeNone,
				DestinationScheme: database.ParameterDestinationSchemeNone,
			}), asserts(v.RBACObjectNoTemplate(), rbac.ActionUpdate), nil)
		})
	})
	suite.Run("TemplateVersionTemplate/InsertParameterValue", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			j := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{})
			tpl := dbgen.Template(t, db, database.Template{})
			v := dbgen.TemplateVersion(t, db, database.TemplateVersion{JobID: j.ID,
				TemplateID: uuid.NullUUID{
					UUID:  tpl.ID,
					Valid: true,
				}},
			)
			return methodCase(values(database.InsertParameterValueParams{
				ScopeID:           j.ID,
				Scope:             database.ParameterScopeImportJob,
				SourceScheme:      database.ParameterSourceSchemeNone,
				DestinationScheme: database.ParameterDestinationSchemeNone,
			}), asserts(v.RBACObject(tpl), rbac.ActionUpdate), nil)
		})
	})
	suite.Run("Template/InsertParameterValue", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			tpl := dbgen.Template(t, db, database.Template{})
			return methodCase(values(database.InsertParameterValueParams{
				ScopeID:           tpl.ID,
				Scope:             database.ParameterScopeTemplate,
				SourceScheme:      database.ParameterSourceSchemeNone,
				DestinationScheme: database.ParameterDestinationSchemeNone,
			}), asserts(tpl, rbac.ActionUpdate), nil)
		})
	})
	suite.Run("Template/ParameterValue", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			tpl := dbgen.Template(t, db, database.Template{})
			pv := dbgen.ParameterValue(t, db, database.ParameterValue{
				ScopeID: tpl.ID,
				Scope:   database.ParameterScopeTemplate,
			})
			return methodCase(values(pv.ID), asserts(tpl, rbac.ActionRead), values(pv))
		})
	})
	suite.Run("ParameterValues", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			tpl := dbgen.Template(t, db, database.Template{})
			a := dbgen.ParameterValue(t, db, database.ParameterValue{
				ScopeID: tpl.ID,
				Scope:   database.ParameterScopeTemplate,
			})
			w := dbgen.Workspace(t, db, database.Workspace{})
			b := dbgen.ParameterValue(t, db, database.ParameterValue{
				ScopeID: w.ID,
				Scope:   database.ParameterScopeWorkspace,
			})
			return methodCase(values(database.ParameterValuesParams{
				IDs: []uuid.UUID{a.ID, b.ID},
			}), asserts(tpl, rbac.ActionRead, w, rbac.ActionRead), values(slice.New(a, b)))
		})
	})
	suite.Run("GetParameterSchemasByJobID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			j := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{})
			tpl := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{JobID: j.ID, TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}})
			a := dbgen.ParameterSchema(t, db, database.ParameterSchema{JobID: j.ID})
			return methodCase(values(j.ID), asserts(tv.RBACObject(tpl), rbac.ActionRead),
				values([]database.ParameterSchema{a}))
		})
	})
	suite.Run("Workspace/GetParameterValueByScopeAndName", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			w := dbgen.Workspace(t, db, database.Workspace{})
			v := dbgen.ParameterValue(t, db, database.ParameterValue{
				Scope:   database.ParameterScopeWorkspace,
				ScopeID: w.ID,
			})
			return methodCase(values(database.GetParameterValueByScopeAndNameParams{
				Scope:   v.Scope,
				ScopeID: v.ScopeID,
				Name:    v.Name,
			}), asserts(w, rbac.ActionRead), values(v))
		})
	})
	suite.Run("Workspace/DeleteParameterValueByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			w := dbgen.Workspace(t, db, database.Workspace{})
			v := dbgen.ParameterValue(t, db, database.ParameterValue{
				Scope:   database.ParameterScopeWorkspace,
				ScopeID: w.ID,
			})
			return methodCase(values(v.ID), asserts(w, rbac.ActionUpdate), values())
		})
	})
}
