package authzquery_test

import (
	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

func (s *MethodTestSuite) TestWorkspace() {
	s.Run("GetWorkspaceByID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		s.Args(ws.ID).Asserts(ws, rbac.ActionRead).Returns(ws)
	})
	s.Run("GetWorkspaces", func() {
		_ = dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		_ = dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		// No asserts here because SQLFilter.
		s.Args(database.GetWorkspacesParams{}).Asserts().Returns(nil)
	})
	s.Run("GetAuthorizedWorkspaces", func() {
		_ = dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		_ = dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		// No asserts here because SQLFilter.
		s.Args(database.GetWorkspacesParams{}, emptyPreparedAuthorized{}).Asserts().Returns(nil)
	})
	s.Run("GetLatestWorkspaceBuildByWorkspaceID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		b := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID})
		s.Args(ws.ID).Asserts(ws, rbac.ActionRead).Returns(b)
	})
	s.Run("GetLatestWorkspaceBuildsByWorkspaceIDs", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		b := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID})
		s.Args([]uuid.UUID{ws.ID}).Asserts(ws, rbac.ActionRead).Returns(slice.New(b))
	})
	s.Run("GetWorkspaceAgentByID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), s.DB, database.WorkspaceAgent{ResourceID: res.ID})
		s.Args(agt.ID).Asserts(ws, rbac.ActionRead).Returns(agt)
	})
	s.Run("GetWorkspaceAgentByInstanceID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), s.DB, database.WorkspaceAgent{ResourceID: res.ID})
		s.Args(agt.AuthInstanceID.String).Asserts(ws, rbac.ActionRead).Returns(agt)
	})
	s.Run("GetWorkspaceAgentsByResourceIDs", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), s.DB, database.WorkspaceAgent{ResourceID: res.ID})
		s.Args([]uuid.UUID{res.ID}).Asserts(ws, rbac.ActionRead).Returns(slice.New(agt))
	})
	s.Run("UpdateWorkspaceAgentLifecycleStateByID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), s.DB, database.WorkspaceAgent{ResourceID: res.ID})
		s.Args(database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID:             agt.ID,
			LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	})
	s.Run("GetWorkspaceAppByAgentIDAndSlug", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), s.DB, database.WorkspaceAgent{ResourceID: res.ID})
		app := dbgen.WorkspaceApp(s.T(), s.DB, database.WorkspaceApp{AgentID: agt.ID})

		s.Args(database.GetWorkspaceAppByAgentIDAndSlugParams{
			AgentID: agt.ID,
			Slug:    app.Slug,
		}).Asserts(ws, rbac.ActionRead).Returns(app)
	})
	s.Run("GetWorkspaceAppsByAgentID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), s.DB, database.WorkspaceAgent{ResourceID: res.ID})
		a := dbgen.WorkspaceApp(s.T(), s.DB, database.WorkspaceApp{AgentID: agt.ID})
		b := dbgen.WorkspaceApp(s.T(), s.DB, database.WorkspaceApp{AgentID: agt.ID})

		s.Args(agt.ID).Asserts(ws, rbac.ActionRead).Returns(slice.New(a, b))
	})
	s.Run("GetWorkspaceAppsByAgentIDs", func() {
		aWs := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		aBuild := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: aWs.ID, JobID: uuid.New()})
		aRes := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: aBuild.JobID})
		aAgt := dbgen.WorkspaceAgent(s.T(), s.DB, database.WorkspaceAgent{ResourceID: aRes.ID})
		a := dbgen.WorkspaceApp(s.T(), s.DB, database.WorkspaceApp{AgentID: aAgt.ID})

		bWs := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		bBuild := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: bWs.ID, JobID: uuid.New()})
		bRes := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: bBuild.JobID})
		bAgt := dbgen.WorkspaceAgent(s.T(), s.DB, database.WorkspaceAgent{ResourceID: bRes.ID})
		b := dbgen.WorkspaceApp(s.T(), s.DB, database.WorkspaceApp{AgentID: bAgt.ID})

		s.Args([]uuid.UUID{aAgt.ID, bAgt.ID}).
			Asserts(aWs, rbac.ActionRead, bWs, rbac.ActionRead).
			Returns(slice.New(a, b))
	})
	s.Run("GetWorkspaceBuildByID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID})
		s.Args(build.ID).Asserts(ws, rbac.ActionRead).Returns(build)
	})
	s.Run("GetWorkspaceBuildByJobID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID})
		s.Args(build.JobID).Asserts(ws, rbac.ActionRead).Returns(build)
	})
	s.Run("GetWorkspaceBuildByWorkspaceIDAndBuildNumber", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 10})
		s.Args(database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
			WorkspaceID: ws.ID,
			BuildNumber: build.BuildNumber,
		}).Asserts(ws, rbac.ActionRead).Returns(build)
	})
	s.Run("GetWorkspaceBuildParameters", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID})
		s.Args(build.ID).Asserts(ws, rbac.ActionRead).Returns([]database.WorkspaceBuildParameter{})
	})
	s.Run("GetWorkspaceBuildsByWorkspaceID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		_ = dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 1})
		_ = dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 2})
		_ = dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 3})
		s.Args(database.GetWorkspaceBuildsByWorkspaceIDParams{WorkspaceID: ws.ID}).Asserts(ws, rbac.ActionRead).Returns(nil) // ordering)
	})
	s.Run("GetWorkspaceByAgentID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), s.DB, database.WorkspaceAgent{ResourceID: res.ID})
		s.Args(agt.ID).Asserts(ws, rbac.ActionRead).Returns(ws)
	})
	s.Run("GetWorkspaceByOwnerIDAndName", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		s.Args(database.GetWorkspaceByOwnerIDAndNameParams{
			OwnerID: ws.OwnerID,
			Deleted: ws.Deleted,
			Name:    ws.Name,
		}).Asserts(ws, rbac.ActionRead).Returns(ws)
	})
	s.Run("GetWorkspaceResourceByID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		_ = dbgen.ProvisionerJob(s.T(), s.DB, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		res := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: build.JobID})
		s.Args(res.ID).Asserts(ws, rbac.ActionRead).Returns(res)
	})
	s.Run("GetWorkspaceResourceMetadataByResourceIDs", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		_ = dbgen.ProvisionerJob(s.T(), s.DB, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		a := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: build.JobID})
		b := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: build.JobID})
		s.Args([]uuid.UUID{a.ID, b.ID}).Asserts(ws, rbac.ActionRead).Returns(nil)
	})
	s.Run("Build/GetWorkspaceResourcesByJobID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		job := dbgen.ProvisionerJob(s.T(), s.DB, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		s.Args(job.ID).Asserts(ws, rbac.ActionRead).Returns([]database.WorkspaceResource{})
	})
	s.Run("Template/GetWorkspaceResourcesByJobID", func() {
		tpl := dbgen.Template(s.T(), s.DB, database.Template{})
		v := dbgen.TemplateVersion(s.T(), s.DB, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, JobID: uuid.New()})
		job := dbgen.ProvisionerJob(s.T(), s.DB, database.ProvisionerJob{ID: v.JobID, Type: database.ProvisionerJobTypeTemplateVersionImport})
		s.Args(job.ID).Asserts(v.RBACObject(tpl), []rbac.Action{rbac.ActionRead, rbac.ActionRead}).Returns([]database.WorkspaceResource{})
	})
	s.Run("GetWorkspaceResourcesByJobIDs", func() {
		tpl := dbgen.Template(s.T(), s.DB, database.Template{})
		v := dbgen.TemplateVersion(s.T(), s.DB, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, JobID: uuid.New()})
		tJob := dbgen.ProvisionerJob(s.T(), s.DB, database.ProvisionerJob{ID: v.JobID, Type: database.ProvisionerJobTypeTemplateVersionImport})

		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		wJob := dbgen.ProvisionerJob(s.T(), s.DB, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		s.Args([]uuid.UUID{tJob.ID, wJob.ID}).Asserts(v.RBACObject(tpl), rbac.ActionRead, ws, rbac.ActionRead).Returns([]database.WorkspaceResource{})
	})
	s.Run("InsertWorkspace", func() {
		u := dbgen.User(s.T(), s.DB, database.User{})
		o := dbgen.Organization(s.T(), s.DB, database.Organization{})
		s.Args(database.InsertWorkspaceParams{
			ID:             uuid.New(),
			OwnerID:        u.ID,
			OrganizationID: o.ID,
		}).
			Asserts(rbac.ResourceWorkspace.WithOwner(u.ID.String()).InOrg(o.ID), rbac.ActionCreate).
			Returns(nil)
	})
	s.Run("Start/InsertWorkspaceBuild", func() {
		w := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		s.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID: w.ID,
			Transition:  database.WorkspaceTransitionStart,
			Reason:      database.BuildReasonInitiator,
		}).Asserts(w, rbac.ActionUpdate).Returns(nil)
	})
	s.Run("Delete/InsertWorkspaceBuild", func() {
		w := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		s.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID: w.ID,
			Transition:  database.WorkspaceTransitionDelete,
			Reason:      database.BuildReasonInitiator,
		}).Asserts(w, rbac.ActionDelete).Returns(nil)
	})
	s.Run("InsertWorkspaceBuildParameters", func() {
		w := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		b := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: w.ID})
		s.Args(database.InsertWorkspaceBuildParametersParams{
			WorkspaceBuildID: b.ID,
			Name:             []string{"foo", "bar"},
			Value:            []string{"baz", "qux"},
		}).Asserts(w, rbac.ActionUpdate).Returns(nil)
	})
	s.Run("UpdateWorkspace", func() {
		w := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		expected := w
		expected.Name = ""
		s.Args(database.UpdateWorkspaceParams{
			ID: w.ID,
		}).Asserts(w, rbac.ActionUpdate).Returns(expected)
	})
	s.Run("UpdateWorkspaceAgentConnectionByID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), s.DB, database.WorkspaceAgent{ResourceID: res.ID})
		s.Args(database.UpdateWorkspaceAgentConnectionByIDParams{
			ID: agt.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	})
	s.Run("InsertAgentStat", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		s.Args(database.InsertAgentStatParams{
			WorkspaceID: ws.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns(nil)
	})
	s.Run("UpdateWorkspaceAgentVersionByID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), s.DB, database.WorkspaceAgent{ResourceID: res.ID})
		s.Args(database.UpdateWorkspaceAgentVersionByIDParams{
			ID: agt.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	})
	s.Run("UpdateWorkspaceAppHealthByID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), s.DB, database.WorkspaceAgent{ResourceID: res.ID})
		app := dbgen.WorkspaceApp(s.T(), s.DB, database.WorkspaceApp{AgentID: agt.ID})
		s.Args(database.UpdateWorkspaceAppHealthByIDParams{
			ID:     app.ID,
			Health: database.WorkspaceAppHealthDisabled,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	})
	s.Run("UpdateWorkspaceAutostart", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		s.Args(database.UpdateWorkspaceAutostartParams{
			ID: ws.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	})
	s.Run("UpdateWorkspaceBuildByID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		s.Args(database.UpdateWorkspaceBuildByIDParams{
			ID:        build.ID,
			UpdatedAt: build.UpdatedAt,
			Deadline:  build.Deadline,
		}).Asserts(ws, rbac.ActionUpdate).Returns(build)
	})
	s.Run("SoftDeleteWorkspaceByID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		ws.Deleted = true
		s.Args(ws.ID).Asserts(ws, rbac.ActionDelete).Returns()
	})
	s.Run("UpdateWorkspaceDeletedByID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{Deleted: true})
		s.Args(database.UpdateWorkspaceDeletedByIDParams{
			ID:      ws.ID,
			Deleted: true,
		}).Asserts(ws, rbac.ActionDelete).Returns()
	})
	s.Run("UpdateWorkspaceLastUsedAt", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		s.Args(database.UpdateWorkspaceLastUsedAtParams{
			ID: ws.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	})
	s.Run("UpdateWorkspaceTTL", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		s.Args(database.UpdateWorkspaceTTLParams{
			ID: ws.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	})
	s.Run("GetWorkspaceByWorkspaceAppID", func() {
		ws := dbgen.Workspace(s.T(), s.DB, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), s.DB, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), s.DB, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), s.DB, database.WorkspaceAgent{ResourceID: res.ID})
		app := dbgen.WorkspaceApp(s.T(), s.DB, database.WorkspaceApp{AgentID: agt.ID})
		s.Args(app.ID).Asserts(ws, rbac.ActionRead).Returns(ws)
	})
}
