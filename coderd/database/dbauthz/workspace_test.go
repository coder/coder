package dbauthz_test

import (
	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

func (s *MethodTestSuite) TestWorkspace() {
	s.Run("GetWorkspaceByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(ws.ID).Asserts(ws, rbac.ActionRead)
	}))
	s.Run("GetWorkspaces", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.Workspace(s.T(), db, database.Workspace{})
		_ = dbgen.Workspace(s.T(), db, database.Workspace{})
		// No asserts here because SQLFilter.
		check.Args(database.GetWorkspacesParams{}).Asserts()
	}))
	s.Run("GetAuthorizedWorkspaces", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.Workspace(s.T(), db, database.Workspace{})
		_ = dbgen.Workspace(s.T(), db, database.Workspace{})
		// No asserts here because SQLFilter.
		check.Args(database.GetWorkspacesParams{}, emptyPreparedAuthorized{}).Asserts()
	}))
	s.Run("GetLatestWorkspaceBuildByWorkspaceID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args(ws.ID).Asserts(ws, rbac.ActionRead).Returns(b)
	}))
	s.Run("GetLatestWorkspaceBuildsByWorkspaceIDs", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args([]uuid.UUID{ws.ID}).Asserts(ws, rbac.ActionRead).Returns(slice.New(b))
	}))
	s.Run("GetWorkspaceAgentByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(agt.ID).Asserts(ws, rbac.ActionRead).Returns(agt)
	}))
	s.Run("GetWorkspaceAgentByInstanceID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(agt.AuthInstanceID.String).Asserts(ws, rbac.ActionRead).Returns(agt)
	}))
	s.Run("GetWorkspaceAgentsByResourceIDs", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args([]uuid.UUID{res.ID}).Asserts(ws, rbac.ActionRead).
			Returns([]database.WorkspaceAgent{agt})
	}))
	s.Run("UpdateWorkspaceAgentLifecycleStateByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID:             agt.ID,
			LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAgentStartupByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentStartupByIDParams{
			ID: agt.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("GetWorkspaceAppByAgentIDAndSlug", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		app := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})

		check.Args(database.GetWorkspaceAppByAgentIDAndSlugParams{
			AgentID: agt.ID,
			Slug:    app.Slug,
		}).Asserts(ws, rbac.ActionRead).Returns(app)
	}))
	s.Run("GetWorkspaceAppsByAgentID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		a := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})
		b := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})

		check.Args(agt.ID).Asserts(ws, rbac.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("GetWorkspaceAppsByAgentIDs", s.Subtest(func(db database.Store, check *expects) {
		aWs := dbgen.Workspace(s.T(), db, database.Workspace{})
		aBuild := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: aWs.ID, JobID: uuid.New()})
		aRes := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: aBuild.JobID})
		aAgt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: aRes.ID})
		a := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: aAgt.ID})

		bWs := dbgen.Workspace(s.T(), db, database.Workspace{})
		bBuild := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: bWs.ID, JobID: uuid.New()})
		bRes := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: bBuild.JobID})
		bAgt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: bRes.ID})
		b := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: bAgt.ID})

		check.Args([]uuid.UUID{a.AgentID, b.AgentID}).
			Asserts(aWs, rbac.ActionRead, bWs, rbac.ActionRead).
			Returns([]database.WorkspaceApp{a, b})
	}))
	s.Run("GetWorkspaceBuildByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args(build.ID).Asserts(ws, rbac.ActionRead).Returns(build)
	}))
	s.Run("GetWorkspaceBuildByJobID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args(build.JobID).Asserts(ws, rbac.ActionRead).Returns(build)
	}))
	s.Run("GetWorkspaceBuildByWorkspaceIDAndBuildNumber", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 10})
		check.Args(database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
			WorkspaceID: ws.ID,
			BuildNumber: build.BuildNumber,
		}).Asserts(ws, rbac.ActionRead).Returns(build)
	}))
	s.Run("GetWorkspaceBuildParameters", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args(build.ID).Asserts(ws, rbac.ActionRead).
			Returns([]database.WorkspaceBuildParameter{})
	}))
	s.Run("GetWorkspaceBuildsByWorkspaceID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 1})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 2})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 3})
		check.Args(database.GetWorkspaceBuildsByWorkspaceIDParams{WorkspaceID: ws.ID}).Asserts(ws, rbac.ActionRead) // ordering
	}))
	s.Run("GetWorkspaceByAgentID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(agt.ID).Asserts(ws, rbac.ActionRead).Returns(ws)
	}))
	s.Run("GetWorkspaceByOwnerIDAndName", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.GetWorkspaceByOwnerIDAndNameParams{
			OwnerID: ws.OwnerID,
			Deleted: ws.Deleted,
			Name:    ws.Name,
		}).Asserts(ws, rbac.ActionRead).Returns(ws)
	}))
	s.Run("GetWorkspaceResourceByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		_ = dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		check.Args(res.ID).Asserts(ws, rbac.ActionRead).Returns(res)
	}))
	s.Run("GetWorkspaceResourceMetadataByResourceIDs", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		_ = dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		a := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		b := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		check.Args([]uuid.UUID{a.ID, b.ID}).
			Asserts(ws, []rbac.Action{rbac.ActionRead, rbac.ActionRead})
	}))
	s.Run("Build/GetWorkspaceResourcesByJobID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		job := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		check.Args(job.ID).Asserts(ws, rbac.ActionRead).Returns([]database.WorkspaceResource{})
	}))
	s.Run("Template/GetWorkspaceResourcesByJobID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, JobID: uuid.New()})
		job := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{ID: v.JobID, Type: database.ProvisionerJobTypeTemplateVersionImport})
		check.Args(job.ID).Asserts(v.RBACObject(tpl), []rbac.Action{rbac.ActionRead, rbac.ActionRead}).Returns([]database.WorkspaceResource{})
	}))
	s.Run("GetWorkspaceResourcesByJobIDs", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, JobID: uuid.New()})
		tJob := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{ID: v.JobID, Type: database.ProvisionerJobTypeTemplateVersionImport})

		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		wJob := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		check.Args([]uuid.UUID{tJob.ID, wJob.ID}).Asserts(v.RBACObject(tpl), rbac.ActionRead, ws, rbac.ActionRead).Returns([]database.WorkspaceResource{})
	}))
	s.Run("InsertWorkspace", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		o := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(database.InsertWorkspaceParams{
			ID:             uuid.New(),
			OwnerID:        u.ID,
			OrganizationID: o.ID,
		}).Asserts(rbac.ResourceWorkspace.WithOwner(u.ID.String()).InOrg(o.ID), rbac.ActionCreate)
	}))
	s.Run("Start/InsertWorkspaceBuild", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID: w.ID,
			Transition:  database.WorkspaceTransitionStart,
			Reason:      database.BuildReasonInitiator,
		}).Asserts(w, rbac.ActionUpdate)
	}))
	s.Run("Delete/InsertWorkspaceBuild", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID: w.ID,
			Transition:  database.WorkspaceTransitionDelete,
			Reason:      database.BuildReasonInitiator,
		}).Asserts(w, rbac.ActionDelete)
	}))
	s.Run("InsertWorkspaceBuildParameters", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: w.ID})
		check.Args(database.InsertWorkspaceBuildParametersParams{
			WorkspaceBuildID: b.ID,
			Name:             []string{"foo", "bar"},
			Value:            []string{"baz", "qux"},
		}).Asserts(w, rbac.ActionUpdate)
	}))
	s.Run("UpdateWorkspace", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		expected := w
		expected.Name = ""
		check.Args(database.UpdateWorkspaceParams{
			ID: w.ID,
		}).Asserts(w, rbac.ActionUpdate).Returns(expected)
	}))
	s.Run("UpdateWorkspaceAgentConnectionByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentConnectionByIDParams{
			ID: agt.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("InsertAgentStat", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.InsertAgentStatParams{
			WorkspaceID: ws.ID,
		}).Asserts(ws, rbac.ActionUpdate)
	}))
	s.Run("UpdateWorkspaceAppHealthByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		app := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})
		check.Args(database.UpdateWorkspaceAppHealthByIDParams{
			ID:     app.ID,
			Health: database.WorkspaceAppHealthDisabled,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAutostart", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.UpdateWorkspaceAutostartParams{
			ID: ws.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceBuildByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		check.Args(database.UpdateWorkspaceBuildByIDParams{
			ID:        build.ID,
			UpdatedAt: build.UpdatedAt,
			Deadline:  build.Deadline,
		}).Asserts(ws, rbac.ActionUpdate).Returns(build)
	}))
	s.Run("SoftDeleteWorkspaceByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		ws.Deleted = true
		check.Args(ws.ID).Asserts(ws, rbac.ActionDelete).Returns()
	}))
	s.Run("UpdateWorkspaceDeletedByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{Deleted: true})
		check.Args(database.UpdateWorkspaceDeletedByIDParams{
			ID:      ws.ID,
			Deleted: true,
		}).Asserts(ws, rbac.ActionDelete).Returns()
	}))
	s.Run("UpdateWorkspaceLastUsedAt", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.UpdateWorkspaceLastUsedAtParams{
			ID: ws.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceTTL", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.UpdateWorkspaceTTLParams{
			ID: ws.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("GetWorkspaceByWorkspaceAppID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		app := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})
		check.Args(app.ID).Asserts(ws, rbac.ActionRead).Returns(ws)
	}))
}
