package dbfake

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/telemetry"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

// CreateWorkspace inserts a workspace into the database.
func CreateWorkspace(t testing.TB, db database.Store, seed database.Workspace) database.Workspace {
	t.Helper()

	// This intentionally fulfills the minimum requirements of the schema.
	// Tests can provide a custom template ID if necessary.
	if seed.TemplateID == uuid.Nil {
		template := dbgen.Template(t, db, database.Template{
			OrganizationID: seed.OrganizationID,
			CreatedBy:      seed.OwnerID,
		})
		seed.TemplateID = template.ID
	}
	return dbgen.Workspace(t, db, seed)
}

// CreateWorkspaceBuild inserts a build and a successful job into the database.
func CreateWorkspaceBuild(t testing.TB, db database.Store, ws database.Workspace, seed database.WorkspaceBuild, resources ...*sdkproto.Resource) database.WorkspaceBuild {
	t.Helper()
	jobID := uuid.New()
	seed.JobID = jobID
	seed.WorkspaceID = ws.ID
	// This intentionally fulfills the minimum requirements of the schema.
	// Tests can provide a custom version ID if necessary.
	if seed.TemplateVersionID == uuid.Nil {
		templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: ws.OrganizationID,
			CreatedBy:      ws.OwnerID,
		})
		seed.TemplateVersionID = templateVersion.ID
	}
	build := dbgen.WorkspaceBuild(t, db, seed)

	payload, err := json.Marshal(provisionerdserver.WorkspaceProvisionJob{
		WorkspaceBuildID: build.ID,
	})
	require.NoError(t, err)
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		ID:             jobID,
		Input:          payload,
		OrganizationID: ws.OrganizationID,
		CompletedAt: sql.NullTime{
			Time:  dbtime.Now(),
			Valid: true,
		},
	})
	CreateProvisionerJobResources(t, db, job.ID, seed.Transition, resources...)
	return build
}

// CreateProvisionerJobResources inserts a series of resources into a provisioner job.
func CreateProvisionerJobResources(t testing.TB, db database.Store, job uuid.UUID, transition database.WorkspaceTransition, resources ...*sdkproto.Resource) {
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
