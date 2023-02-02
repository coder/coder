package authzquery_test

import (
	"testing"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
)

func (s *MethodTestSuite) TestWorkspace() {
	s.Run("GetWorkspaceByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			workspace := dbgen.Workspace(t, db, database.Workspace{})
			return methodCase(inputs(workspace.ID), asserts(workspace, rbac.ActionRead))
		})
	})
}
