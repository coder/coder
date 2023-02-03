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
			return methodCase(values(ws.ID), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaces", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.Workspace(t, db, database.Workspace{})
			_ = dbgen.Workspace(t, db, database.Workspace{})
			// No asserts here because SQLFilter.
			return methodCase(values(database.GetWorkspacesParams{}), asserts())
		})
	})
	s.Run("GetLatestWorkspaceBuildByWorkspaceID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID})
			return methodCase(values(ws.ID), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetLatestWorkspaceBuildsByWorkspaceIDs", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID})
			return methodCase(
				values([]uuid.UUID{ws.ID}),
				asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceAgentByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
			agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
			return methodCase(values(agt.ID), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceAgentByInstanceID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
			agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
			return methodCase(values(agt.AuthInstanceID.String), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceAgentsByResourceIDs", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
			_ = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
			return methodCase(values([]uuid.UUID{res.ID}), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("UpdateWorkspaceAgentLifecycleStateByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
			agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
			return methodCase(values(database.UpdateWorkspaceAgentLifecycleStateByIDParams{
				ID:             agt.ID,
				LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
			}), asserts(ws, rbac.ActionUpdate))
		})
	})
	s.Run("GetWorkspaceAppByAgentIDAndSlug", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
			agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
			app := dbgen.WorkspaceApp(t, db, database.WorkspaceApp{AgentID: agt.ID})

			return methodCase(values(database.GetWorkspaceAppByAgentIDAndSlugParams{
				AgentID: agt.ID,
				Slug:    app.Slug,
			}), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceAppsByAgentID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
			agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
			_ = dbgen.WorkspaceApp(t, db, database.WorkspaceApp{AgentID: agt.ID})
			_ = dbgen.WorkspaceApp(t, db, database.WorkspaceApp{AgentID: agt.ID})

			return methodCase(values(agt.ID), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceAppsByAgentIDs", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			aWs := dbgen.Workspace(t, db, database.Workspace{})
			aBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: aWs.ID, JobID: uuid.New()})
			aRes := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: aBuild.JobID})
			aAgt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: aRes.ID})
			a := dbgen.WorkspaceApp(t, db, database.WorkspaceApp{AgentID: aAgt.ID})

			bWs := dbgen.Workspace(t, db, database.Workspace{})
			bBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: bWs.ID, JobID: uuid.New()})
			bRes := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: bBuild.JobID})
			bAgt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: bRes.ID})
			b := dbgen.WorkspaceApp(t, db, database.WorkspaceApp{AgentID: bAgt.ID})

			return methodCase(values([]uuid.UUID{a.AgentID, b.AgentID}), asserts(aWs, rbac.ActionRead, bWs, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceBuildByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID})
			return methodCase(values(build.ID), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceBuildByJobID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID})
			return methodCase(values(build.JobID), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceBuildByWorkspaceIDAndBuildNumber", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 10})
			return methodCase(values(database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
				WorkspaceID: ws.ID,
				BuildNumber: build.BuildNumber,
			}), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceBuildParameters", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID})
			return methodCase(values(build.ID), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceBuildsByWorkspaceID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 1})
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 2})
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 3})
			return methodCase(values(database.GetWorkspaceBuildsByWorkspaceIDParams{WorkspaceID: ws.ID}), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceByAgentID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
			agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
			return methodCase(values(agt.ID), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceByOwnerIDAndName", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			return methodCase(values(database.GetWorkspaceByOwnerIDAndNameParams{
				OwnerID: ws.OwnerID,
				Deleted: ws.Deleted,
				Name:    ws.Name,
			}), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceResourceByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
			return methodCase(values(res.ID), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("GetWorkspaceResourceMetadataByResourceIDs", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			a := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
			b := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
			return methodCase(values([]uuid.UUID{a.ID, b.ID}), asserts(ws, []rbac.Action{rbac.ActionRead, rbac.ActionRead}))
		})
	})
	s.Run("Build/GetWorkspaceResourcesByJobID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			job := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
			return methodCase(values(job.ID), asserts(ws, rbac.ActionRead))
		})
	})
	s.Run("Template/GetWorkspaceResourcesByJobID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			tpl := dbgen.Template(t, db, database.Template{})
			v := dbgen.TemplateVersion(t, db, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, JobID: uuid.New()})
			job := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{ID: v.JobID, Type: database.ProvisionerJobTypeTemplateVersionImport})
			return methodCase(values(job.ID), asserts(v.RBACObject(tpl), []rbac.Action{rbac.ActionRead, rbac.ActionRead}))
		})
	})
	s.Run("GetWorkspaceResourcesByJobIDs", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			tpl := dbgen.Template(t, db, database.Template{})
			v := dbgen.TemplateVersion(t, db, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, JobID: uuid.New()})
			tJob := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{ID: v.JobID, Type: database.ProvisionerJobTypeTemplateVersionImport})

			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			wJob := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
			return methodCase(values([]uuid.UUID{tJob.ID, wJob.ID}), asserts(v.RBACObject(tpl), rbac.ActionRead, ws, rbac.ActionRead))
		})
	})
	s.Run("InsertWorkspace", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			o := dbgen.Organization(t, db, database.Organization{})
			return methodCase(values(database.InsertWorkspaceParams{
				ID:             uuid.New(),
				OwnerID:        u.ID,
				OrganizationID: o.ID,
			}), asserts(rbac.ResourceWorkspace.WithOwner(u.ID.String()).InOrg(o.ID), rbac.ActionCreate))
		})
	})
	s.Run("Start/InsertWorkspaceBuild", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			w := dbgen.Workspace(t, db, database.Workspace{})
			return methodCase(values(database.InsertWorkspaceBuildParams{
				WorkspaceID: w.ID,
				Transition:  database.WorkspaceTransitionStart,
				Reason:      database.BuildReasonInitiator,
			}), asserts(w, rbac.ActionUpdate))
		})
	})
	s.Run("Delete/InsertWorkspaceBuild", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			w := dbgen.Workspace(t, db, database.Workspace{})
			return methodCase(values(database.InsertWorkspaceBuildParams{
				WorkspaceID: w.ID,
				Transition:  database.WorkspaceTransitionDelete,
				Reason:      database.BuildReasonInitiator,
			}), asserts(w, rbac.ActionDelete))
		})
	})
	s.Run("InsertWorkspaceBuildParameters", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			w := dbgen.Workspace(t, db, database.Workspace{})
			b := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w.ID})
			return methodCase(values(database.InsertWorkspaceBuildParametersParams{
				WorkspaceBuildID: b.ID,
				Name:             []string{"foo", "bar"},
				Value:            []string{"baz", "qux"},
			}), asserts(w, rbac.ActionUpdate))
		})
	})
	s.Run("UpdateWorkspace", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			w := dbgen.Workspace(t, db, database.Workspace{})
			return methodCase(values(database.UpdateWorkspaceParams{
				ID: w.ID,
			}), asserts(w, rbac.ActionUpdate))
		})
	})
	s.Run("UpdateWorkspaceAgentConnectionByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
			agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
			return methodCase(values(database.UpdateWorkspaceAgentConnectionByIDParams{
				ID: agt.ID,
			}), asserts(ws, rbac.ActionUpdate))
		})
	})
	s.Run("InsertAgentStat", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			return methodCase(values(database.InsertAgentStatParams{
				WorkspaceID: ws.ID,
			}), asserts(ws, rbac.ActionUpdate))
		})
	})
	s.Run("UpdateWorkspaceAgentVersionByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
			agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
			return methodCase(values(database.UpdateWorkspaceAgentVersionByIDParams{
				ID:      agt.ID,
				Version: "test",
			}), asserts(ws, rbac.ActionUpdate))
		})
	})
	s.Run("UpdateWorkspaceAppHealthByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
			agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
			app := dbgen.WorkspaceApp(t, db, database.WorkspaceApp{AgentID: agt.ID})
			return methodCase(values(database.UpdateWorkspaceAppHealthByIDParams{
				ID:     app.ID,
				Health: database.WorkspaceAppHealthHealthy,
			}), asserts(ws, rbac.ActionUpdate))
		})
	})
	s.Run("UpdateWorkspaceAutostart", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			return methodCase(values(database.UpdateWorkspaceAutostartParams{
				ID: ws.ID,
			}), asserts(ws, rbac.ActionUpdate))
		})
	})
	s.Run("UpdateWorkspaceBuildByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			return methodCase(values(database.UpdateWorkspaceBuildByIDParams{
				ID: build.ID,
			}), asserts(ws, rbac.ActionUpdate))
		})
	})
	s.Run("SoftDeleteWorkspaceByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			return methodCase(values(ws.ID), asserts(ws, rbac.ActionDelete))
		})
	})
	s.Run("UpdateWorkspaceDeletedByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			return methodCase(values(database.UpdateWorkspaceDeletedByIDParams{
				ID:      ws.ID,
				Deleted: true,
			}), asserts(ws, rbac.ActionDelete))
		})
	})
	s.Run("UpdateWorkspaceLastUsedAt", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			return methodCase(values(database.UpdateWorkspaceLastUsedAtParams{
				ID: ws.ID,
			}), asserts(ws, rbac.ActionUpdate))
		})
	})
	s.Run("UpdateWorkspaceTTL", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			return methodCase(values(database.UpdateWorkspaceTTLParams{
				ID: ws.ID,
			}), asserts(ws, rbac.ActionUpdate))
		})
	})
	s.Run("GetWorkspaceByWorkspaceAppID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			ws := dbgen.Workspace(t, db, database.Workspace{})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
			res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
			agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
			app := dbgen.WorkspaceApp(t, db, database.WorkspaceApp{AgentID: agt.ID})
			return methodCase(values(app.ID), asserts(ws, rbac.ActionRead))
		})
	})
}
