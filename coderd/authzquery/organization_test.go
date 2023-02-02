package authzquery_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
)

func (suite *MethodTestSuite) TestOrganization() {
	suite.Run("GetGroupsByOrganizationID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			a := dbgen.Group(t, db, database.Group{OrganizationID: o.ID})
			b := dbgen.Group(t, db, database.Group{OrganizationID: o.ID})
			return methodCase(inputs(o.ID), asserts(a, rbac.ActionRead, b, rbac.ActionRead))
		})
	})
	suite.Run("GetOrganizationByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			return methodCase(inputs(o.ID), asserts(o, rbac.ActionRead))
		})
	})
	suite.Run("GetOrganizationByName", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			return methodCase(inputs(o.Name), asserts(o, rbac.ActionRead))
		})
	})
	suite.Run("GetOrganizationIDsByMemberIDs", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			u := dbgen.User(t, db, database.User{})
			var _ = o.ID
			// TODO: Implement this and do rbac check
			//mem := dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: o.ID, UserID: u.ID})
			return methodCase(inputs([]uuid.UUID{u.ID}), asserts())
		})
	})
	suite.Run("GetOrganizationMemberByUserID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			u := dbgen.User(t, db, database.User{})
			// TODO: Implement this and do rbac check
			//mem := dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: o.ID, UserID: u.ID})
			return methodCase(inputs(database.GetOrganizationMemberByUserIDParams{
				OrganizationID: o.ID,
				UserID:         u.ID,
			}), asserts())
		})
	})
	suite.Run("GetOrganizationMembershipsByUserID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			u := dbgen.User(t, db, database.User{})
			var _ = o.ID
			// TODO: Implement this and do rbac check
			//mem := dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: o.ID, UserID: u.ID})
			return methodCase(inputs(u.ID), asserts())
		})
	})
	suite.Run("GetOrganizations", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a := dbgen.Organization(t, db, database.Organization{})
			b := dbgen.Organization(t, db, database.Organization{})
			return methodCase(inputs(), asserts(a, rbac.ActionRead, b, rbac.ActionRead))
		})
	})
	suite.Run("GetOrganizationsByUserID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			u := dbgen.User(t, db, database.User{})
			var _ = o.ID
			// TODO: Implement this and do rbac check
			//mem := dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: o.ID, UserID: u.ID})
			return methodCase(inputs(u.ID), asserts(u, rbac.ActionRead))
		})
	})
	suite.Run("InsertOrganization", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(inputs(database.InsertOrganizationParams{
				ID:   uuid.New(),
				Name: "random",
			}), asserts(rbac.ResourceOrganization, rbac.ActionCreate))
		})
	})
	suite.Run("InsertOrganizationMember", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			u := dbgen.User(t, db, database.User{})

			return methodCase(inputs(database.InsertOrganizationMemberParams{
				OrganizationID: o.ID,
				UserID:         u.ID,
				Roles:          []string{rbac.RoleOrgAdmin(o.ID)},
			}), asserts(
				rbac.ResourceRoleAssignment.InOrg(o.ID), rbac.ActionCreate,
				rbac.ResourceOrganizationMember.InOrg(o.ID).WithID(u.ID), rbac.ActionCreate),
			)
		})
	})
	suite.Run("UpdateMemberRoles", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			o := dbgen.Organization(t, db, database.Organization{})
			u := dbgen.User(t, db, database.User{})
			// TODO: Implement this and do rbac check
			//mem := dbgen.OrganizationMember(t, db, database.OrganizationMember{
			//	OrganizationID: o.ID,
			//	UserID:         u.ID,
			//	Roles:          []string{rbac.RoleOrgAdmin(o.ID)},
			//})

			return methodCase(inputs(database.UpdateMemberRolesParams{
				GrantedRoles: []string{},
				UserID:       u.ID,
				OrgID:        o.ID,
			}), asserts(
				rbac.ResourceRoleAssignment.InOrg(o.ID), rbac.ActionDelete,
				rbac.ResourceOrganizationMember.InOrg(o.ID).WithID(u.ID), rbac.ActionCreate,
			))
		})
	})
}
