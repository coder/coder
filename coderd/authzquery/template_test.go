package authzquery_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
)

func (suite *MethodTestSuite) TestTemplate() {
	suite.Run("GetPreviousTemplateVersion", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			tvid := uuid.New()
			now := time.Now()
			o1 := dbgen.Organization(t, db, database.Organization{})
			t1 := dbgen.Template(t, db, database.Template{
				OrganizationID:  o1.ID,
				ActiveVersionID: tvid,
			})
			_ = dbgen.TemplateVersion(t, db, database.TemplateVersion{
				CreatedAt:      now.Add(-time.Hour),
				ID:             tvid,
				Name:           t1.Name,
				OrganizationID: o1.ID,
				TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true}})
			_ = dbgen.TemplateVersion(t, db, database.TemplateVersion{
				CreatedAt:      now.Add(-2 * time.Hour),
				Name:           t1.Name,
				OrganizationID: o1.ID,
				TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true}})
			return methodCase(inputs(database.GetPreviousTemplateVersionParams{
				Name:           t1.Name,
				OrganizationID: o1.ID,
				TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true},
			}), asserts(t1, rbac.ActionRead))
		})
	})
	suite.Run("GetTemplateAverageBuildTime", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(inputs(database.GetTemplateAverageBuildTimeParams{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			}), asserts(t1, rbac.ActionRead))
		})
	})
	suite.Run("GetTemplateByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(inputs(t1.ID), asserts(t1, rbac.ActionRead))
		})
	})
	suite.Run("GetTemplateByOrganizationAndName", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o1 := dbgen.Organization(t, db, database.Organization{})
			t1 := dbgen.Template(t, db, database.Template{
				OrganizationID: o1.ID,
			})
			return methodCase(inputs(database.GetTemplateByOrganizationAndNameParams{
				Name:           t1.Name,
				OrganizationID: o1.ID,
			}), asserts(t1, rbac.ActionRead))
		})
	})
	suite.Run("GetTemplateDAUs", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(inputs(t1.ID), asserts(t1, rbac.ActionRead))
		})
	})
	suite.Run("GetTemplateVersionByJobID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(inputs(tv.JobID), asserts(t1, rbac.ActionRead))
		})
	})
	suite.Run("GetTemplateVersionByTemplateIDAndName", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(inputs(database.GetTemplateVersionByTemplateIDAndNameParams{
				Name:       tv.Name,
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			}), asserts(t1, rbac.ActionRead))
		})
	})
	suite.Run("GetTemplateVersionParameters", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(inputs(tv.ID), asserts(t1, rbac.ActionRead))
		})
	})
	suite.Run("GetTemplateGroupRoles", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(inputs(t1.ID), asserts(t1, rbac.ActionRead))
		})
	})
	suite.Run("GetTemplateUserRoles", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(inputs(t1.ID), asserts(t1, rbac.ActionRead))
		})
	})
	suite.Run("GetTemplateVersionByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(inputs(tv.ID), asserts(t1, rbac.ActionRead))
		})
	})
	suite.Run("GetTemplateVersionsByIDs", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			t2 := dbgen.Template(t, db, database.Template{})
			tv1 := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			tv2 := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t2.ID, Valid: true},
			})
			return methodCase(inputs([]uuid.UUID{tv1.ID, tv2.ID}),
				asserts(t1, rbac.ActionRead, t2, rbac.ActionRead))
		})
	})
	suite.Run("GetTemplateVersionsByTemplateID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			_ = dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			_ = dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(inputs(database.GetTemplateVersionsByTemplateIDParams{
				TemplateID: t1.ID,
			}), asserts(t1, rbac.ActionRead))
		})
	})
	suite.Run("GetTemplateVersionsCreatedAfter", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			now := time.Now()
			t1 := dbgen.Template(t, db, database.Template{})
			_ = dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
				CreatedAt:  now.Add(-time.Hour),
			})
			_ = dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
				CreatedAt:  now.Add(-2 * time.Hour),
			})
			return methodCase(inputs(now.Add(-time.Hour)), asserts(rbac.ResourceTemplate.All(), rbac.ActionRead))
		})
	})
	suite.Run("GetTemplatesWithFilter", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.Template(t, db, database.Template{})
			// No asserts because SQLFilter.
			return methodCase(inputs(database.GetTemplatesWithFilterParams{}), asserts())
		})
	})
	suite.Run("InsertTemplate", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			orgID := uuid.New()
			return methodCase(inputs(database.InsertTemplateParams{
				Provisioner:    "echo",
				OrganizationID: orgID,
			}), asserts(rbac.ResourceTemplate.InOrg(orgID), rbac.ActionCreate))
		})
	})
	suite.Run("InsertTemplateVersion", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(inputs(database.InsertTemplateVersionParams{
				TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true},
				OrganizationID: t1.OrganizationID,
			}), asserts(t1, rbac.ActionRead, t1, rbac.ActionCreate))
		})
	})
	suite.Run("SoftDeleteTemplateByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(inputs(t1.ID), asserts(t1, rbac.ActionDelete))
		})
	})
	suite.Run("UpdateTemplateACLByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(inputs(database.UpdateTemplateACLByIDParams{
				ID: t1.ID,
			}), asserts(t1, rbac.ActionCreate))
		})
	})
	suite.Run("UpdateTemplateActiveVersionByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(inputs(database.UpdateTemplateActiveVersionByIDParams{
				ID:              t1.ID,
				ActiveVersionID: tv.ID,
			}), asserts(t1, rbac.ActionUpdate))
		})
	})
	suite.Run("UpdateTemplateDeletedByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(inputs(database.UpdateTemplateDeletedByIDParams{
				ID:      t1.ID,
				Deleted: true,
			}), asserts(t1, rbac.ActionDelete))
		})
	})
	suite.Run("UpdateTemplateMetaByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(inputs(database.UpdateTemplateMetaByIDParams{
				ID:   t1.ID,
				Name: "foo",
			}), asserts(t1, rbac.ActionUpdate))
		})
	})
	suite.Run("UpdateTemplateVersionByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(inputs(database.UpdateTemplateVersionByIDParams{
				ID:         tv.ID,
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			}), asserts(t1, rbac.ActionUpdate))
		})
	})
	suite.Run("UpdateTemplateVersionDescriptionByJobID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			jobID := uuid.New()
			t1 := dbgen.Template(t, db, database.Template{})
			_ = dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
				JobID:      jobID,
			})
			return methodCase(inputs(database.UpdateTemplateVersionDescriptionByJobIDParams{
				JobID:  jobID,
				Readme: "foo",
			}), asserts(t1, rbac.ActionUpdate))
		})
	})
}
