package authzquery_test

import (
	"testing"
	"time"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

func (s *MethodTestSuite) TestAPIKey() {
	s.Run("DeleteAPIKeyByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			key, _ := dbgen.APIKey(t, db, database.APIKey{})
			return methodCase(values(key.ID), asserts(key, rbac.ActionDelete), values())
		})
	})
	s.Run("GetAPIKeyByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			key, _ := dbgen.APIKey(t, db, database.APIKey{})
			return methodCase(values(key.ID), asserts(key, rbac.ActionRead), values(key))
		})
	})
	s.Run("GetAPIKeysByLoginType", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a, _ := dbgen.APIKey(t, db, database.APIKey{LoginType: database.LoginTypePassword})
			b, _ := dbgen.APIKey(t, db, database.APIKey{LoginType: database.LoginTypePassword})
			_, _ = dbgen.APIKey(t, db, database.APIKey{LoginType: database.LoginTypeGithub})
			return methodCase(values(database.LoginTypePassword),
				asserts(a, rbac.ActionRead, b, rbac.ActionRead),
				values(slice.New(a, b)))
		})
	})
	s.Run("GetAPIKeysLastUsedAfter", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a, _ := dbgen.APIKey(t, db, database.APIKey{LastUsed: time.Now().Add(time.Hour)})
			b, _ := dbgen.APIKey(t, db, database.APIKey{LastUsed: time.Now().Add(time.Hour)})
			_, _ = dbgen.APIKey(t, db, database.APIKey{LastUsed: time.Now().Add(-time.Hour)})
			return methodCase(values(time.Now()),
				asserts(a, rbac.ActionRead, b, rbac.ActionRead),
				values(slice.New(a, b)))
		})
	})
	s.Run("InsertAPIKey", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.InsertAPIKeyParams{
				UserID:    u.ID,
				LoginType: database.LoginTypePassword,
				Scope:     database.APIKeyScopeAll,
			}), asserts(rbac.ResourceAPIKey.WithOwner(u.ID.String()), rbac.ActionCreate),
				nil)
		})
	})
	s.Run("UpdateAPIKeyByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			a, _ := dbgen.APIKey(t, db, database.APIKey{})
			return methodCase(values(database.UpdateAPIKeyByIDParams{
				ID: a.ID,
			}), asserts(a, rbac.ActionUpdate), values())
		})
	})
}
