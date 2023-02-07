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

func (s *MethodTestSuite) TestSystemFunctions() {
	s.Run("UpdateUserLinkedID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			l := dbgen.UserLink(t, db, database.UserLink{UserID: u.ID})
			return methodCase(values(database.UpdateUserLinkedIDParams{
				UserID:    u.ID,
				LinkedID:  l.LinkedID,
				LoginType: database.LoginTypeGithub,
			}), asserts(), values(l))
		})
	})
	s.Run("GetUserLinkByLinkedID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			l := dbgen.UserLink(t, db, database.UserLink{})
			return methodCase(values(l.LinkedID), asserts(), values(l))
		})
	})
	s.Run("GetUserLinkByUserIDLoginType", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			l := dbgen.UserLink(t, db, database.UserLink{})
			return methodCase(values(database.GetUserLinkByUserIDLoginTypeParams{
				UserID:    l.UserID,
				LoginType: l.LoginType,
			}), asserts(), values(l))
		})
	})
	s.Run("GetLatestWorkspaceBuilds", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{})
			dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{})
			return methodCase(values(), asserts(), nil)
		})
	})
	s.Run("GetWorkspaceAgentByAuthToken", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{})
			return methodCase(values(agt.AuthToken), asserts(), values(agt))
		})
	})
	s.Run("GetActiveUserCount", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(), asserts(), values(int64(0)))
		})
	})
	s.Run("GetUnexpiredLicenses", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(), asserts(), nil)
		})
	})
	s.Run("GetAuthorizationUserRoles", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			u := dbgen.User(t, db, database.User{})
			return methodCase(values(u.ID), asserts(), nil)
		})
	})
	s.Run("GetDERPMeshKey", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(), asserts(), nil)
		})
	})
	s.Run("InsertDERPMeshKey", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values("value"), asserts(), values())
		})
	})
	s.Run("InsertDeploymentID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values("value"), asserts(), values())
		})
	})
	s.Run("InsertReplica", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertReplicaParams{
				ID: uuid.New(),
			}), asserts(), nil)
		})
	})
	s.Run("UpdateReplica", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			replica, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New()})
			require.NoError(t, err)
			return methodCase(values(database.UpdateReplicaParams{
				ID:              replica.ID,
				DatabaseLatency: 100,
			}), asserts(), nil)
		})
	})
	s.Run("DeleteReplicasUpdatedBefore", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New(), UpdatedAt: time.Now()})
			require.NoError(t, err)
			return methodCase(values(time.Now().Add(time.Hour)), asserts(), nil)
		})
	})
	s.Run("GetReplicasUpdatedAfter", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New(), UpdatedAt: time.Now()})
			require.NoError(t, err)
			return methodCase(values(time.Now().Add(time.Hour*-1)), asserts(), nil)
		})
	})
	s.Run("GetUserCount", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(), asserts(), values(int64(0)))
		})
	})
	s.Run("GetTemplates", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.Template(t, db, database.Template{})
			return methodCase(values(), asserts(), nil)
		})
	})
	s.Run("UpdateWorkspaceBuildCostByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			b := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{})
			o := b
			o.DailyCost = 10
			return methodCase(values(database.UpdateWorkspaceBuildCostByIDParams{
				ID:        b.ID,
				DailyCost: 10,
			}), asserts(), values(o))
		})
	})
	s.Run("InsertOrUpdateLastUpdateCheck", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values("value"), asserts(), nil)
		})
	})
	s.Run("GetLastUpdateCheck", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			err := db.InsertOrUpdateLastUpdateCheck(context.Background(), "value")
			require.NoError(t, err)
			return methodCase(values(), asserts(), nil)
		})
	})
	s.Run("GetWorkspaceBuildsCreatedAfter", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{CreatedAt: time.Now().Add(-time.Hour)})
			return methodCase(values(time.Now()), asserts(), nil)
		})
	})
	s.Run("GetWorkspaceAgentsCreatedAfter", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{CreatedAt: time.Now().Add(-time.Hour)})
			return methodCase(values(time.Now()), asserts(), nil)
		})
	})
	s.Run("GetWorkspaceAppsCreatedAfter", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.WorkspaceApp(t, db, database.WorkspaceApp{CreatedAt: time.Now().Add(-time.Hour)})
			return methodCase(values(time.Now()), asserts(), nil)
		})
	})
	s.Run("GetWorkspaceResourcesCreatedAfter", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{CreatedAt: time.Now().Add(-time.Hour)})
			return methodCase(values(time.Now()), asserts(), nil)
		})
	})
	s.Run("GetWorkspaceResourceMetadataCreatedAfter", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.WorkspaceResourceMetadata(t, db, database.WorkspaceResourceMetadatum{})
			return methodCase(values(time.Now()), asserts(), nil)
		})
	})
	s.Run("DeleteOldAgentStats", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(), asserts(), nil)
		})
	})
	s.Run("GetParameterSchemasCreatedAfter", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.ParameterSchema(t, db, database.ParameterSchema{CreatedAt: time.Now().Add(-time.Hour)})
			return methodCase(values(time.Now()), asserts(), nil)
		})
	})
	s.Run("GetProvisionerJobsCreatedAfter", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{CreatedAt: time.Now().Add(-time.Hour)})
			return methodCase(values(time.Now()), asserts(), nil)
		})
	})
	s.Run("InsertWorkspaceAgent", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertWorkspaceAgentParams{
				ID: uuid.New(),
			}), asserts(), nil)
		})
	})
	s.Run("InsertWorkspaceApp", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertWorkspaceAppParams{
				ID:           uuid.New(),
				Health:       database.WorkspaceAppHealthDisabled,
				SharingLevel: database.AppSharingLevelOwner,
			}), asserts(), nil)
		})
	})
	s.Run("InsertWorkspaceResourceMetadata", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertWorkspaceResourceMetadataParams{
				WorkspaceResourceID: uuid.New(),
			}), asserts(), nil)
		})
	})
	s.Run("AcquireProvisionerJob", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			j := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
				StartedAt: sql.NullTime{Valid: false},
			})
			return methodCase(values(database.AcquireProvisionerJobParams{Types: []database.ProvisionerType{j.Provisioner}}),
				asserts(), nil)
		})
	})
	s.Run("UpdateProvisionerJobWithCompleteByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			j := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{})
			return methodCase(values(database.UpdateProvisionerJobWithCompleteByIDParams{
				ID: j.ID,
			}), asserts(), nil)
		})
	})
	s.Run("UpdateProvisionerJobByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			j := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{})
			return methodCase(values(database.UpdateProvisionerJobByIDParams{
				ID:        j.ID,
				UpdatedAt: time.Now(),
			}), asserts(), nil)
		})
	})
	s.Run("InsertProvisionerJob", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertProvisionerJobParams{
				ID:            uuid.New(),
				Provisioner:   database.ProvisionerTypeEcho,
				StorageMethod: database.ProvisionerStorageMethodFile,
				Type:          database.ProvisionerJobTypeWorkspaceBuild,
			}), asserts(), nil)
		})
	})
	s.Run("InsertProvisionerJobLogs", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			j := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{})
			return methodCase(values(database.InsertProvisionerJobLogsParams{
				JobID: j.ID,
			}), asserts(), nil)
		})
	})
	s.Run("InsertProvisionerDaemon", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertProvisionerDaemonParams{
				ID: uuid.New(),
			}), asserts(), nil)
		})
	})
	s.Run("InsertTemplateVersionParameter", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			v := dbgen.TemplateVersion(t, db, database.TemplateVersion{})
			return methodCase(values(database.InsertTemplateVersionParameterParams{
				TemplateVersionID: v.ID,
			}), asserts(), nil)
		})
	})
	s.Run("InsertWorkspaceResource", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			r := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{})
			return methodCase(values(database.InsertWorkspaceResourceParams{
				ID:         r.ID,
				Transition: database.WorkspaceTransitionStart,
			}), asserts(), nil)
		})
	})
	s.Run("InsertParameterSchema", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertParameterSchemaParams{
				ID:                       uuid.New(),
				DefaultSourceScheme:      database.ParameterSourceSchemeNone,
				DefaultDestinationScheme: database.ParameterDestinationSchemeNone,
				ValidationTypeSystem:     database.ParameterTypeSystemNone,
			}), asserts(), nil)
		})
	})
}
