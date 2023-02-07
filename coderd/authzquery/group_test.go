package authzquery_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

func (s *MethodTestSuite) TestGroup() {
	s.Run("DeleteGroupByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			g := dbgen.Group(t, db, database.Group{})
			return methodCase(values(g.ID), asserts(g, rbac.ActionDelete), values())
		})
	})
	s.Run("DeleteGroupMemberFromGroup", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			g := dbgen.Group(t, db, database.Group{})
			m := dbgen.GroupMember(t, db, database.GroupMember{
				GroupID: g.ID,
			})
			return methodCase(values(database.DeleteGroupMemberFromGroupParams{
				UserID:  m.UserID,
				GroupID: g.ID,
			}), asserts(g, rbac.ActionUpdate), values())
		})
	})
	s.Run("GetGroupByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			g := dbgen.Group(t, db, database.Group{})
			return methodCase(values(g.ID), asserts(g, rbac.ActionRead), values(g))
		})
	})
	s.Run("GetGroupByOrgAndName", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			g := dbgen.Group(t, db, database.Group{})
			return methodCase(values(database.GetGroupByOrgAndNameParams{
				OrganizationID: g.OrganizationID,
				Name:           g.Name,
			}), asserts(g, rbac.ActionRead), values(g))
		})
	})
	s.Run("GetGroupMembers", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			g := dbgen.Group(t, db, database.Group{})
			_ = dbgen.GroupMember(t, db, database.GroupMember{})
			return methodCase(values(g.ID), asserts(g, rbac.ActionRead), nil)
		})
	})
	s.Run("InsertAllUsersGroup", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			return methodCase(values(o.ID), asserts(rbac.ResourceGroup.InOrg(o.ID), rbac.ActionCreate),
				nil)
		})
	})
	s.Run("InsertGroup", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			return methodCase(values(database.InsertGroupParams{
				OrganizationID: o.ID,
				Name:           "test",
			}), asserts(rbac.ResourceGroup.InOrg(o.ID), rbac.ActionCreate),
				nil)
		})
	})
	s.Run("InsertGroupMember", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			g := dbgen.Group(t, db, database.Group{})
			return methodCase(values(database.InsertGroupMemberParams{
				UserID:  uuid.New(),
				GroupID: g.ID,
			}), asserts(g, rbac.ActionUpdate),
				values())
		})
	})
	s.Run("InsertUserGroupsByName", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			u1 := dbgen.User(t, db, database.User{})
			g1 := dbgen.Group(t, db, database.Group{OrganizationID: o.ID})
			g2 := dbgen.Group(t, db, database.Group{OrganizationID: o.ID})
			_ = dbgen.GroupMember(t, db, database.GroupMember{GroupID: g1.ID, UserID: u1.ID})
			return methodCase(values(database.InsertUserGroupsByNameParams{
				OrganizationID: o.ID,
				UserID:         u1.ID,
				GroupNames:     slice.New(g1.Name, g2.Name),
			}), asserts(rbac.ResourceGroup.InOrg(o.ID), rbac.ActionUpdate), values())
		})
	})
	s.Run("DeleteGroupMembersByOrgAndUser", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			u1 := dbgen.User(t, db, database.User{})
			g1 := dbgen.Group(t, db, database.Group{OrganizationID: o.ID})
			g2 := dbgen.Group(t, db, database.Group{OrganizationID: o.ID})
			_ = dbgen.GroupMember(t, db, database.GroupMember{GroupID: g1.ID, UserID: u1.ID})
			_ = dbgen.GroupMember(t, db, database.GroupMember{GroupID: g2.ID, UserID: u1.ID})
			return methodCase(values(database.DeleteGroupMembersByOrgAndUserParams{
				OrganizationID: o.ID,
				UserID:         u1.ID,
			}), asserts(rbac.ResourceGroup.InOrg(o.ID), rbac.ActionUpdate), values())
		})
	})
	s.Run("UpdateGroupByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			g := dbgen.Group(t, db, database.Group{})
			return methodCase(values(database.UpdateGroupByIDParams{
				ID: g.ID,
			}), asserts(g, rbac.ActionUpdate), nil)
		})
	})
}
