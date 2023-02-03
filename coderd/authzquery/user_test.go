package authzquery_test

import (
	"testing"
	"time"

	"github.com/coder/coder/coderd/util/slice"

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
			return methodCase(values(u.ID), asserts(u, rbac.ActionRead), values(int64(0)))
		})
	})
	s.Run("GetQuotaConsumedForUser", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(u.ID), asserts(u, rbac.ActionRead), values(int64(0)))
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
			return methodCase(values(database.GetFilteredUserCountParams{}, emptyPreparedAuthorized{}), asserts(), values(int64(1)))
		})
	})
	s.Run("GetFilteredUserCount", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.User(t, db, database.User{})
			return methodCase(values(database.GetFilteredUserCountParams{}), asserts(), values(int64(1)))
		})
	})
	s.Run("GetUsers", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a := dbgen.User(t, db, database.User{CreatedAt: database.Now().Add(-time.Hour)})
			b := dbgen.User(t, db, database.User{CreatedAt: database.Now()})
			return methodCase(values(database.GetUsersParams{}),
				asserts(a, rbac.ActionRead, b, rbac.ActionRead),
				nil)
		})
	})
	s.Run("GetUsersWithCount", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a := dbgen.User(t, db, database.User{CreatedAt: database.Now().Add(-time.Hour)})
			b := dbgen.User(t, db, database.User{CreatedAt: database.Now()})
			return methodCase(values(database.GetUsersParams{}), asserts(a, rbac.ActionRead, b, rbac.ActionRead), nil)
		})
	})
	s.Run("GetUsersByIDs", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a := dbgen.User(t, db, database.User{CreatedAt: database.Now().Add(-time.Hour)})
			b := dbgen.User(t, db, database.User{CreatedAt: database.Now()})
			return methodCase(values([]uuid.UUID{a.ID, b.ID}),
				asserts(a, rbac.ActionRead, b, rbac.ActionRead),
				values(slice.New(a, b)))
		})
	})
	s.Run("InsertUser", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertUserParams{
				ID:        uuid.New(),
				LoginType: database.LoginTypePassword,
			}), asserts(rbac.ResourceRoleAssignment, rbac.ActionCreate, rbac.ResourceUser, rbac.ActionCreate), nil)
		})
	})
	s.Run("InsertUserLink", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.InsertUserLinkParams{
				UserID:    u.ID,
				LoginType: database.LoginTypeOIDC,
			}), asserts(u, rbac.ActionUpdate), nil)
		})
	})
	s.Run("SoftDeleteUserByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(u.ID), asserts(u, rbac.ActionDelete), values())
		})
	})
	s.Run("UpdateUserDeletedByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{Deleted: true})
			return methodCase(values(database.UpdateUserDeletedByIDParams{
				ID:      u.ID,
				Deleted: true,
			}), asserts(u, rbac.ActionDelete), values())
		})
	})
	s.Run("UpdateUserHashedPassword", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.UpdateUserHashedPasswordParams{
				ID: u.ID,
			}), asserts(u.UserDataRBACObject(), rbac.ActionUpdate), values())
		})
	})
	s.Run("UpdateUserLastSeenAt", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.UpdateUserLastSeenAtParams{
				ID:         u.ID,
				UpdatedAt:  u.UpdatedAt,
				LastSeenAt: u.LastSeenAt,
			}), asserts(u, rbac.ActionUpdate), values(u))
		})
	})
	s.Run("UpdateUserProfile", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.UpdateUserProfileParams{
				ID:        u.ID,
				Email:     u.Email,
				Username:  u.Username,
				UpdatedAt: u.UpdatedAt,
			}), asserts(u.UserDataRBACObject(), rbac.ActionUpdate), values(u))
		})
	})
	s.Run("UpdateUserStatus", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.UpdateUserStatusParams{
				ID:        u.ID,
				Status:    u.Status,
				UpdatedAt: u.UpdatedAt,
			}), asserts(u, rbac.ActionUpdate), values(u))
		})
	})
	s.Run("DeleteGitSSHKey", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			key := dbgen.GitSSHKey(t, db, database.GitSSHKey{})
			return methodCase(values(key.UserID), asserts(key, rbac.ActionDelete), values())
		})
	})
	s.Run("GetGitSSHKey", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			key := dbgen.GitSSHKey(t, db, database.GitSSHKey{})
			return methodCase(values(key.UserID), asserts(key, rbac.ActionRead), values(key))
		})
	})
	s.Run("InsertGitSSHKey", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.InsertGitSSHKeyParams{
				UserID: u.ID,
			}), asserts(rbac.ResourceUserData.WithID(u.ID).WithOwner(u.ID.String()), rbac.ActionCreate), nil)
		})
	})
	s.Run("UpdateGitSSHKey", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			key := dbgen.GitSSHKey(t, db, database.GitSSHKey{})
			return methodCase(values(database.UpdateGitSSHKeyParams{
				UserID:    key.UserID,
				UpdatedAt: key.UpdatedAt,
			}), asserts(key, rbac.ActionUpdate), values(key))
		})
	})
	s.Run("GetGitAuthLink", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			link := dbgen.GitAuthLink(t, db, database.GitAuthLink{})
			return methodCase(values(database.GetGitAuthLinkParams{
				ProviderID: link.ProviderID,
				UserID:     link.UserID,
			}), asserts(link, rbac.ActionRead), values(link))
		})
	})
	s.Run("InsertGitAuthLink", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.InsertGitAuthLinkParams{
				ProviderID: uuid.NewString(),
				UserID:     u.ID,
			}), asserts(rbac.ResourceUserData.WithOwner(u.ID.String()).WithID(u.ID), rbac.ActionCreate), nil)
		})
	})
	s.Run("UpdateGitAuthLink", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			link := dbgen.GitAuthLink(t, db, database.GitAuthLink{})
			return methodCase(values(database.UpdateGitAuthLinkParams{
				ProviderID: link.ProviderID,
				UserID:     link.UserID,
			}), asserts(link, rbac.ActionUpdate), values())
		})
	})
	s.Run("UpdateUserLink", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			link := dbgen.UserLink(t, db, database.UserLink{})
			return methodCase(values(database.UpdateUserLinkParams{
				OAuthAccessToken:  link.OAuthAccessToken,
				OAuthRefreshToken: link.OAuthRefreshToken,
				OAuthExpiry:       link.OAuthExpiry,
				UserID:            link.UserID,
				LoginType:         link.LoginType,
			}), asserts(link, rbac.ActionUpdate), values(link))
		})
	})
	s.Run("UpdateUserRoles", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{RBACRoles: []string{rbac.RoleTemplateAdmin()}})
			o := u
			o.RBACRoles = []string{rbac.RoleUserAdmin()}
			return methodCase(values(database.UpdateUserRolesParams{
				GrantedRoles: []string{rbac.RoleUserAdmin()},
				ID:           u.ID,
			}), asserts(
				u, rbac.ActionRead,
				rbac.ResourceRoleAssignment, rbac.ActionCreate,
				rbac.ResourceRoleAssignment, rbac.ActionDelete,
			), values(o))
		})
	})
}
