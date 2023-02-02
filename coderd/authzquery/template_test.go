package authzquery_test

import (
	"testing"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
)

func (suite *MethodTestSuite) TestTemplate() {
	suite.Run("GetTemplateByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			obj := dbgen.Template(t, db, database.Template{})
			return MethodCase{
				Inputs:     methodInputs(obj.ID),
				Assertions: asserts(obj, rbac.ActionRead),
			}
		})
	})
}
