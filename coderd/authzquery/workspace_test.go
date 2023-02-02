package authzquery_test

import (
	"testing"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
)

func (suite *MethodTestSuite) TestWorkspace() {
	t := suite.T()
	suite.Run("GetWorkspaceByID", func() {
		suite.RunMethodTest(t, func(t *testing.T, db database.Store) MethodCase {
			workspace := dbgen.Workspace(t, db, database.Workspace{})
			return MethodCase{
				Inputs:     methodInputs(workspace.ID),
				Assertions: asserts(workspace, rbac.ActionRead),
			}
		})
	})
}
