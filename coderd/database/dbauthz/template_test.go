package dbauthz_test

import (
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

func (s *MethodTestSuite) TestTemplate() {
	s.Run("GetPreviousTemplateVersion", s.Subtest(func(db database.Store, check *expects) {
		tvid := uuid.New()
		now := time.Now()
		o1 := dbgen.Organization(s.T(), db, database.Organization{})
		t1 := dbgen.Template(s.T(), db, database.Template{
			OrganizationID:  o1.ID,
			ActiveVersionID: tvid,
		})
		_ = dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			CreatedAt:      now.Add(-time.Hour),
			ID:             tvid,
			Name:           t1.Name,
			OrganizationID: o1.ID,
			TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true}})
		b := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			CreatedAt:      now.Add(-2 * time.Hour),
			Name:           t1.Name,
			OrganizationID: o1.ID,
			TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true}})
		check.Args(database.GetPreviousTemplateVersionParams{
			Name:           t1.Name,
			OrganizationID: o1.ID,
			TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true},
		}).Asserts(t1, rbac.ActionRead).Returns(b)
	}))
	s.Run("GetTemplateAverageBuildTime", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.GetTemplateAverageBuildTimeParams{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		}).Asserts(t1, rbac.ActionRead)
	}))
	s.Run("GetTemplateByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(t1.ID).Asserts(t1, rbac.ActionRead).Returns(t1)
	}))
	s.Run("GetTemplateByOrganizationAndName", s.Subtest(func(db database.Store, check *expects) {
		o1 := dbgen.Organization(s.T(), db, database.Organization{})
		t1 := dbgen.Template(s.T(), db, database.Template{
			OrganizationID: o1.ID,
		})
		check.Args(database.GetTemplateByOrganizationAndNameParams{
			Name:           t1.Name,
			OrganizationID: o1.ID,
		}).Asserts(t1, rbac.ActionRead).Returns(t1)
	}))
	s.Run("GetTemplateDAUs", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(t1.ID).Asserts(t1, rbac.ActionRead)
	}))
	s.Run("GetTemplateVersionByJobID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(tv.JobID).Asserts(t1, rbac.ActionRead).Returns(tv)
	}))
	s.Run("GetTemplateVersionByTemplateIDAndName", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(database.GetTemplateVersionByTemplateIDAndNameParams{
			Name:       tv.Name,
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		}).Asserts(t1, rbac.ActionRead).Returns(tv)
	}))
	s.Run("GetTemplateVersionParameters", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(tv.ID).Asserts(t1, rbac.ActionRead).Returns([]database.TemplateVersionParameter{})
	}))
	s.Run("GetTemplateGroupRoles", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(t1.ID).Asserts(t1, rbac.ActionRead)
	}))
	s.Run("GetTemplateUserRoles", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(t1.ID).Asserts(t1, rbac.ActionRead)
	}))
	s.Run("GetTemplateVersionByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(tv.ID).Asserts(t1, rbac.ActionRead).Returns(tv)
	}))
	s.Run("GetTemplateVersionsByIDs", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		t2 := dbgen.Template(s.T(), db, database.Template{})
		tv1 := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		tv2 := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t2.ID, Valid: true},
		})
		tv3 := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t2.ID, Valid: true},
		})
		check.Args([]uuid.UUID{tv1.ID, tv2.ID, tv3.ID}).
			Asserts(t1, rbac.ActionRead, t2, rbac.ActionRead).
			Returns(slice.New(tv1, tv2, tv3))
	}))
	s.Run("GetTemplateVersionsByTemplateID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		a := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		b := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(database.GetTemplateVersionsByTemplateIDParams{
			TemplateID: t1.ID,
		}).Asserts(t1, rbac.ActionRead).
			Returns(slice.New(a, b))
	}))
	s.Run("GetTemplateVersionsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		now := time.Now()
		t1 := dbgen.Template(s.T(), db, database.Template{})
		_ = dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			CreatedAt:  now.Add(-time.Hour),
		})
		_ = dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			CreatedAt:  now.Add(-2 * time.Hour),
		})
		check.Args(now.Add(-time.Hour)).Asserts(rbac.ResourceTemplate.All(), rbac.ActionRead)
	}))
	s.Run("GetTemplatesWithFilter", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.Template(s.T(), db, database.Template{})
		// No asserts because SQLFilter.
		check.Args(database.GetTemplatesWithFilterParams{}).
			Asserts().Returns(slice.New(a))
	}))
	s.Run("GetAuthorizedTemplates", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.Template(s.T(), db, database.Template{})
		// No asserts because SQLFilter.
		check.Args(database.GetTemplatesWithFilterParams{}, emptyPreparedAuthorized{}).
			Asserts().
			Returns(slice.New(a))
	}))
	s.Run("InsertTemplate", s.Subtest(func(db database.Store, check *expects) {
		orgID := uuid.New()
		check.Args(database.InsertTemplateParams{
			Provisioner:    "echo",
			OrganizationID: orgID,
		}).Asserts(rbac.ResourceTemplate.InOrg(orgID), rbac.ActionCreate)
	}))
	s.Run("InsertTemplateVersion", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.InsertTemplateVersionParams{
			TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true},
			OrganizationID: t1.OrganizationID,
		}).Asserts(t1, rbac.ActionRead, t1, rbac.ActionCreate)
	}))
	s.Run("SoftDeleteTemplateByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(t1.ID).Asserts(t1, rbac.ActionDelete)
	}))
	s.Run("UpdateTemplateACLByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateACLByIDParams{
			ID: t1.ID,
		}).Asserts(t1, rbac.ActionCreate).Returns(t1)
	}))
	s.Run("UpdateTemplateActiveVersionByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{
			ActiveVersionID: uuid.New(),
		})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			ID:         t1.ActiveVersionID,
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(database.UpdateTemplateActiveVersionByIDParams{
			ID:              t1.ID,
			ActiveVersionID: tv.ID,
		}).Asserts(t1, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateTemplateDeletedByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateDeletedByIDParams{
			ID:      t1.ID,
			Deleted: true,
		}).Asserts(t1, rbac.ActionDelete).Returns()
	}))
	s.Run("UpdateTemplateMetaByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateMetaByIDParams{
			ID: t1.ID,
		}).Asserts(t1, rbac.ActionUpdate)
	}))
	s.Run("UpdateTemplateVersionByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(database.UpdateTemplateVersionByIDParams{
			ID:         tv.ID,
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		}).Asserts(t1, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateTemplateVersionDescriptionByJobID", s.Subtest(func(db database.Store, check *expects) {
		jobID := uuid.New()
		t1 := dbgen.Template(s.T(), db, database.Template{})
		_ = dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			JobID:      jobID,
		})
		check.Args(database.UpdateTemplateVersionDescriptionByJobIDParams{
			JobID:  jobID,
			Readme: "foo",
		}).Asserts(t1, rbac.ActionUpdate).Returns()
	}))
}
