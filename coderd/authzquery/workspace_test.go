package authzquery_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
)

func (s *MethodTestSuite) TestWorkspace() {
	s.Run("GetWorkspaceByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			return methodCase(inputs(ws.ID), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaces", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.Workspace(t, db, database.Workspace{})
			_ = dbgen.Workspace(t, db, database.Workspace{})
			// No asserts here because SQLFilter.
			return methodCase(inputs(database.GetWorkspacesParams{}), asserts())
		})
	})
	s.Run("GetLatestWorkspaceBuildByWorkspaceID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID})
			return methodCase(inputs(ws.ID), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetLatestWorkspaceBuildsByWorkspaceIDs", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID})
			return methodCase(
				inputs([]uuid.UUID{ws.ID}),
				asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceAgentByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{})
			return methodCase(inputs(agt.ID), asserts())
		})
	})
	s.Run("GetWorkspaceAgentByInstanceID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{})
			return methodCase(inputs(agt.AuthInstanceID.String), asserts())
		})
	})
}
