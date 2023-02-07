package authzquery_test

import (
	"time"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

func (s *MethodTestSuite) TestAPIKey() {
	s.Run("DeleteAPIKeyByID", s.Subtest(func(db database.Store, check *expects) {
		key, _ := dbgen.APIKey(s.T(), db, database.APIKey{})
		check.Args(key.ID).Asserts(key, rbac.ActionDelete).Returns()
	}))
	s.Run("GetAPIKeyByID", s.Subtest(func(db database.Store, check *expects) {
		key, _ := dbgen.APIKey(s.T(), db, database.APIKey{})
		check.Args(key.ID).Asserts(key, rbac.ActionRead).Returns(key)
	}))
	s.Run("GetAPIKeysByLoginType", s.Subtest(func(db database.Store, check *expects) {
		a, _ := dbgen.APIKey(s.T(), db, database.APIKey{LoginType: database.LoginTypePassword})
		b, _ := dbgen.APIKey(s.T(), db, database.APIKey{LoginType: database.LoginTypePassword})
		_, _ = dbgen.APIKey(s.T(), db, database.APIKey{LoginType: database.LoginTypeGithub})
		check.Args(database.LoginTypePassword).
			Asserts(a, rbac.ActionRead, b, rbac.ActionRead).
			Returns(slice.New(a, b))
	}))
	s.Run("GetAPIKeysLastUsedAfter", s.Subtest(func(db database.Store, check *expects) {
		a, _ := dbgen.APIKey(s.T(), db, database.APIKey{LastUsed: time.Now().Add(time.Hour)})
		b, _ := dbgen.APIKey(s.T(), db, database.APIKey{LastUsed: time.Now().Add(time.Hour)})
		_, _ = dbgen.APIKey(s.T(), db, database.APIKey{LastUsed: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).
			Asserts(a, rbac.ActionRead, b, rbac.ActionRead).
			Returns(slice.New(a, b))
	}))
	s.Run("InsertAPIKey", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertAPIKeyParams{
			UserID:    u.ID,
			LoginType: database.LoginTypePassword,
			Scope:     database.APIKeyScopeAll,
		}).Asserts(rbac.ResourceAPIKey.WithOwner(u.ID.String()), rbac.ActionCreate)
	}))
	s.Run("UpdateAPIKeyByID", s.Subtest(func(db database.Store, check *expects) {
		a, _ := dbgen.APIKey(s.T(), db, database.APIKey{})
		check.Args(database.UpdateAPIKeyByIDParams{
			ID: a.ID,
		}).Asserts(a, rbac.ActionUpdate).Returns()
	}))
}
