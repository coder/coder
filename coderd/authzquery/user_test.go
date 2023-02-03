package authzquery_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
)

func (s *MethodTestSuite) TestUser() {
	s.Run("DeleteAPIKeysByUserID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(u.ID), asserts(rbac.ResourceAPIKey.WithOwner(u.ID.String()), rbac.ActionDelete), values())
		})
	})
	s.Run("GetQuotaAllowanceForUser", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(u.ID), asserts(u, rbac.ActionRead), values(0))
		})
	})
	s.Run("GetQuotaConsumedForUser", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(u.ID), asserts(u, rbac.ActionRead), values(0))
		})
	})
	s.Run("GetUserByEmailOrUsername", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.GetUserByEmailOrUsernameParams{
				Username: u.Username,
				Email:    u.Email,
			}), asserts(u, rbac.ActionRead), values(u))
		})
	})
	s.Run("GetUserByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(u.ID), asserts(u, rbac.ActionRead), values(u))
		})
	})
	s.Run("GetAuthorizedUserCount", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.User(t, db, database.User{})
			return methodCase(values(database.GetFilteredUserCountParams{}, emptyPreparedAuthorized{}), asserts(), values(1))
		})
	})
	s.Run("GetFilteredUserCount", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.User(t, db, database.User{})
			return methodCase(values(database.GetFilteredUserCountParams{}), asserts(), values(1))
		})
	})
	s.Run("GetUsers", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a := dbgen.User(t, db, database.User{})
			b := dbgen.User(t, db, database.User{})
			return methodCase(values(database.GetUsersParams{}), asserts(a, rbac.ActionRead, b, rbac.ActionRead))
		})
	})
	s.Run("GetUsersWithCount", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a := dbgen.User(t, db, database.User{})
			b := dbgen.User(t, db, database.User{})
			return methodCase(values(database.GetUsersParams{}), asserts(a, rbac.ActionRead, b, rbac.ActionRead))
		})
	})
	s.Run("GetUsersByIDs", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a := dbgen.User(t, db, database.User{})
			b := dbgen.User(t, db, database.User{})
			return methodCase(values([]uuid.UUID{a.ID, b.ID}), asserts(a, rbac.ActionRead, b, rbac.ActionRead))
		})
	})
	s.Run("InsertUser", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertUserParams{
				ID:        uuid.New(),
				LoginType: database.LoginTypePassword,
			}), asserts(rbac.ResourceRoleAssignment, rbac.ActionCreate, rbac.ResourceUser, rbac.ActionCreate))
		})
	})
	s.Run("InsertUserLink", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.InsertUserLinkParams{
				UserID:    u.ID,
				LoginType: database.LoginTypeOIDC,
			}), asserts(u, rbac.ActionUpdate))
		})
	})
	s.Run("SoftDeleteUserByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(u.ID), asserts(u, rbac.ActionDelete))
		})
	})
	s.Run("UpdateUserDeletedByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.UpdateUserDeletedByIDParams{
				ID:      u.ID,
				Deleted: true,
			}), asserts(u, rbac.ActionDelete))
		})
	})
	s.Run("UpdateUserHashedPassword", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.UpdateUserHashedPasswordParams{
				ID: u.ID,
			}), asserts(u.UserDataRBACObject(), rbac.ActionUpdate))
		})
	})
	s.Run("UpdateUserLastSeenAt", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.UpdateUserLastSeenAtParams{
				ID: u.ID,
			}), asserts(u, rbac.ActionUpdate))
		})
	})
	s.Run("UpdateUserProfile", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.UpdateUserProfileParams{
				ID: u.ID,
			}), asserts(u.UserDataRBACObject(), rbac.ActionUpdate))
		})
	})
	s.Run("UpdateUserStatus", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.UpdateUserStatusParams{
				ID:     u.ID,
				Status: database.UserStatusActive,
			}), asserts(u, rbac.ActionUpdate))
		})
	})
	s.Run("DeleteGitSSHKey", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			key := dbgen.GitSSHKey(t, db, database.GitSSHKey{})
			return methodCase(values(key.UserID), asserts(key, rbac.ActionDelete))
		})
	})
	s.Run("GetGitSSHKey", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			key := dbgen.GitSSHKey(t, db, database.GitSSHKey{})
			return methodCase(values(key.UserID), asserts(key, rbac.ActionRead))
		})
	})
	s.Run("InsertGitSSHKey", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.InsertGitSSHKeyParams{
				UserID: u.ID,
			}), asserts(rbac.ResourceUserData.WithID(u.ID).WithOwner(u.ID.String()), rbac.ActionCreate))
		})
	})
	s.Run("UpdateGitSSHKey", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			key := dbgen.GitSSHKey(t, db, database.GitSSHKey{})
			return methodCase(values(database.UpdateGitSSHKeyParams{
				UserID: key.UserID,
			}), asserts(key, rbac.ActionUpdate))
		})
	})
	s.Run("GetGitAuthLink", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			link := dbgen.GitAuthLink(t, db, database.GitAuthLink{})
			return methodCase(values(database.GetGitAuthLinkParams{
				ProviderID: link.ProviderID,
				UserID:     link.UserID,
			}), asserts(link, rbac.ActionRead))
		})
	})
	s.Run("InsertGitAuthLink", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.InsertGitAuthLinkParams{
				ProviderID: uuid.NewString(),
				UserID:     u.ID,
			}), asserts(rbac.ResourceUserData.WithOwner(u.ID.String()).WithID(u.ID), rbac.ActionCreate))
		})
	})
	s.Run("UpdateGitAuthLink", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			link := dbgen.GitAuthLink(t, db, database.GitAuthLink{})
			return methodCase(values(database.UpdateGitAuthLinkParams{
				ProviderID: link.ProviderID,
				UserID:     link.UserID,
			}), asserts(link, rbac.ActionUpdate))
		})
	})
	s.Run("UpdateUserLink", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			link := dbgen.UserLink(t, db, database.UserLink{})
			return methodCase(values(database.UpdateUserLinkParams{
				UserID:    link.UserID,
				LoginType: link.LoginType,
			}), asserts(link, rbac.ActionUpdate))
		})
	})
	s.Run("UpdateUserRoles", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{RBACRoles: []string{rbac.RoleTemplateAdmin()}})
			return methodCase(values(database.UpdateUserRolesParams{
				GrantedRoles: []string{rbac.RoleUserAdmin()},
				ID:           u.ID,
			}), asserts(
				u, rbac.ActionRead,
				rbac.ResourceRoleAssignment, rbac.ActionCreate,
				rbac.ResourceRoleAssignment, rbac.ActionDelete,
			))
		})
	})
}
