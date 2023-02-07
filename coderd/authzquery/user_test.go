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
	s.Run("DeleteAPIKeysByUserID", func() {
		u := dbgen.User(s.T(), s.DB, database.User{})
		s.Args(u.ID).Asserts(rbac.ResourceAPIKey.WithOwner(u.ID.String()), rbac.ActionDelete).Returns()
	})
	s.Run("GetQuotaAllowanceForUser", func() {
		u := dbgen.User(s.T(), s.DB, database.User{})
		s.Args(u.ID).Asserts(u, rbac.ActionRead).Returns(int64(0))
	})
	s.Run("GetQuotaConsumedForUser", func() {
		u := dbgen.User(s.T(), s.DB, database.User{})
		s.Args(u.ID).Asserts(u, rbac.ActionRead).Returns(int64(0))
	})
	s.Run("GetUserByEmailOrUsername", func() {
		u := dbgen.User(s.T(), s.DB, database.User{})
		s.Args(database.GetUserByEmailOrUsernameParams{
			Username: u.Username,
			Email:    u.Email,
		}).Asserts(u, rbac.ActionRead).Returns(u)
	})
	s.Run("GetUserByID", func() {
		u := dbgen.User(s.T(), s.DB, database.User{})
		s.Args(u.ID).Asserts(u, rbac.ActionRead).Returns(u)
	})
	s.Run("GetAuthorizedUserCount", func() {
		_ = dbgen.User(s.T(), s.DB, database.User{})
		s.Args(database.GetFilteredUserCountParams{}, emptyPreparedAuthorized{}).Asserts().Returns(int64(1))
	})
	s.Run("GetFilteredUserCount", func() {
		_ = dbgen.User(s.T(), s.DB, database.User{})
		s.Args(database.GetFilteredUserCountParams{}).Asserts().Returns(int64(1))
	})
	s.Run("GetUsers", func() {
		a := dbgen.User(s.T(), s.DB, database.User{CreatedAt: database.Now().Add(-time.Hour)})
		b := dbgen.User(s.T(), s.DB, database.User{CreatedAt: database.Now()})
		s.Args(database.GetUsersParams{}).Asserts(a, rbac.ActionRead, b, rbac.ActionRead).Returns(slice.New(a, b))
	})
	s.Run("GetUsersWithCount", func() {
		a := dbgen.User(s.T(), s.DB, database.User{CreatedAt: database.Now().Add(-time.Hour)})
		b := dbgen.User(s.T(), s.DB, database.User{CreatedAt: database.Now()})
		s.Args(database.GetUsersParams{}).Asserts(a, rbac.ActionRead, b, rbac.ActionRead).Returns(nil)
	})
	s.Run("GetUsersByIDs", func() {
		a := dbgen.User(s.T(), s.DB, database.User{CreatedAt: database.Now().Add(-time.Hour)})
		b := dbgen.User(s.T(), s.DB, database.User{CreatedAt: database.Now()})
		s.Args([]uuid.UUID{a.ID, b.ID}).Asserts(a, rbac.ActionRead, b, rbac.ActionRead).Returns(slice.New(a, b))
	})
	s.Run("InsertUser", func() {
		s.Args(database.InsertUserParams{
			ID:        uuid.New(),
			LoginType: database.LoginTypePassword,
		}).Asserts(rbac.ResourceRoleAssignment, rbac.ActionCreate, rbac.ResourceUser, rbac.ActionCreate).Returns()
	})
	s.Run("InsertUserLink", func() {
		u := dbgen.User(s.T(), s.DB, database.User{})
		s.Args(database.InsertUserLinkParams{
			UserID:    u.ID,
			LoginType: database.LoginTypeOIDC,
		}).Asserts(u, rbac.ActionUpdate).Returns()
	})
	s.Run("SoftDeleteUserByID", func() {
		u := dbgen.User(s.T(), s.DB, database.User{})
		s.Args(u.ID).Asserts(u, rbac.ActionDelete).Returns()
	})
	s.Run("UpdateUserDeletedByID", func() {
		u := dbgen.User(s.T(), s.DB, database.User{Deleted: true})
		s.Args(database.UpdateUserDeletedByIDParams{
			ID:      u.ID,
			Deleted: true,
		}).Asserts(u, rbac.ActionDelete).Returns()
	})
	s.Run("UpdateUserHashedPassword", func() {
		u := dbgen.User(s.T(), s.DB, database.User{})
		s.Args(database.UpdateUserHashedPasswordParams{
			ID: u.ID,
		}).Asserts(u.UserDataRBACObject(), rbac.ActionUpdate).Returns()
	})
	s.Run("UpdateUserLastSeenAt", func() {
		u := dbgen.User(s.T(), s.DB, database.User{})
		s.Args(database.UpdateUserLastSeenAtParams{
			ID:         u.ID,
			UpdatedAt:  u.UpdatedAt,
			LastSeenAt: u.LastSeenAt,
		}).Asserts(u, rbac.ActionUpdate).Returns()
	})
	s.Run("UpdateUserProfile", func() {
		u := dbgen.User(s.T(), s.DB, database.User{})
		s.Args(database.UpdateUserProfileParams{
			ID:        u.ID,
			Email:     u.Email,
			Username:  u.Username,
			UpdatedAt: u.UpdatedAt,
		}).Asserts(u.UserDataRBACObject(), rbac.ActionUpdate).Returns(u)
	})
	s.Run("UpdateUserStatus", func() {
		u := dbgen.User(s.T(), s.DB, database.User{})
		s.Args(database.UpdateUserStatusParams{
			ID:        u.ID,
			Status:    u.Status,
			UpdatedAt: u.UpdatedAt,
		}).Asserts(u, rbac.ActionUpdate).Returns(u)
	})
	s.Run("DeleteGitSSHKey", func() {
		key := dbgen.GitSSHKey(s.T(), s.DB, database.GitSSHKey{})
		s.Args(key.UserID).Asserts(key, rbac.ActionDelete).Returns()
	})
	s.Run("GetGitSSHKey", func() {
		key := dbgen.GitSSHKey(s.T(), s.DB, database.GitSSHKey{})
		s.Args(key.UserID).Asserts(key, rbac.ActionRead).Returns(key)
	})
	s.Run("InsertGitSSHKey", func() {
		u := dbgen.User(s.T(), s.DB, database.User{})
		s.Args(database.InsertGitSSHKeyParams{
			UserID: u.ID,
		}).Asserts(rbac.ResourceUserData.WithID(u.ID).WithOwner(u.ID.String()), rbac.ActionCreate).Returns(nil)
	})
	s.Run("UpdateGitSSHKey", func() {
		key := dbgen.GitSSHKey(s.T(), s.DB, database.GitSSHKey{})
		s.Args(database.UpdateGitSSHKeyParams{
			UserID:    key.UserID,
			UpdatedAt: key.UpdatedAt,
		}).Asserts(key, rbac.ActionUpdate).Returns(key)
	})
	s.Run("GetGitAuthLink", func() {
		link := dbgen.GitAuthLink(s.T(), s.DB, database.GitAuthLink{})
		s.Args(database.GetGitAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		}).Asserts(link, rbac.ActionRead).Returns(link)
	})
	s.Run("InsertGitAuthLink", func() {
		u := dbgen.User(s.T(), s.DB, database.User{})
		s.Args(database.InsertGitAuthLinkParams{
			ProviderID: uuid.NewString(),
			UserID:     u.ID,
		}).Asserts(rbac.ResourceUserData.WithOwner(u.ID.String()).WithID(u.ID), rbac.ActionCreate).Returns(nil)
	})
	s.Run("UpdateGitAuthLink", func() {
		link := dbgen.GitAuthLink(s.T(), s.DB, database.GitAuthLink{})
		s.Args(database.UpdateGitAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		}).Asserts(link, rbac.ActionUpdate).Returns()
	})
	s.Run("UpdateUserLink", func() {
		link := dbgen.UserLink(s.T(), s.DB, database.UserLink{})
		s.Args(database.UpdateUserLinkParams{
			OAuthAccessToken:  link.OAuthAccessToken,
			OAuthRefreshToken: link.OAuthRefreshToken,
			OAuthExpiry:       link.OAuthExpiry,
			UserID:            link.UserID,
			LoginType:         link.LoginType,
		}).Asserts(link, rbac.ActionUpdate).Returns(link)
	})
	s.Run("UpdateUserRoles", func() {
		u := dbgen.User(s.T(), s.DB, database.User{RBACRoles: []string{rbac.RoleTemplateAdmin()}})
		o := u
		o.RBACRoles = []string{rbac.RoleUserAdmin()}
		s.Args(database.UpdateUserRolesParams{
			GrantedRoles: []string{rbac.RoleUserAdmin()},
			ID:           u.ID,
		}).Asserts(
			u, rbac.ActionRead,
			rbac.ResourceRoleAssignment, rbac.ActionCreate,
			rbac.ResourceRoleAssignment, rbac.ActionDelete,
		).Returns(o)
	})
}
