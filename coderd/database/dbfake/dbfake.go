package dbfake

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

var ownerCtx = dbauthz.As(context.Background(), rbac.Subject{
	ID:     "owner",
	Roles:  rbac.Roles(must(rbac.RoleNames{rbac.RoleOwner()}.Expand())),
	Groups: []string{},
	Scope:  rbac.ExpandableScope(rbac.ScopeAll),
})

type WorkspaceResponse struct {
	Workspace  database.Workspace
	Build      database.WorkspaceBuild
	AgentToken string
	TemplateVersionResponse
}

// WorkspaceBuildBuilder generates workspace builds and associated
// resources.
type WorkspaceBuildBuilder struct {
	t          testing.TB
	db         database.Store
	ps         pubsub.Pubsub
	ws         database.Workspace
	seed       database.WorkspaceBuild
	resources  []*sdkproto.Resource
	params     []database.WorkspaceBuildParameter
	agentToken string
	dispo      workspaceBuildDisposition
}

type workspaceBuildDisposition struct {
	starting bool
}

// WorkspaceBuild generates a workspace build for the provided workspace.
// Pass a database.Workspace{} with a nil ID to also generate a new workspace.
// Omitting the template ID on a workspace will also generate a new template
// with a template version.
func WorkspaceBuild(t testing.TB, db database.Store, ws database.Workspace) WorkspaceBuildBuilder {
	return WorkspaceBuildBuilder{t: t, db: db, ws: ws}
}

func (b WorkspaceBuildBuilder) Pubsub(ps pubsub.Pubsub) WorkspaceBuildBuilder {
	// nolint: revive // returns modified struct
	b.ps = ps
	return b
}

func (b WorkspaceBuildBuilder) Seed(seed database.WorkspaceBuild) WorkspaceBuildBuilder {
	//nolint: revive // returns modified struct
	b.seed = seed
	return b
}

func (b WorkspaceBuildBuilder) Resource(resource ...*sdkproto.Resource) WorkspaceBuildBuilder {
	//nolint: revive // returns modified struct
	b.resources = append(b.resources, resource...)
	return b
}

func (b WorkspaceBuildBuilder) Params(params ...database.WorkspaceBuildParameter) WorkspaceBuildBuilder {
	b.params = params
	return b
}

func (b WorkspaceBuildBuilder) WithAgent(mutations ...func([]*sdkproto.Agent) []*sdkproto.Agent) WorkspaceBuildBuilder {
	//nolint: revive // returns modified struct
	b.agentToken = uuid.NewString()
	agents := []*sdkproto.Agent{{
		Id: uuid.NewString(),
		Auth: &sdkproto.Agent_Token{
			Token: b.agentToken,
		},
	}}
	for _, m := range mutations {
		agents = m(agents)
	}
	b.resources = append(b.resources, &sdkproto.Resource{
		Name:   "example",
		Type:   "aws_instance",
		Agents: agents,
	})
	return b
}

func (b WorkspaceBuildBuilder) Starting() WorkspaceBuildBuilder {
	//nolint: revive // returns modified struct
	b.dispo.starting = true
	return b
}

// Do generates all the resources associated with a workspace build.
// Template and TemplateVersion will be optionally populated if no
// TemplateID is set on the provided workspace.
// Workspace will be optionally populated if no ID is set on the provided
// workspace.
func (b WorkspaceBuildBuilder) Do() WorkspaceResponse {
	b.t.Helper()
	jobID := uuid.New()
	b.seed.ID = uuid.New()
	b.seed.JobID = jobID

	resp := WorkspaceResponse{
		AgentToken: b.agentToken,
	}
	if b.ws.TemplateID == uuid.Nil {
		resp.TemplateVersionResponse = TemplateVersion(b.t, b.db).
			Resources(b.resources...).
			Pubsub(b.ps).
			Seed(database.TemplateVersion{
				OrganizationID: b.ws.OrganizationID,
				CreatedBy:      b.ws.OwnerID,
			}).
			Do()
		b.ws.TemplateID = resp.Template.ID
		b.seed.TemplateVersionID = resp.TemplateVersion.ID
	}

	// If no template version is set assume the active version.
	if b.seed.TemplateVersionID == uuid.Nil {
		template, err := b.db.GetTemplateByID(ownerCtx, b.ws.TemplateID)
		require.NoError(b.t, err)
		require.NotNil(b.t, template.ActiveVersionID, "active version ID unexpectedly nil")
		b.seed.TemplateVersionID = template.ActiveVersionID
	}

	// No ID on the workspace implies we should generate an entry.
	if b.ws.ID == uuid.Nil {
		// nolint: revive
		b.ws = dbgen.Workspace(b.t, b.db, b.ws)
		resp.Workspace = b.ws
	}
	b.seed.WorkspaceID = b.ws.ID
	b.seed.InitiatorID = takeFirst(b.seed.InitiatorID, b.ws.OwnerID)

	// Create a provisioner job for the build!
	payload, err := json.Marshal(provisionerdserver.WorkspaceProvisionJob{
		WorkspaceBuildID: b.seed.ID,
	})
	require.NoError(b.t, err)

	job, err := b.db.InsertProvisionerJob(ownerCtx, database.InsertProvisionerJobParams{
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
		Tags:           map[string]string{},
		TraceMetadata:  pqtype.NullRawMessage{},
	})
	require.NoError(b.t, err, "insert job")

	if b.dispo.starting {
		// might need to do this multiple times if we got a template version
		// import job as well
		for {
			j, err := b.db.AcquireProvisionerJob(ownerCtx, database.AcquireProvisionerJobParams{
				StartedAt: sql.NullTime{
					Time:  dbtime.Now(),
					Valid: true,
				},
				WorkerID: uuid.NullUUID{
					UUID:  uuid.New(),
					Valid: true,
				},
				Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
				Tags:  []byte(`{"scope": "organization"}`),
			})
			require.NoError(b.t, err, "acquire starting job")
			if j.ID == job.ID {
				break
			}
		}
	} else {
		err = b.db.UpdateProvisionerJobWithCompleteByID(ownerCtx, database.UpdateProvisionerJobWithCompleteByIDParams{
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
		ProvisionerJobResources(b.t, b.db, job.ID, b.seed.Transition, b.resources...).Do()
	}

	resp.Build = dbgen.WorkspaceBuild(b.t, b.db, b.seed)

	for i := range b.params {
		b.params[i].WorkspaceBuildID = resp.Build.ID
	}
	_ = dbgen.WorkspaceBuildParameters(b.t, b.db, b.params)

	if b.ps != nil {
		err = b.ps.Publish(codersdk.WorkspaceNotifyChannel(resp.Build.WorkspaceID), []byte{})
		require.NoError(b.t, err)
	}

	return resp
}

type ProvisionerJobResourcesBuilder struct {
	t          testing.TB
	db         database.Store
	jobID      uuid.UUID
	transition database.WorkspaceTransition
	resources  []*sdkproto.Resource
}

// ProvisionerJobResources inserts a series of resources into a provisioner job.
func ProvisionerJobResources(
	t testing.TB, db database.Store, jobID uuid.UUID, transition database.WorkspaceTransition, resources ...*sdkproto.Resource,
) ProvisionerJobResourcesBuilder {
	return ProvisionerJobResourcesBuilder{
		t:          t,
		db:         db,
		jobID:      jobID,
		transition: transition,
		resources:  resources,
	}
}

func (b ProvisionerJobResourcesBuilder) Do() {
	b.t.Helper()
	transition := b.transition
	if transition == "" {
		// Default to start!
		transition = database.WorkspaceTransitionStart
	}
	for _, resource := range b.resources {
		//nolint:gocritic // This is only used by tests.
		err := provisionerdserver.InsertWorkspaceResource(ownerCtx, b.db, b.jobID, transition, resource, &telemetry.Snapshot{})
		require.NoError(b.t, err)
	}
}

type TemplateVersionResponse struct {
	Template        database.Template
	TemplateVersion database.TemplateVersion
}

type TemplateVersionBuilder struct {
	t         testing.TB
	db        database.Store
	seed      database.TemplateVersion
	ps        pubsub.Pubsub
	resources []*sdkproto.Resource
	params    []database.TemplateVersionParameter
	promote   bool
}

// TemplateVersion generates a template version and optionally a parent
// template if no template ID is set on the seed.
func TemplateVersion(t testing.TB, db database.Store) TemplateVersionBuilder {
	return TemplateVersionBuilder{
		t:       t,
		db:      db,
		promote: true,
	}
}

func (t TemplateVersionBuilder) Seed(v database.TemplateVersion) TemplateVersionBuilder {
	// nolint: revive // returns modified struct
	t.seed = v
	return t
}

func (t TemplateVersionBuilder) Pubsub(ps pubsub.Pubsub) TemplateVersionBuilder {
	// nolint: revive // returns modified struct
	t.ps = ps
	return t
}

func (t TemplateVersionBuilder) Resources(rs ...*sdkproto.Resource) TemplateVersionBuilder {
	// nolint: revive // returns modified struct
	t.resources = rs
	return t
}

func (t TemplateVersionBuilder) Params(ps ...database.TemplateVersionParameter) TemplateVersionBuilder {
	// nolint: revive // returns modified struct
	t.params = ps
	return t
}

func (t TemplateVersionBuilder) Do() TemplateVersionResponse {
	t.t.Helper()

	t.seed.OrganizationID = takeFirst(t.seed.OrganizationID, uuid.New())
	t.seed.ID = takeFirst(t.seed.ID, uuid.New())
	t.seed.CreatedBy = takeFirst(t.seed.CreatedBy, uuid.New())

	var resp TemplateVersionResponse
	if t.seed.TemplateID.UUID == uuid.Nil {
		resp.Template = dbgen.Template(t.t, t.db, database.Template{
			ActiveVersionID: t.seed.ID,
			OrganizationID:  t.seed.OrganizationID,
			CreatedBy:       t.seed.CreatedBy,
		})
		t.seed.TemplateID = uuid.NullUUID{
			Valid: true,
			UUID:  resp.Template.ID,
		}
	}

	version := dbgen.TemplateVersion(t.t, t.db, t.seed)

	// Always make this version the active version. We can easily
	// add a conditional to the builder to opt out of this when
	// necessary.
	err := t.db.UpdateTemplateActiveVersionByID(ownerCtx, database.UpdateTemplateActiveVersionByIDParams{
		ID:              t.seed.TemplateID.UUID,
		ActiveVersionID: t.seed.ID,
		UpdatedAt:       dbtime.Now(),
	})
	require.NoError(t.t, err)

	payload, err := json.Marshal(provisionerdserver.TemplateVersionImportJob{
		TemplateVersionID: t.seed.ID,
	})
	require.NoError(t.t, err)

	job := dbgen.ProvisionerJob(t.t, t.db, t.ps, database.ProvisionerJob{
		ID:             version.JobID,
		OrganizationID: t.seed.OrganizationID,
		InitiatorID:    t.seed.CreatedBy,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
		Input:          payload,
		CompletedAt: sql.NullTime{
			Time:  dbtime.Now(),
			Valid: true,
		},
	})

	t.seed.JobID = job.ID

	ProvisionerJobResources(t.t, t.db, job.ID, "", t.resources...).Do()

	for i, param := range t.params {
		param.TemplateVersionID = version.ID
		t.params[i] = dbgen.TemplateVersionParameter(t.t, t.db, param)
	}

	resp.TemplateVersion = version
	return resp
}

type JobCompleteBuilder struct {
	t     testing.TB
	db    database.Store
	jobID uuid.UUID
	ps    pubsub.Pubsub
}

type JobCompleteResponse struct {
	CompletedAt time.Time
}

func JobComplete(t testing.TB, db database.Store, jobID uuid.UUID) JobCompleteBuilder {
	return JobCompleteBuilder{
		t:     t,
		db:    db,
		jobID: jobID,
	}
}

func (b JobCompleteBuilder) Pubsub(ps pubsub.Pubsub) JobCompleteBuilder {
	// nolint: revive // returns modified struct
	b.ps = ps
	return b
}

func (b JobCompleteBuilder) Do() JobCompleteResponse {
	r := JobCompleteResponse{CompletedAt: dbtime.Now()}
	err := b.db.UpdateProvisionerJobWithCompleteByID(ownerCtx, database.UpdateProvisionerJobWithCompleteByIDParams{
		ID:        b.jobID,
		UpdatedAt: r.CompletedAt,
		Error:     sql.NullString{},
		ErrorCode: sql.NullString{},
		CompletedAt: sql.NullTime{
			Time:  r.CompletedAt,
			Valid: true,
		},
	})
	require.NoError(b.t, err, "complete job")
	if b.ps != nil {
		data, err := json.Marshal(provisionersdk.ProvisionerJobLogsNotifyMessage{EndOfLogs: true})
		require.NoError(b.t, err)
		err = b.ps.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(b.jobID), data)
		require.NoError(b.t, err)
	}
	return r
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

// takeFirst will take the first non-empty value.
func takeFirst[Value comparable](values ...Value) Value {
	var empty Value
	return takeFirstF(values, func(v Value) bool {
		return v != empty
	})
}
