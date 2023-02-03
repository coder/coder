package authzquery_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
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
			b := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				CreatedAt:      now.Add(-2 * time.Hour),
				Name:           t1.Name,
				OrganizationID: o1.ID,
				TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true}})
			return methodCase(values(database.GetPreviousTemplateVersionParams{
				Name:           t1.Name,
				OrganizationID: o1.ID,
				TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true},
			}), asserts(t1, rbac.ActionRead), values(b))
		})
	})
	suite.Run("GetTemplateAverageBuildTime", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(database.GetTemplateAverageBuildTimeParams{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			}), asserts(t1, rbac.ActionRead), nil)
		})
	})
	suite.Run("GetTemplateByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(t1.ID), asserts(t1, rbac.ActionRead), values(t1))
		})
	})
	suite.Run("GetTemplateByOrganizationAndName", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o1 := dbgen.Organization(t, db, database.Organization{})
			t1 := dbgen.Template(t, db, database.Template{
				OrganizationID: o1.ID,
			})
			return methodCase(values(database.GetTemplateByOrganizationAndNameParams{
				Name:           t1.Name,
				OrganizationID: o1.ID,
			}), asserts(t1, rbac.ActionRead), values(t1))
		})
	})
	suite.Run("GetTemplateDAUs", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(t1.ID), asserts(t1, rbac.ActionRead), nil)
		})
	})
	suite.Run("GetTemplateVersionByJobID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(values(tv.JobID), asserts(t1, rbac.ActionRead), values(tv))
		})
	})
	suite.Run("GetTemplateVersionByTemplateIDAndName", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(values(database.GetTemplateVersionByTemplateIDAndNameParams{
				Name:       tv.Name,
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			}), asserts(t1, rbac.ActionRead), values(tv))
		})
	})
	suite.Run("GetTemplateVersionParameters", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(values(tv.ID), asserts(t1, rbac.ActionRead), values([]database.TemplateVersionParameter{}))
		})
	})
	suite.Run("GetTemplateGroupRoles", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(t1.ID), asserts(t1, rbac.ActionRead), nil)
		})
	})
	suite.Run("GetTemplateUserRoles", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(t1.ID), asserts(t1, rbac.ActionRead), nil)
		})
	})
	suite.Run("GetTemplateVersionByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(values(tv.ID), asserts(t1, rbac.ActionRead), values(tv))
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
			tv3 := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t2.ID, Valid: true},
			})
			return methodCase(values([]uuid.UUID{tv1.ID, tv2.ID, tv3.ID}),
				asserts(t1, rbac.ActionRead, t2, rbac.ActionRead),
				values(slice.New(tv1, tv2, tv3)))
		})
	})
	suite.Run("GetTemplateVersionsByTemplateID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			a := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			b := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(values(database.GetTemplateVersionsByTemplateIDParams{
				TemplateID: t1.ID,
			}), asserts(t1, rbac.ActionRead),
				values(slice.New(a, b)))
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
			return methodCase(values(now.Add(-time.Hour)), asserts(rbac.ResourceTemplate.All(), rbac.ActionRead), nil)
		})
	})
	suite.Run("GetTemplatesWithFilter", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a := dbgen.Template(t, db, database.Template{})
			// No asserts because SQLFilter.
			return methodCase(values(database.GetTemplatesWithFilterParams{}),
				asserts(), values(slice.New(a)))
		})
	})
	suite.Run("GetAuthorizedTemplates", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a := dbgen.Template(t, db, database.Template{})
			// No asserts because SQLFilter.
			return methodCase(values(database.GetTemplatesWithFilterParams{}, emptyPreparedAuthorized{}),
				asserts(),
				values(slice.New(a)))
		})
	})
	suite.Run("InsertTemplate", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			orgID := uuid.New()
			return methodCase(values(database.InsertTemplateParams{
				Provisioner:    "echo",
				OrganizationID: orgID,
			}), asserts(rbac.ResourceTemplate.InOrg(orgID), rbac.ActionCreate), nil)
		})
	})
	suite.Run("InsertTemplateVersion", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(database.InsertTemplateVersionParams{
				TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true},
				OrganizationID: t1.OrganizationID,
			}), asserts(t1, rbac.ActionRead, t1, rbac.ActionCreate), nil)
		})
	})
	suite.Run("SoftDeleteTemplateByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(t1.ID), asserts(t1, rbac.ActionDelete), nil)
		})
	})
	suite.Run("UpdateTemplateACLByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(database.UpdateTemplateACLByIDParams{
				ID: t1.ID,
			}), asserts(t1, rbac.ActionCreate), values(t1))
		})
	})
	suite.Run("UpdateTemplateActiveVersionByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{
				ActiveVersionID: uuid.New(),
			})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				ID:         t1.ActiveVersionID,
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(values(database.UpdateTemplateActiveVersionByIDParams{
				ID:              t1.ID,
				ActiveVersionID: tv.ID,
			}), asserts(t1, rbac.ActionUpdate), values())
		})
	})
	suite.Run("UpdateTemplateDeletedByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(database.UpdateTemplateDeletedByIDParams{
				ID:      t1.ID,
				Deleted: true,
			}), asserts(t1, rbac.ActionDelete), values())
		})
	})
	suite.Run("UpdateTemplateMetaByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(database.UpdateTemplateMetaByIDParams{
				ID: t1.ID,
			}), asserts(t1, rbac.ActionUpdate), nil)
		})
	})
	suite.Run("UpdateTemplateVersionByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(values(database.UpdateTemplateVersionByIDParams{
				ID:         tv.ID,
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			}), asserts(t1, rbac.ActionUpdate), values())
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
			return methodCase(values(database.UpdateTemplateVersionDescriptionByJobIDParams{
				JobID:  jobID,
				Readme: "foo",
			}), asserts(t1, rbac.ActionUpdate), values())
		})
	})
}
