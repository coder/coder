package dbauthz_test

import (
	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

func (s *MethodTestSuite) TestOrganization() {
	s.Run("GetGroupsByOrganizationID", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		a := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		b := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		check.Args(o.ID).Asserts(a, rbac.ActionRead, b, rbac.ActionRead).
			Returns([]database.Group{a, b})
	}))
	s.Run("GetOrganizationByID", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(o.ID).Asserts(o, rbac.ActionRead).Returns(o)
	}))
	s.Run("GetOrganizationByName", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(o.Name).Asserts(o, rbac.ActionRead).Returns(o)
	}))
	s.Run("GetOrganizationIDsByMemberIDs", s.Subtest(func(db database.Store, check *expects) {
		oa := dbgen.Organization(s.T(), db, database.Organization{})
		ob := dbgen.Organization(s.T(), db, database.Organization{})
		ma := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{OrganizationID: oa.ID})
		mb := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{OrganizationID: ob.ID})
		check.Args([]uuid.UUID{ma.UserID, mb.UserID}).
			Asserts(rbac.ResourceUser.WithID(ma.UserID), rbac.ActionRead, rbac.ResourceUser.WithID(mb.UserID), rbac.ActionRead)
	}))
	s.Run("GetOrganizationMemberByUserID", s.Subtest(func(db database.Store, check *expects) {
		mem := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{})
		check.Args(database.GetOrganizationMemberByUserIDParams{
			OrganizationID: mem.OrganizationID,
			UserID:         mem.UserID,
		}).Asserts(mem, rbac.ActionRead).Returns(mem)
	}))
	s.Run("GetOrganizationMembershipsByUserID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		a := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{UserID: u.ID})
		b := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{UserID: u.ID})
		check.Args(u.ID).Asserts(a, rbac.ActionRead, b, rbac.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("GetOrganizations", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.Organization(s.T(), db, database.Organization{})
		b := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args().Asserts(a, rbac.ActionRead, b, rbac.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("GetOrganizationsByUserID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		a := dbgen.Organization(s.T(), db, database.Organization{})
		_ = dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{UserID: u.ID, OrganizationID: a.ID})
		b := dbgen.Organization(s.T(), db, database.Organization{})
		_ = dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{UserID: u.ID, OrganizationID: b.ID})
		check.Args(u.ID).Asserts(a, rbac.ActionRead, b, rbac.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("InsertOrganization", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertOrganizationParams{
			ID:   uuid.New(),
			Name: "random",
		}).Asserts(rbac.ResourceOrganization, rbac.ActionCreate)
	}))
	s.Run("InsertOrganizationMember", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		u := dbgen.User(s.T(), db, database.User{})

		check.Args(database.InsertOrganizationMemberParams{
			OrganizationID: o.ID,
			UserID:         u.ID,
			Roles:          []string{rbac.RoleOrgAdmin(o.ID)},
		}).Asserts(
			rbac.ResourceRoleAssignment.InOrg(o.ID), rbac.ActionCreate,
			rbac.ResourceOrganizationMember.InOrg(o.ID).WithID(u.ID), rbac.ActionCreate)
	}))
	s.Run("UpdateMemberRoles", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		u := dbgen.User(s.T(), db, database.User{})
		mem := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{
			OrganizationID: o.ID,
			UserID:         u.ID,
			Roles:          []string{rbac.RoleOrgAdmin(o.ID)},
		})
		out := mem
		out.Roles = []string{}

		check.Args(database.UpdateMemberRolesParams{
			GrantedRoles: []string{},
			UserID:       u.ID,
			OrgID:        o.ID,
		}).Asserts(
			mem, rbac.ActionRead,
			rbac.ResourceRoleAssignment.InOrg(o.ID), rbac.ActionCreate, // org-mem
			rbac.ResourceRoleAssignment.InOrg(o.ID), rbac.ActionDelete, // org-admin
		).Returns(out)
	}))
}
