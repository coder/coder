package dbfake

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/telemetry"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

// Workspace inserts a workspace into the database.
func Workspace(t testing.TB, db database.Store, seed database.Workspace) database.Workspace {
	t.Helper()

	// This intentionally fulfills the minimum requirements of the schema.
	// Tests can provide a custom template ID if necessary.
	if seed.TemplateID == uuid.Nil {
		template := dbgen.Template(t, db, database.Template{
			OrganizationID: seed.OrganizationID,
			CreatedBy:      seed.OwnerID,
		})
		seed.TemplateID = template.ID
		seed.OwnerID = template.CreatedBy
		seed.OrganizationID = template.OrganizationID
	}
	return dbgen.Workspace(t, db, seed)
}

// WorkspaceWithAgent is a helper that generates a workspace with a single resource
// that has an agent attached to it. The agent token is returned.
func WorkspaceWithAgent(t testing.TB, db database.Store, seed database.Workspace) (database.Workspace, string) {
	t.Helper()
	authToken := uuid.NewString()
	ws := Workspace(t, db, seed)
	WorkspaceBuild(t, db, ws, database.WorkspaceBuild{}, &sdkproto.Resource{
		Name: "example",
		Type: "aws_instance",
		Agents: []*sdkproto.Agent{{
			Id: uuid.NewString(),
			Auth: &sdkproto.Agent_Token{
				Token: authToken,
			},
		}},
	})
	return ws, authToken
}

// WorkspaceBuild inserts a build and a successful job into the database.
func WorkspaceBuild(t testing.TB, db database.Store, ws database.Workspace, seed database.WorkspaceBuild, resources ...*sdkproto.Resource) database.WorkspaceBuild {
	t.Helper()
	jobID := uuid.New()
	seed.ID = uuid.New()
	seed.JobID = jobID
	seed.WorkspaceID = ws.ID

	// Create a provisioner job for the build!
	payload, err := json.Marshal(provisionerdserver.WorkspaceProvisionJob{
		WorkspaceBuildID: seed.ID,
	})
	require.NoError(t, err)
	//nolint:gocritic // This is only used by tests.
	ctx := dbauthz.AsSystemRestricted(context.Background())
	job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
		ID:             jobID,
		CreatedAt:      dbtime.Now(),
		UpdatedAt:      dbtime.Now(),
		OrganizationID: ws.OrganizationID,
		InitiatorID:    ws.OwnerID,
		Provisioner:    database.ProvisionerTypeEcho,
		StorageMethod:  database.ProvisionerStorageMethodFile,
		FileID:         uuid.New(),
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		Input:          payload,
		Tags:           nil,
		TraceMetadata:  pqtype.NullRawMessage{},
	})
	require.NoError(t, err, "insert job")
	err = db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
		ID:        job.ID,
		UpdatedAt: dbtime.Now(),
		Error:     sql.NullString{},
		ErrorCode: sql.NullString{},
		CompletedAt: sql.NullTime{
			Time:  dbtime.Now(),
			Valid: true,
		},
	})
	require.NoError(t, err, "complete job")

	// This intentionally fulfills the minimum requirements of the schema.
	// Tests can provide a custom version ID if necessary.
	if seed.TemplateVersionID == uuid.Nil {
		jobID := uuid.New()
		templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			JobID:          jobID,
			OrganizationID: ws.OrganizationID,
			CreatedBy:      ws.OwnerID,
			TemplateID: uuid.NullUUID{
				UUID:  ws.TemplateID,
				Valid: true,
			},
		})
		payload, _ := json.Marshal(provisionerdserver.TemplateVersionImportJob{
			TemplateVersionID: templateVersion.ID,
		})
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			ID:             jobID,
			OrganizationID: ws.OrganizationID,
			Input:          payload,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			CompletedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
		})
		seed.TemplateVersionID = templateVersion.ID
	}
	build := dbgen.WorkspaceBuild(t, db, seed)
	ProvisionerJobResources(t, db, job.ID, seed.Transition, resources...)
	return build
}

// ProvisionerJobResources inserts a series of resources into a provisioner job.
func ProvisionerJobResources(t testing.TB, db database.Store, job uuid.UUID, transition database.WorkspaceTransition, resources ...*sdkproto.Resource) {
	t.Helper()
	if transition == "" {
		// Default to start!
		transition = database.WorkspaceTransitionStart
	}
	for _, resource := range resources {
		//nolint:gocritic // This is only used by tests.
		err := provisionerdserver.InsertWorkspaceResource(dbauthz.AsSystemRestricted(context.Background()), db, job, transition, resource, &telemetry.Snapshot{})
		require.NoError(t, err)
	}
}
