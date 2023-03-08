package dbauthz_test

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

func (s *MethodTestSuite) TestSystemFunctions() {
	var (
		sys    = rbac.ResourceSystem
		create = rbac.ActionCreate
		read   = rbac.ActionRead
		update = rbac.ActionUpdate
		delete = rbac.ActionDelete
	)
	s.Run("UpdateUserLinkedID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		l := dbgen.UserLink(s.T(), db, database.UserLink{UserID: u.ID})
		check.Args(database.UpdateUserLinkedIDParams{
			UserID:    u.ID,
			LinkedID:  l.LinkedID,
			LoginType: database.LoginTypeGithub,
		}).Asserts(sys, update).Returns(l)
	}))
	s.Run("GetUserLinkByLinkedID", s.Subtest(func(db database.Store, check *expects) {
		l := dbgen.UserLink(s.T(), db, database.UserLink{})
		check.Args(l.LinkedID).Asserts(sys, read).Returns(l)
	}))
	s.Run("GetUserLinkByUserIDLoginType", s.Subtest(func(db database.Store, check *expects) {
		l := dbgen.UserLink(s.T(), db, database.UserLink{})
		check.Args(database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    l.UserID,
			LoginType: l.LoginType,
		}).Asserts(sys, read).Returns(l)
	}))
	s.Run("GetLatestWorkspaceBuilds", s.Subtest(func(db database.Store, check *expects) {
		dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{})
		dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{})
		check.Args().Asserts(sys, read)
	}))
	s.Run("GetWorkspaceAgentByAuthToken", s.Subtest(func(db database.Store, check *expects) {
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{})
		check.Args(agt.AuthToken).Asserts(sys, read).Returns(agt)
	}))
	s.Run("GetActiveUserCount", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(sys, read).Returns(int64(0))
	}))
	s.Run("GetUnexpiredLicenses", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(sys, read)
	}))
	s.Run("GetAuthorizationUserRoles", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(sys, read)
	}))
	s.Run("GetDERPMeshKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(sys, read)
	}))
	s.Run("InsertDERPMeshKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts(sys, create).Returns()
	}))
	s.Run("InsertDeploymentID", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts(sys, create).Returns()
	}))
	s.Run("InsertReplica", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertReplicaParams{
			ID: uuid.New(),
		}).Asserts(sys, create)
	}))
	s.Run("UpdateReplica", s.Subtest(func(db database.Store, check *expects) {
		replica, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New()})
		require.NoError(s.T(), err)
		check.Args(database.UpdateReplicaParams{
			ID:              replica.ID,
			DatabaseLatency: 100,
		}).Asserts(sys, update)
	}))
	s.Run("DeleteReplicasUpdatedBefore", s.Subtest(func(db database.Store, check *expects) {
		_, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New(), UpdatedAt: time.Now()})
		require.NoError(s.T(), err)
		check.Args(time.Now().Add(time.Hour)).Asserts(sys, delete)
	}))
	s.Run("GetReplicasUpdatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New(), UpdatedAt: time.Now()})
		require.NoError(s.T(), err)
		check.Args(time.Now().Add(time.Hour*-1)).Asserts(sys, read)
	}))
	s.Run("GetUserCount", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(sys, read).Returns(int64(0))
	}))
	s.Run("GetTemplates", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.Template(s.T(), db, database.Template{})
		check.Args().Asserts(sys, read)
	}))
	s.Run("UpdateWorkspaceBuildCostByID", s.Subtest(func(db database.Store, check *expects) {
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{})
		o := b
		o.DailyCost = 10
		check.Args(database.UpdateWorkspaceBuildCostByIDParams{
			ID:        b.ID,
			DailyCost: 10,
		}).Asserts(sys, update).Returns(o)
	}))
	s.Run("InsertOrUpdateLastUpdateCheck", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts(sys, update)
	}))
	s.Run("GetLastUpdateCheck", s.Subtest(func(db database.Store, check *expects) {
		err := db.InsertOrUpdateLastUpdateCheck(context.Background(), "value")
		require.NoError(s.T(), err)
		check.Args().Asserts(sys, read)
	}))
	s.Run("GetWorkspaceBuildsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts(sys, read)
	}))
	s.Run("GetWorkspaceAgentsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts(sys, read)
	}))
	s.Run("GetWorkspaceAppsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts(sys, read)
	}))
	s.Run("GetWorkspaceResourcesCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts(sys, read)
	}))
	s.Run("GetWorkspaceResourceMetadataCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceResourceMetadatums(s.T(), db, database.WorkspaceResourceMetadatum{})
		check.Args(time.Now()).Asserts(sys, read)
	}))
	s.Run("DeleteOldWorkspaceAgentStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(sys, delete)
	}))
	s.Run("GetParameterSchemasCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.ParameterSchema(s.T(), db, database.ParameterSchema{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts(sys, read)
	}))
	s.Run("GetProvisionerJobsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts(sys, read)
	}))
	s.Run("GetTemplateVersionsByIDs", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		t2 := dbgen.Template(s.T(), db, database.Template{})
		tv1 := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		tv2 := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t2.ID, Valid: true},
		})
		tv3 := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t2.ID, Valid: true},
		})
		check.Args([]uuid.UUID{tv1.ID, tv2.ID, tv3.ID}).
			Asserts(sys, read).
			Returns(slice.New(tv1, tv2, tv3))
	}))
	s.Run("GetUsersByIDs", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.User(s.T(), db, database.User{CreatedAt: database.Now().Add(-time.Hour)})
		b := dbgen.User(s.T(), db, database.User{CreatedAt: database.Now()})
		check.Args([]uuid.UUID{a.ID, b.ID}).
			Asserts(sys, read).
			Returns(slice.New(a, b))
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
			Asserts(sys, read).
			Returns([]database.WorkspaceApp{a, b})
	}))
	s.Run("GetWorkspaceResourcesByJobIDs", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, JobID: uuid.New()})
		tJob := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{ID: v.JobID, Type: database.ProvisionerJobTypeTemplateVersionImport})

		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		wJob := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		check.Args([]uuid.UUID{tJob.ID, wJob.ID}).
			Asserts(sys, read).
			Returns([]database.WorkspaceResource{})
	}))
	s.Run("GetWorkspaceResourceMetadataByResourceIDs", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		_ = dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		a := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		b := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		check.Args([]uuid.UUID{a.ID, b.ID}).
			Asserts(sys, read)
	}))
	s.Run("GetWorkspaceAgentsByResourceIDs", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args([]uuid.UUID{res.ID}).
			Asserts(sys, read).
			Returns([]database.WorkspaceAgent{agt})
	}))
	s.Run("GetProvisionerJobsByIDs", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		b := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		check.Args([]uuid.UUID{a.ID, b.ID}).
			Asserts(sys, read).
			Returns(slice.New(a, b))
	}))
	s.Run("InsertWorkspaceAgent", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentParams{
			ID: uuid.New(),
		}).Asserts(sys, create)
	}))
	s.Run("InsertWorkspaceApp", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAppParams{
			ID:           uuid.New(),
			Health:       database.WorkspaceAppHealthDisabled,
			SharingLevel: database.AppSharingLevelOwner,
		}).Asserts(sys, create)
	}))
	s.Run("InsertWorkspaceResourceMetadata", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceResourceMetadataParams{
			WorkspaceResourceID: uuid.New(),
		}).Asserts(sys, create)
	}))
	s.Run("AcquireProvisionerJob", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			StartedAt: sql.NullTime{Valid: false},
		})
		check.Args(database.AcquireProvisionerJobParams{Types: []database.ProvisionerType{j.Provisioner}}).
			Asserts(sys, update)
	}))
	s.Run("UpdateProvisionerJobWithCompleteByID", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		check.Args(database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: j.ID,
		}).Asserts(sys, update)
	}))
	s.Run("UpdateProvisionerJobByID", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		check.Args(database.UpdateProvisionerJobByIDParams{
			ID:        j.ID,
			UpdatedAt: time.Now(),
		}).Asserts(sys, update)
	}))
	s.Run("InsertProvisionerJob", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeWorkspaceBuild,
		}).Asserts(sys, create)
	}))
	s.Run("InsertProvisionerJobLogs", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		check.Args(database.InsertProvisionerJobLogsParams{
			JobID: j.ID,
		}).Asserts(sys, create)
	}))
	s.Run("InsertProvisionerDaemon", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertProvisionerDaemonParams{
			ID: uuid.New(),
		}).Asserts(sys, create)
	}))
	s.Run("InsertTemplateVersionParameter", s.Subtest(func(db database.Store, check *expects) {
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{})
		check.Args(database.InsertTemplateVersionParameterParams{
			TemplateVersionID: v.ID,
		}).Asserts(sys, create)
	}))
	s.Run("InsertWorkspaceResource", s.Subtest(func(db database.Store, check *expects) {
		r := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{})
		check.Args(database.InsertWorkspaceResourceParams{
			ID:         r.ID,
			Transition: database.WorkspaceTransitionStart,
		}).Asserts(sys, create)
	}))
	s.Run("InsertParameterSchema", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertParameterSchemaParams{
			ID:                       uuid.New(),
			DefaultSourceScheme:      database.ParameterSourceSchemeNone,
			DefaultDestinationScheme: database.ParameterDestinationSchemeNone,
			ValidationTypeSystem:     database.ParameterTypeSystemNone,
		}).Asserts(sys, create)
	}))
}
