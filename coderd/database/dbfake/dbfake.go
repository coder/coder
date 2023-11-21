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
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
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
func WorkspaceWithAgent(
	t testing.TB, db database.Store, seed database.Workspace,
	mutations ...func([]*sdkproto.Agent) []*sdkproto.Agent,
) (
	database.Workspace, string,
) {
	t.Helper()
	authToken := uuid.NewString()
	agents := []*sdkproto.Agent{{
		Id: uuid.NewString(),
		Auth: &sdkproto.Agent_Token{
			Token: authToken,
		},
	}}
	for _, m := range mutations {
		agents = m(agents)
	}
	ws := Workspace(t, db, seed)
	WorkspaceBuild(t, db, ws, database.WorkspaceBuild{}, &sdkproto.Resource{
		Name:   "example",
		Type:   "aws_instance",
		Agents: agents,
	})
	return ws, authToken
}

type BuildBuilder struct {
	t         testing.TB
	db        database.Store
	ps        pubsub.Pubsub
	ws        database.Workspace
	seed      database.WorkspaceBuild
	resources []*sdkproto.Resource
}

func WorkspaceBuildBuilder(t testing.TB, db database.Store, ws database.Workspace) BuildBuilder {
	return BuildBuilder{t: t, db: db, ws: ws}
}

func (b BuildBuilder) Pubsub(ps pubsub.Pubsub) BuildBuilder {
	//nolint: revive // returns modified struct
	b.ps = ps
	return b
}

func (b BuildBuilder) Seed(seed database.WorkspaceBuild) BuildBuilder {
	//nolint: revive // returns modified struct
	b.seed = seed
	return b
}

func (b BuildBuilder) Resource(resource *sdkproto.Resource) BuildBuilder {
	//nolint: revive // returns modified struct
	b.resources = append(b.resources, resource)
	return b
}

func (b BuildBuilder) Do() database.WorkspaceBuild {
	b.t.Helper()
	jobID := uuid.New()
	b.seed.ID = uuid.New()
	b.seed.JobID = jobID
	b.seed.WorkspaceID = b.ws.ID

	// Create a provisioner job for the build!
	payload, err := json.Marshal(provisionerdserver.WorkspaceProvisionJob{
		WorkspaceBuildID: b.seed.ID,
	})
	require.NoError(b.t, err)
	//nolint:gocritic // This is only used by tests.
	ctx := dbauthz.AsSystemRestricted(context.Background())
	job, err := b.db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
		ID:             jobID,
		CreatedAt:      dbtime.Now(),
		UpdatedAt:      dbtime.Now(),
		OrganizationID: b.ws.OrganizationID,
		InitiatorID:    b.ws.OwnerID,
		Provisioner:    database.ProvisionerTypeEcho,
		StorageMethod:  database.ProvisionerStorageMethodFile,
		FileID:         uuid.New(),
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		Input:          payload,
		Tags:           nil,
		TraceMetadata:  pqtype.NullRawMessage{},
	})
	require.NoError(b.t, err, "insert job")
	err = b.db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
		ID:        job.ID,
		UpdatedAt: dbtime.Now(),
		Error:     sql.NullString{},
		ErrorCode: sql.NullString{},
		CompletedAt: sql.NullTime{
			Time:  dbtime.Now(),
			Valid: true,
		},
	})
	require.NoError(b.t, err, "complete job")

	// This intentionally fulfills the minimum requirements of the schema.
	// Tests can provide a custom version ID if necessary.
	if b.seed.TemplateVersionID == uuid.Nil {
		jobID := uuid.New()
		templateVersion := dbgen.TemplateVersion(b.t, b.db, database.TemplateVersion{
			JobID:          jobID,
			OrganizationID: b.ws.OrganizationID,
			CreatedBy:      b.ws.OwnerID,
			TemplateID: uuid.NullUUID{
				UUID:  b.ws.TemplateID,
				Valid: true,
			},
		})
		payload, _ := json.Marshal(provisionerdserver.TemplateVersionImportJob{
			TemplateVersionID: templateVersion.ID,
		})
		dbgen.ProvisionerJob(b.t, b.db, nil, database.ProvisionerJob{
			ID:             jobID,
			OrganizationID: b.ws.OrganizationID,
			Input:          payload,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			CompletedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
		})
		ProvisionerJobResources(b.t, b.db, jobID, b.seed.Transition, b.resources...)
		b.seed.TemplateVersionID = templateVersion.ID
	}
	build := dbgen.WorkspaceBuild(b.t, b.db, b.seed)
	ProvisionerJobResources(b.t, b.db, job.ID, b.seed.Transition, b.resources...)
	if b.ps != nil {
		err = b.ps.Publish(codersdk.WorkspaceNotifyChannel(build.WorkspaceID), []byte{})
		require.NoError(b.t, err)
	}
	return build
}

// WorkspaceBuild inserts a build and a successful job into the database.
func WorkspaceBuild(t testing.TB, db database.Store, ws database.Workspace, seed database.WorkspaceBuild, resources ...*sdkproto.Resource) database.WorkspaceBuild {
	b := WorkspaceBuildBuilder(t, db, ws).Seed(seed)
	for _, r := range resources {
		b = b.Resource(r)
	}
	return b.Do()
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
