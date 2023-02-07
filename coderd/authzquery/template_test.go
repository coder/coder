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

func (s *MethodTestSuite) TestTemplate() {
	s.Run("GetPreviousTemplateVersion", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
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
	s.Run("GetTemplateAverageBuildTime", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(database.GetTemplateAverageBuildTimeParams{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			}), asserts(t1, rbac.ActionRead), nil)
		})
	})
	s.Run("GetTemplateByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(t1.ID), asserts(t1, rbac.ActionRead), values(t1))
		})
	})
	s.Run("GetTemplateByOrganizationAndName", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
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
	s.Run("GetTemplateDAUs", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(t1.ID), asserts(t1, rbac.ActionRead), nil)
		})
	})
	s.Run("GetTemplateVersionByJobID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(values(tv.JobID), asserts(t1, rbac.ActionRead), values(tv))
		})
	})
	s.Run("GetTemplateVersionByTemplateIDAndName", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
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
	s.Run("GetTemplateVersionParameters", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(values(tv.ID), asserts(t1, rbac.ActionRead), values([]database.TemplateVersionParameter{}))
		})
	})
	s.Run("GetTemplateGroupRoles", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(t1.ID), asserts(t1, rbac.ActionRead), nil)
		})
	})
	s.Run("GetTemplateUserRoles", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(t1.ID), asserts(t1, rbac.ActionRead), nil)
		})
	})
	s.Run("GetTemplateVersionByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			})
			return methodCase(values(tv.ID), asserts(t1, rbac.ActionRead), values(tv))
		})
	})
	s.Run("GetTemplateVersionsByIDs", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
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
	s.Run("GetTemplateVersionsByTemplateID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
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
	s.Run("GetTemplateVersionsCreatedAfter", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
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
	s.Run("GetTemplatesWithFilter", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a := dbgen.Template(t, db, database.Template{})
			// No asserts because SQLFilter.
			return methodCase(values(database.GetTemplatesWithFilterParams{}),
				asserts(), values(slice.New(a)))
		})
	})
	s.Run("GetAuthorizedTemplates", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a := dbgen.Template(t, db, database.Template{})
			// No asserts because SQLFilter.
			return methodCase(values(database.GetTemplatesWithFilterParams{}, emptyPreparedAuthorized{}),
				asserts(),
				values(slice.New(a)))
		})
	})
	s.Run("InsertTemplate", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			orgID := uuid.New()
			return methodCase(values(database.InsertTemplateParams{
				Provisioner:    "echo",
				OrganizationID: orgID,
			}), asserts(rbac.ResourceTemplate.InOrg(orgID), rbac.ActionCreate), nil)
		})
	})
	s.Run("InsertTemplateVersion", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(database.InsertTemplateVersionParams{
				TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true},
				OrganizationID: t1.OrganizationID,
			}), asserts(t1, rbac.ActionRead, t1, rbac.ActionCreate), nil)
		})
	})
	s.Run("SoftDeleteTemplateByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(t1.ID), asserts(t1, rbac.ActionDelete), nil)
		})
	})
	s.Run("UpdateTemplateACLByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(database.UpdateTemplateACLByIDParams{
				ID: t1.ID,
			}), asserts(t1, rbac.ActionCreate), values(t1))
		})
	})
	s.Run("UpdateTemplateActiveVersionByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
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
	s.Run("UpdateTemplateDeletedByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(database.UpdateTemplateDeletedByIDParams{
				ID:      t1.ID,
				Deleted: true,
			}), asserts(t1, rbac.ActionDelete), values())
		})
	})
	s.Run("UpdateTemplateMetaByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			t1 := dbgen.Template(t, db, database.Template{})
			return methodCase(values(database.UpdateTemplateMetaByIDParams{
				ID: t1.ID,
			}), asserts(t1, rbac.ActionUpdate), nil)
		})
	})
	s.Run("UpdateTemplateVersionByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
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
	s.Run("UpdateTemplateVersionDescriptionByJobID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
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
