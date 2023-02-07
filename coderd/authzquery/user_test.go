package authzquery_test

import (
	"time"

	"github.com/coder/coder/coderd/util/slice"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
)

func (s *MethodTestSuite) TestUser() {
	s.Run("DeleteAPIKeysByUserID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(rbac.ResourceAPIKey.WithOwner(u.ID.String()), rbac.ActionDelete).Returns()
	}))
	s.Run("GetQuotaAllowanceForUser", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(u, rbac.ActionRead).Returns(int64(0))
	}))
	s.Run("GetQuotaConsumedForUser", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(u, rbac.ActionRead).Returns(int64(0))
	}))
	s.Run("GetUserByEmailOrUsername", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.GetUserByEmailOrUsernameParams{
			Username: u.Username,
			Email:    u.Email,
		}).Asserts(u, rbac.ActionRead).Returns(u)
	}))
	s.Run("GetUserByID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(u, rbac.ActionRead).Returns(u)
	}))
	s.Run("GetAuthorizedUserCount", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.User(s.T(), db, database.User{})
		check.Args(database.GetFilteredUserCountParams{}, emptyPreparedAuthorized{}).Asserts().Returns(int64(1))
	}))
	s.Run("GetFilteredUserCount", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.User(s.T(), db, database.User{})
		check.Args(database.GetFilteredUserCountParams{}).Asserts().Returns(int64(1))
	}))
	s.Run("GetUsers", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.User(s.T(), db, database.User{CreatedAt: database.Now().Add(-time.Hour)})
		b := dbgen.User(s.T(), db, database.User{CreatedAt: database.Now()})
		check.Args(database.GetUsersParams{}).
			Asserts(a, rbac.ActionRead, b, rbac.ActionRead)
	}))
	s.Run("GetUsersWithCount", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.User(s.T(), db, database.User{CreatedAt: database.Now().Add(-time.Hour)})
		b := dbgen.User(s.T(), db, database.User{CreatedAt: database.Now()})
		check.Args(database.GetUsersParams{}).Asserts(a, rbac.ActionRead, b, rbac.ActionRead)
	}))
	s.Run("GetUsersByIDs", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.User(s.T(), db, database.User{CreatedAt: database.Now().Add(-time.Hour)})
		b := dbgen.User(s.T(), db, database.User{CreatedAt: database.Now()})
		check.Args([]uuid.UUID{a.ID, b.ID}).
			Asserts(a, rbac.ActionRead, b, rbac.ActionRead).
			Returns(slice.New(a, b))
	}))
	s.Run("InsertUser", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertUserParams{
			ID:        uuid.New(),
			LoginType: database.LoginTypePassword,
		}).Asserts(rbac.ResourceRoleAssignment, rbac.ActionCreate, rbac.ResourceUser, rbac.ActionCreate)
	}))
	s.Run("InsertUserLink", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertUserLinkParams{
			UserID:    u.ID,
			LoginType: database.LoginTypeOIDC,
		}).Asserts(u, rbac.ActionUpdate)
	}))
	s.Run("SoftDeleteUserByID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(u, rbac.ActionDelete).Returns()
	}))
	s.Run("UpdateUserDeletedByID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{Deleted: true})
		check.Args(database.UpdateUserDeletedByIDParams{
			ID:      u.ID,
			Deleted: true,
		}).Asserts(u, rbac.ActionDelete).Returns()
	}))
	s.Run("UpdateUserHashedPassword", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserHashedPasswordParams{
			ID: u.ID,
		}).Asserts(u.UserDataRBACObject(), rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateUserLastSeenAt", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserLastSeenAtParams{
			ID:         u.ID,
			UpdatedAt:  u.UpdatedAt,
			LastSeenAt: u.LastSeenAt,
		}).Asserts(u, rbac.ActionUpdate).Returns(u)
	}))
	s.Run("UpdateUserProfile", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserProfileParams{
			ID:        u.ID,
			Email:     u.Email,
			Username:  u.Username,
			UpdatedAt: u.UpdatedAt,
		}).Asserts(u.UserDataRBACObject(), rbac.ActionUpdate).Returns(u)
	}))
	s.Run("UpdateUserStatus", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserStatusParams{
			ID:        u.ID,
			Status:    u.Status,
			UpdatedAt: u.UpdatedAt,
		}).Asserts(u, rbac.ActionUpdate).Returns(u)
	}))
	s.Run("DeleteGitSSHKey", s.Subtest(func(db database.Store, check *expects) {
		key := dbgen.GitSSHKey(s.T(), db, database.GitSSHKey{})
		check.Args(key.UserID).Asserts(key, rbac.ActionDelete).Returns()
	}))
	s.Run("GetGitSSHKey", s.Subtest(func(db database.Store, check *expects) {
		key := dbgen.GitSSHKey(s.T(), db, database.GitSSHKey{})
		check.Args(key.UserID).Asserts(key, rbac.ActionRead).Returns(key)
	}))
	s.Run("InsertGitSSHKey", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertGitSSHKeyParams{
			UserID: u.ID,
		}).Asserts(rbac.ResourceUserData.WithID(u.ID).WithOwner(u.ID.String()), rbac.ActionCreate)
	}))
	s.Run("UpdateGitSSHKey", s.Subtest(func(db database.Store, check *expects) {
		key := dbgen.GitSSHKey(s.T(), db, database.GitSSHKey{})
		check.Args(database.UpdateGitSSHKeyParams{
			UserID:    key.UserID,
			UpdatedAt: key.UpdatedAt,
		}).Asserts(key, rbac.ActionUpdate).Returns(key)
	}))
	s.Run("GetGitAuthLink", s.Subtest(func(db database.Store, check *expects) {
		link := dbgen.GitAuthLink(s.T(), db, database.GitAuthLink{})
		check.Args(database.GetGitAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		}).Asserts(link, rbac.ActionRead).Returns(link)
	}))
	s.Run("InsertGitAuthLink", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertGitAuthLinkParams{
			ProviderID: uuid.NewString(),
			UserID:     u.ID,
		}).Asserts(rbac.ResourceUserData.WithOwner(u.ID.String()).WithID(u.ID), rbac.ActionCreate)
	}))
	s.Run("UpdateGitAuthLink", s.Subtest(func(db database.Store, check *expects) {
		link := dbgen.GitAuthLink(s.T(), db, database.GitAuthLink{})
		check.Args(database.UpdateGitAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		}).Asserts(link, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateUserLink", s.Subtest(func(db database.Store, check *expects) {
		link := dbgen.UserLink(s.T(), db, database.UserLink{})
		check.Args(database.UpdateUserLinkParams{
			OAuthAccessToken:  link.OAuthAccessToken,
			OAuthRefreshToken: link.OAuthRefreshToken,
			OAuthExpiry:       link.OAuthExpiry,
			UserID:            link.UserID,
			LoginType:         link.LoginType,
		}).Asserts(link, rbac.ActionUpdate).Returns(link)
	}))
	s.Run("UpdateUserRoles", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{RBACRoles: []string{rbac.RoleTemplateAdmin()}})
		o := u
		o.RBACRoles = []string{rbac.RoleUserAdmin()}
		check.Args(database.UpdateUserRolesParams{
			GrantedRoles: []string{rbac.RoleUserAdmin()},
			ID:           u.ID,
		}).Asserts(
			u, rbac.ActionRead,
			rbac.ResourceRoleAssignment, rbac.ActionCreate,
			rbac.ResourceRoleAssignment, rbac.ActionDelete,
		).Returns(o)
	}))
}
