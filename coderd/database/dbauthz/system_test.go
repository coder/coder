package dbauthz_test

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
)

func (s *MethodTestSuite) TestSystemFunctions() {
	s.Run("UpdateUserLinkedID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		l := dbgen.UserLink(s.T(), db, database.UserLink{UserID: u.ID})
		check.Args(database.UpdateUserLinkedIDParams{
			UserID:    u.ID,
			LinkedID:  l.LinkedID,
			LoginType: database.LoginTypeGithub,
		}).Asserts().Returns(l)
	}))
	s.Run("GetUserLinkByLinkedID", s.Subtest(func(db database.Store, check *expects) {
		l := dbgen.UserLink(s.T(), db, database.UserLink{})
		check.Args(l.LinkedID).Asserts().Returns(l)
	}))
	s.Run("GetUserLinkByUserIDLoginType", s.Subtest(func(db database.Store, check *expects) {
		l := dbgen.UserLink(s.T(), db, database.UserLink{})
		check.Args(database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    l.UserID,
			LoginType: l.LoginType,
		}).Asserts().Returns(l)
	}))
	s.Run("GetLatestWorkspaceBuilds", s.Subtest(func(db database.Store, check *expects) {
		dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{})
		dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{})
		check.Args().Asserts()
	}))
	s.Run("GetWorkspaceAgentByAuthToken", s.Subtest(func(db database.Store, check *expects) {
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{})
		check.Args(agt.AuthToken).Asserts().Returns(agt)
	}))
	s.Run("GetActiveUserCount", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts().Returns(int64(0))
	}))
	s.Run("GetUnexpiredLicenses", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts()
	}))
	s.Run("GetAuthorizationUserRoles", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts()
	}))
	s.Run("GetDERPMeshKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts()
	}))
	s.Run("InsertDERPMeshKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts().Returns()
	}))
	s.Run("InsertDeploymentID", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts().Returns()
	}))
	s.Run("InsertReplica", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertReplicaParams{
			ID: uuid.New(),
		}).Asserts()
	}))
	s.Run("UpdateReplica", s.Subtest(func(db database.Store, check *expects) {
		replica, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New()})
		require.NoError(s.T(), err)
		check.Args(database.UpdateReplicaParams{
			ID:              replica.ID,
			DatabaseLatency: 100,
		}).Asserts()
	}))
	s.Run("DeleteReplicasUpdatedBefore", s.Subtest(func(db database.Store, check *expects) {
		_, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New(), UpdatedAt: time.Now()})
		require.NoError(s.T(), err)
		check.Args(time.Now().Add(time.Hour)).Asserts()
	}))
	s.Run("GetReplicasUpdatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New(), UpdatedAt: time.Now()})
		require.NoError(s.T(), err)
		check.Args(time.Now().Add(time.Hour * -1)).Asserts()
	}))
	s.Run("GetUserCount", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts().Returns(int64(0))
	}))
	s.Run("GetTemplates", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.Template(s.T(), db, database.Template{})
		check.Args().Asserts()
	}))
	s.Run("UpdateWorkspaceBuildCostByID", s.Subtest(func(db database.Store, check *expects) {
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{})
		o := b
		o.DailyCost = 10
		check.Args(database.UpdateWorkspaceBuildCostByIDParams{
			ID:        b.ID,
			DailyCost: 10,
		}).Asserts().Returns(o)
	}))
	s.Run("InsertOrUpdateLastUpdateCheck", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts()
	}))
	s.Run("GetLastUpdateCheck", s.Subtest(func(db database.Store, check *expects) {
		err := db.InsertOrUpdateLastUpdateCheck(context.Background(), "value")
		require.NoError(s.T(), err)
		check.Args().Asserts()
	}))
	s.Run("GetWorkspaceBuildsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts()
	}))
	s.Run("GetWorkspaceAgentsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts()
	}))
	s.Run("GetWorkspaceAppsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts()
	}))
	s.Run("GetWorkspaceResourcesCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts()
	}))
	s.Run("GetWorkspaceResourceMetadataCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceResourceMetadatums(s.T(), db, database.WorkspaceResourceMetadatum{})
		check.Args(time.Now()).Asserts()
	}))
	s.Run("DeleteOldAgentStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts()
	}))
	s.Run("GetParameterSchemasCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.ParameterSchema(s.T(), db, database.ParameterSchema{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts()
	}))
	s.Run("GetProvisionerJobsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts()
	}))
	s.Run("InsertWorkspaceAgent", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentParams{
			ID: uuid.New(),
		}).Asserts()
	}))
	s.Run("InsertWorkspaceApp", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAppParams{
			ID:           uuid.New(),
			Health:       database.WorkspaceAppHealthDisabled,
			SharingLevel: database.AppSharingLevelOwner,
		}).Asserts()
	}))
	s.Run("InsertWorkspaceResourceMetadata", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceResourceMetadataParams{
			WorkspaceResourceID: uuid.New(),
		}).Asserts()
	}))
	s.Run("AcquireProvisionerJob", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			StartedAt: sql.NullTime{Valid: false},
		})
		check.Args(database.AcquireProvisionerJobParams{Types: []database.ProvisionerType{j.Provisioner}}).
			Asserts()
	}))
	s.Run("UpdateProvisionerJobWithCompleteByID", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		check.Args(database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: j.ID,
		}).Asserts()
	}))
	s.Run("UpdateProvisionerJobByID", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		check.Args(database.UpdateProvisionerJobByIDParams{
			ID:        j.ID,
			UpdatedAt: time.Now(),
		}).Asserts()
	}))
	s.Run("InsertProvisionerJob", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeWorkspaceBuild,
		}).Asserts()
	}))
	s.Run("InsertProvisionerJobLogs", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		check.Args(database.InsertProvisionerJobLogsParams{
			JobID: j.ID,
		}).Asserts()
	}))
	s.Run("InsertProvisionerDaemon", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertProvisionerDaemonParams{
			ID: uuid.New(),
		}).Asserts()
	}))
	s.Run("InsertTemplateVersionParameter", s.Subtest(func(db database.Store, check *expects) {
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{})
		check.Args(database.InsertTemplateVersionParameterParams{
			TemplateVersionID: v.ID,
		}).Asserts()
	}))
	s.Run("InsertWorkspaceResource", s.Subtest(func(db database.Store, check *expects) {
		r := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{})
		check.Args(database.InsertWorkspaceResourceParams{
			ID:         r.ID,
			Transition: database.WorkspaceTransitionStart,
		}).Asserts()
	}))
	s.Run("InsertParameterSchema", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertParameterSchemaParams{
			ID:                       uuid.New(),
			DefaultSourceScheme:      database.ParameterSourceSchemeNone,
			DefaultDestinationScheme: database.ParameterDestinationSchemeNone,
			ValidationTypeSystem:     database.ParameterTypeSystemNone,
		}).Asserts()
	}))
}
