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
	"github.com/coder/coder/v2/provisionersdk/proto"
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

func TemplateWithVersion(t testing.TB, db database.Store, tpl database.Template, tv database.TemplateVersion, job database.ProvisionerJob, resources ...*proto.Resource) (database.Template, database.TemplateVersion) {
	t.Helper()

	template := dbgen.Template(t, db, tpl)

	tv.TemplateID = dbgen.TakeFirst(tv.TemplateID, uuid.NullUUID{UUID: template.ID, Valid: true})
	tv.OrganizationID = dbgen.TakeFirst(tv.OrganizationID, template.OrganizationID)
	tv.CreatedBy = dbgen.TakeFirst(tv.CreatedBy, template.CreatedBy)
	version := TemplateVersion(t, db, tv, job, resources...)

	err := db.UpdateTemplateActiveVersionByID(dbgen.Ctx, database.UpdateTemplateActiveVersionByIDParams{
		ID:              template.ID,
		ActiveVersionID: version.ID,
		UpdatedAt:       dbtime.Now(),
	})
	require.NoError(t, err)

	return template, version
}

func TemplateVersion(t testing.TB, db database.Store, tv database.TemplateVersion, job database.ProvisionerJob, resources ...*proto.Resource) database.TemplateVersion {
	templateVersion := dbgen.TemplateVersion(t, db, tv)
	payload, err := json.Marshal(provisionerdserver.TemplateVersionImportJob{
		TemplateVersionID: templateVersion.ID,
	})
	require.NoError(t, err)

	job.ID = dbgen.TakeFirst(job.ID, templateVersion.JobID)
	job.OrganizationID = dbgen.TakeFirst(job.OrganizationID, templateVersion.OrganizationID)
	job.Input = dbgen.TakeFirstSlice(job.Input, payload)
	job.Type = dbgen.TakeFirst(job.Type, database.ProvisionerJobTypeTemplateVersionImport)
	job.CompletedAt = dbgen.TakeFirst(job.CompletedAt, sql.NullTime{
		Time:  dbtime.Now(),
		Valid: true,
	})

	job = dbgen.ProvisionerJob(t, db, nil, job)
	ProvisionerJobResources(t, db, job.ID, "", resources...)
	return templateVersion
}

func TemplateVersionWithParams(t testing.TB, db database.Store, tv database.TemplateVersion, job database.ProvisionerJob, params []database.TemplateVersionParameter) (database.TemplateVersion, []database.TemplateVersionParameter) {
	t.Helper()

	version := TemplateVersion(t, db, tv, job)
	tvps := make([]database.TemplateVersionParameter, 0, len(params))

	for _, param := range params {
		if param.TemplateVersionID == uuid.Nil {
			param.TemplateVersionID = version.ID
		}
		tvp := dbgen.TemplateVersionParameter(t, db, param)
		tvps = append(tvps, tvp)
	}

	return version, tvps
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

func WorkspaceBuildWithParameters(t testing.TB, db database.Store, ws database.Workspace, build database.WorkspaceBuild, params []database.WorkspaceBuildParameter, resources ...*sdkproto.Resource) (database.WorkspaceBuild, []database.WorkspaceBuildParameter) {
	t.Helper()

	b := WorkspaceBuild(t, db, ws, build, resources...)

	for i, param := range params {
		if param.WorkspaceBuildID == uuid.Nil {
			params[i].WorkspaceBuildID = b.ID
		}
	}
	return b, dbgen.WorkspaceBuildParameters(t, db, params)
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
	job, err := db.InsertProvisionerJob(dbgen.Ctx, database.InsertProvisionerJobParams{
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
	err = db.UpdateProvisionerJobWithCompleteByID(dbgen.Ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
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
		templateVersion := TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: ws.OrganizationID,
			CreatedBy:      ws.OwnerID,
			TemplateID: uuid.NullUUID{
				UUID:  ws.TemplateID,
				Valid: true,
			},
		}, database.ProvisionerJob{})
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

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}

// takeFirstF takes the first value that returns true
func takeFirstF[Value any](values []Value, take func(v Value) bool) Value {
	for _, v := range values {
		if take(v) {
			return v
		}
	}
	// If all empty, return the last element
	if len(values) > 0 {
		return values[len(values)-1]
	}
	var empty Value
	return empty
}
