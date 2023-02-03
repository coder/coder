package authzquery_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
)

func (suite *MethodTestSuite) TestSystemFunctions() {
	suite.Run("UpdateUserLinkedID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			l := dbgen.UserLink(t, db, database.UserLink{UserID: u.ID})
			return methodCase(values(database.UpdateUserLinkedIDParams{
				UserID:    u.ID,
				LinkedID:  l.LinkedID,
				LoginType: database.LoginTypeGithub,
			}), asserts(), values(l))
		})
	})
	suite.Run("GetUserLinkByLinkedID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			l := dbgen.UserLink(t, db, database.UserLink{})
			return methodCase(values(l.LinkedID), asserts(), values(l))
		})
	})
	suite.Run("GetUserLinkByUserIDLoginType", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			l := dbgen.UserLink(t, db, database.UserLink{})
			return methodCase(values(database.GetUserLinkByUserIDLoginTypeParams{
				UserID:    l.UserID,
				LoginType: l.LoginType,
			}), asserts(), values(l))
		})
	})
	suite.Run("GetLatestWorkspaceBuilds", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{})
			dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{})
			return methodCase(values(), asserts(), nil)
		})
	})
	suite.Run("GetWorkspaceAgentByAuthToken", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{})
			return methodCase(values(agt.AuthToken), asserts(), values(agt))
		})
	})
	suite.Run("GetActiveUserCount", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(), asserts(), values(int64(0)))
		})
	})
	suite.Run("GetUnexpiredLicenses", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(), asserts(), nil)
		})
	})
	suite.Run("GetAuthorizationUserRoles", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(u.ID), asserts(), nil)
		})
	})
	suite.Run("GetDERPMeshKey", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(), asserts(), nil)
		})
	})
	suite.Run("InsertDERPMeshKey", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values("value"), asserts(), values())
		})
	})
	suite.Run("InsertDeploymentID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values("value"), asserts(), values())
		})
	})
	suite.Run("InsertReplica", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertReplicaParams{
				ID: uuid.New(),
			}), asserts(), nil)
		})
	})
	suite.Run("UpdateReplica", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			replica, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New()})
			require.NoError(t, err)
			return methodCase(values(database.UpdateReplicaParams{
				ID:              replica.ID,
				DatabaseLatency: 100,
			}), asserts(), nil)
		})
	})
	suite.Run("DeleteReplicasUpdatedBefore", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New(), UpdatedAt: time.Now()})
			require.NoError(t, err)
			return methodCase(values(time.Now().Add(time.Hour)), asserts(), nil)
		})
	})
	suite.Run("GetReplicasUpdatedAfter", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New(), UpdatedAt: time.Now()})
			require.NoError(t, err)
			return methodCase(values(time.Now().Add(time.Hour*-1)), asserts(), nil)
		})
	})
	suite.Run("GetUserCount", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(), asserts(), values(0))
		})
	})
	suite.Run("GetTemplates", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.Template(t, db, database.Template{})
			return methodCase(values(), asserts(), nil)
		})
	})
	suite.Run("UpdateWorkspaceBuildCostByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			b := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{})
			o := b
			b.DailyCost = 10
			return methodCase(values(database.UpdateWorkspaceBuildCostByIDParams{
				ID:        b.ID,
				DailyCost: 10,
			}), asserts(), values(o))
		})
	})
	suite.Run("InsertOrUpdateLastUpdateCheck", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values("value"), asserts(), nil)
		})
	})
	suite.Run("GetLastUpdateCheck", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			err := db.InsertOrUpdateLastUpdateCheck(context.Background(), "value")
			require.NoError(t, err)
			return methodCase(values(), asserts(), nil)
		})
	})
	suite.Run("GetWorkspaceBuildsCreatedAfter", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{CreatedAt: time.Now().Add(-time.Hour)})
			return methodCase(values(time.Now()), asserts(), nil)
		})
	})
	suite.Run("GetWorkspaceAgentsCreatedAfter", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{CreatedAt: time.Now().Add(-time.Hour)})
			return methodCase(values(time.Now()), asserts(), nil)
		})
	})
	suite.Run("GetWorkspaceAppsCreatedAfter", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.WorkspaceApp(t, db, database.WorkspaceApp{CreatedAt: time.Now().Add(-time.Hour)})
			return methodCase(values(time.Now()), asserts(), nil)
		})
	})
	suite.Run("GetWorkspaceResourcesCreatedAfter", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{CreatedAt: time.Now().Add(-time.Hour)})
			return methodCase(values(time.Now()), asserts(), nil)
		})
	})
	suite.Run("GetWorkspaceResourceMetadataCreatedAfter", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.WorkspaceResourceMetadata(t, db, database.WorkspaceResourceMetadatum{})
			return methodCase(values(time.Now()), asserts(), nil)
		})
	})
	suite.Run("DeleteOldAgentStats", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(), asserts(), nil)
		})
	})
	suite.Run("GetParameterSchemasCreatedAfter", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.ParameterSchema(t, db, database.ParameterSchema{CreatedAt: time.Now().Add(-time.Hour)})
			return methodCase(values(time.Now()), asserts(), nil)
		})
	})
	suite.Run("GetProvisionerJobsCreatedAfter", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{CreatedAt: time.Now().Add(-time.Hour)})
			return methodCase(values(time.Now()), asserts(), nil)
		})
	})
	suite.Run("InsertWorkspaceAgent", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertWorkspaceAgentParams{
				ID: uuid.New(),
			}), asserts(), nil)
		})
	})
	suite.Run("InsertWorkspaceApp", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertWorkspaceAppParams{
				ID:           uuid.New(),
				Health:       database.WorkspaceAppHealthDisabled,
				SharingLevel: database.AppSharingLevelOwner,
			}), asserts(), nil)
		})
	})
	suite.Run("InsertWorkspaceResourceMetadata", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertWorkspaceResourceMetadataParams{
				WorkspaceResourceID: uuid.New(),
			}), asserts(), nil)
		})
	})
	suite.Run("AcquireProvisionerJob", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			j := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
				StartedAt: sql.NullTime{Valid: false},
			})
			return methodCase(values(database.AcquireProvisionerJobParams{Types: []database.ProvisionerType{j.Provisioner}}),
				asserts(), nil)
		})
	})
	suite.Run("UpdateProvisionerJobWithCompleteByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			j := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{})
			return methodCase(values(database.UpdateProvisionerJobWithCompleteByIDParams{
				ID: j.ID,
			}), asserts(), nil)
		})
	})
	suite.Run("UpdateProvisionerJobByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			j := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{})
			return methodCase(values(database.UpdateProvisionerJobByIDParams{
				ID:        j.ID,
				UpdatedAt: time.Now(),
			}), asserts(), nil)
		})
	})
	suite.Run("InsertProvisionerJob", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertProvisionerJobParams{
				ID:            uuid.New(),
				Provisioner:   database.ProvisionerTypeEcho,
				StorageMethod: database.ProvisionerStorageMethodFile,
				Type:          database.ProvisionerJobTypeWorkspaceBuild,
			}), asserts(), nil)
		})
	})
	suite.Run("InsertProvisionerJobLogs", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			j := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{})
			return methodCase(values(database.InsertProvisionerJobLogsParams{
				JobID: j.ID,
			}), asserts(), nil)
		})
	})
	suite.Run("InsertProvisionerDaemon", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertProvisionerDaemonParams{
				ID: uuid.New(),
			}), asserts(), nil)
		})
	})
	suite.Run("InsertTemplateVersionParameter", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			v := dbgen.TemplateVersion(t, db, database.TemplateVersion{})
			return methodCase(values(database.InsertTemplateVersionParameterParams{
				TemplateVersionID: v.ID,
			}), asserts(), nil)
		})
	})
	suite.Run("InsertWorkspaceResource", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			r := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{})
			return methodCase(values(database.InsertWorkspaceResourceParams{
				ID:         r.ID,
				Transition: database.WorkspaceTransitionStart,
			}), asserts(), nil)
		})
	})
	suite.Run("InsertParameterSchema", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertParameterSchemaParams{
				ID:                       uuid.New(),
				DefaultSourceScheme:      database.ParameterSourceSchemeNone,
				DefaultDestinationScheme: database.ParameterDestinationSchemeNone,
				ValidationTypeSystem:     database.ParameterTypeSystemNone,
			}), asserts(), nil)
		})
	})
}
