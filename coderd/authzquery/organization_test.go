package authzquery_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

func (s *MethodTestSuite) TestOrganization() {
	s.Run("GetGroupsByOrganizationID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			a := dbgen.Group(t, db, database.Group{OrganizationID: o.ID})
			b := dbgen.Group(t, db, database.Group{OrganizationID: o.ID})
			return methodCase(values(o.ID), asserts(a, rbac.ActionRead, b, rbac.ActionRead),
				values([]database.Group{a, b}))
		})
	})
	s.Run("GetOrganizationByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			return methodCase(values(o.ID), asserts(o, rbac.ActionRead), values(o))
		})
	})
	s.Run("GetOrganizationByName", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			return methodCase(values(o.Name), asserts(o, rbac.ActionRead), values(o))
		})
	})
	s.Run("GetOrganizationIDsByMemberIDs", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			oa := dbgen.Organization(t, db, database.Organization{})
			ob := dbgen.Organization(t, db, database.Organization{})
			ma := dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: oa.ID})
			mb := dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: ob.ID})
			return methodCase(values([]uuid.UUID{ma.UserID, mb.UserID}),
				asserts(rbac.ResourceUser.WithID(ma.UserID), rbac.ActionRead, rbac.ResourceUser.WithID(mb.UserID), rbac.ActionRead),
				nil)
		})
	})
	s.Run("GetOrganizationMemberByUserID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			mem := dbgen.OrganizationMember(t, db, database.OrganizationMember{})
			return methodCase(values(database.GetOrganizationMemberByUserIDParams{
				OrganizationID: mem.OrganizationID,
				UserID:         mem.UserID,
			}), asserts(mem, rbac.ActionRead),
				values(mem))
		})
	})
	s.Run("GetOrganizationMembershipsByUserID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			a := dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: u.ID})
			b := dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: u.ID})
			return methodCase(values(u.ID), asserts(a, rbac.ActionRead, b, rbac.ActionRead),
				values(slice.New(a, b)))
		})
	})
	s.Run("GetOrganizations", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a := dbgen.Organization(t, db, database.Organization{})
			b := dbgen.Organization(t, db, database.Organization{})
			return methodCase(values(), asserts(a, rbac.ActionRead, b, rbac.ActionRead),
				values(slice.New(a, b)))
		})
	})
	s.Run("GetOrganizationsByUserID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			a := dbgen.Organization(t, db, database.Organization{})
			_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: u.ID, OrganizationID: a.ID})
			b := dbgen.Organization(t, db, database.Organization{})
			_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: u.ID, OrganizationID: b.ID})
			return methodCase(values(u.ID), asserts(a, rbac.ActionRead, b, rbac.ActionRead),
				values(slice.New(a, b)))
		})
	})
	s.Run("InsertOrganization", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertOrganizationParams{
				ID:   uuid.New(),
				Name: "random",
			}), asserts(rbac.ResourceOrganization, rbac.ActionCreate), nil)
		})
	})
	s.Run("InsertOrganizationMember", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			u := dbgen.User(t, db, database.User{})

			return methodCase(values(database.InsertOrganizationMemberParams{
				OrganizationID: o.ID,
				UserID:         u.ID,
				Roles:          []string{rbac.RoleOrgAdmin(o.ID)},
			}), asserts(
				rbac.ResourceRoleAssignment.InOrg(o.ID), rbac.ActionCreate,
				rbac.ResourceOrganizationMember.InOrg(o.ID).WithID(u.ID), rbac.ActionCreate),
				nil)
		})
	})
	s.Run("UpdateMemberRoles", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			u := dbgen.User(t, db, database.User{})
			mem := dbgen.OrganizationMember(t, db, database.OrganizationMember{
				OrganizationID: o.ID,
				UserID:         u.ID,
				Roles:          []string{rbac.RoleOrgAdmin(o.ID)},
			})
			out := mem
			out.Roles = []string{}

			return methodCase(values(database.UpdateMemberRolesParams{
				GrantedRoles: []string{},
				UserID:       u.ID,
				OrgID:        o.ID,
			}), asserts(
				mem, rbac.ActionRead,
				rbac.ResourceRoleAssignment.InOrg(o.ID), rbac.ActionCreate, // org-mem
				rbac.ResourceRoleAssignment.InOrg(o.ID), rbac.ActionDelete, // org-admin
			), values(out))
		})
	})
}
