package authzquery_test

import (
	"testing"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
)

func (s *MethodTestSuite) TestFile() {
	s.Run("GetFileByHashAndCreator", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			f := dbgen.File(t, db, database.File{})
			return methodCase(values(database.GetFileByHashAndCreatorParams{
				Hash:      f.Hash,
				CreatedBy: f.CreatedBy,
			}), asserts(f, rbac.ActionRead), values(f))
		})
	})
	s.Run("GetFileByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			f := dbgen.File(t, db, database.File{})
			return methodCase(values(f.ID), asserts(f, rbac.ActionRead), values(f))
		})
	})
	s.Run("InsertFile", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(database.InsertFileParams{
				CreatedBy: u.ID,
			}), asserts(rbac.ResourceFile.WithOwner(u.ID.String()), rbac.ActionCreate),
				nil)
		})
	})
}
