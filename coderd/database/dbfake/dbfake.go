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
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

var sysCtx = dbauthz.As(context.Background(), rbac.Subject{
	ID:     "owner",
	Roles:  rbac.Roles(must(rbac.RoleNames{rbac.RoleOwner()}.Expand())),
	Groups: []string{},
	Scope:  rbac.ExpandableScope(rbac.ScopeAll),
})

type WorkspaceBuilder struct {
	t          testing.TB
	db         database.Store
	seed       database.Workspace
	resources  []*sdkproto.Resource
	agentToken string
}

type WorkspaceResponse struct {
	Workspace       database.Workspace
	TemplateVersion database.TemplateVersion
	Build           database.WorkspaceBuild
	AgentToken      string
}

func NewWorkspaceBuilder(t testing.TB, db database.Store) WorkspaceBuilder {
	return WorkspaceBuilder{t: t, db: db}
}

func (b WorkspaceBuilder) Seed(seed database.Workspace) WorkspaceBuilder {
	//nolint: revive // returns modified struct
	b.seed = seed
	return b
}

func (b WorkspaceBuilder) WithAgent(mutations ...func([]*sdkproto.Agent) []*sdkproto.Agent) WorkspaceBuilder {
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

func (b WorkspaceBuilder) Do() WorkspaceResponse {
	b.t.Helper()

	var r WorkspaceResponse

	// This intentionally fulfills the minimum requirements of the schema.
	// Tests can provide a custom template ID if necessary.
	if b.seed.TemplateID == uuid.Nil {
		version := TemplateVersion(b.t, b.db).Seed(database.TemplateVersion{
			OrganizationID: b.seed.OrganizationID,
			CreatedBy:      b.seed.OwnerID,
		}).Do()

		b.seed.TemplateID = version.TemplateID.UUID
		r.TemplateVersion = version
	}
	r.Workspace = dbgen.Workspace(b.t, b.db, b.seed)
	if b.agentToken != "" {
		r.AgentToken = b.agentToken
		r.Build = NewWorkspaceBuildBuilder(b.t, b.db, r.Workspace).
			Seed(database.WorkspaceBuild{
				TemplateVersionID: r.TemplateVersion.ID,
			}).
			Resource(b.resources...).
			Do()
	}
	return r
}

type WorkspaceBuildBuilder struct {
	t         testing.TB
	db        database.Store
	ps        pubsub.Pubsub
	ws        database.Workspace
	seed      database.WorkspaceBuild
	resources []*sdkproto.Resource
	params    []database.WorkspaceBuildParameter
}

func NewWorkspaceBuildBuilder(t testing.TB, db database.Store, ws database.Workspace) WorkspaceBuildBuilder {
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

func (b WorkspaceBuildBuilder) Do() database.WorkspaceBuild {
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
	job, err := b.db.InsertProvisionerJob(sysCtx, database.InsertProvisionerJobParams{
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
	err = b.db.UpdateProvisionerJobWithCompleteByID(sysCtx, database.UpdateProvisionerJobWithCompleteByIDParams{
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
		version := TemplateVersion(b.t, b.db).
			Resources(b.resources...).
			Pubsub(b.ps).
			Do()
		b.seed.TemplateVersionID = version.ID
	}

	build := dbgen.WorkspaceBuild(b.t, b.db, b.seed)
	NewProvisionerJobResourcesBuilder(b.t, b.db, job.ID, b.seed.Transition, b.resources...).Do()
	if b.ps != nil {
		err = b.ps.Publish(codersdk.WorkspaceNotifyChannel(build.WorkspaceID), []byte{})
		require.NoError(b.t, err)
	}

	for i := range b.params {
		b.params[i].WorkspaceBuildID = build.ID
	}
	_ = dbgen.WorkspaceBuildParameters(b.t, b.db, b.params)
	return build
}

type ProvisionerJobResourcesBuilder struct {
	t          testing.TB
	db         database.Store
	jobID      uuid.UUID
	transition database.WorkspaceTransition
	resources  []*sdkproto.Resource
}

// NewProvisionerJobResourcesBuilder inserts a series of resources into a provisioner job.
func NewProvisionerJobResourcesBuilder(
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
		err := provisionerdserver.InsertWorkspaceResource(sysCtx, b.db, b.jobID, transition, resource, &telemetry.Snapshot{})
		require.NoError(b.t, err)
	}
}

type TemplateVersionBuilder struct {
	t         testing.TB
	db        database.Store
	seed      database.TemplateVersion
	ps        pubsub.Pubsub
	resources []*sdkproto.Resource
	params    []database.TemplateVersionParameter
	template  database.Template
	promote   bool
}

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

func (t TemplateVersionBuilder) Do() database.TemplateVersion {
	t.t.Helper()

	t.seed.OrganizationID = dbgen.TakeFirst(t.seed.OrganizationID, uuid.New())
	t.seed.ID = dbgen.TakeFirst(t.seed.ID, uuid.New())
	t.seed.CreatedBy = dbgen.TakeFirst(t.seed.CreatedBy, uuid.New())

	if t.seed.TemplateID.UUID == uuid.Nil {
		template := dbgen.Template(t.t, t.db, database.Template{
			ActiveVersionID: t.seed.ID,
			OrganizationID:  t.seed.OrganizationID,
			CreatedBy:       t.seed.CreatedBy,
		})
		t.seed.TemplateID = uuid.NullUUID{
			Valid: true,
			UUID:  template.ID,
		}
	}

	version := dbgen.TemplateVersion(t.t, t.db, t.seed)

	if t.template.ID != uuid.Nil {
		// Always make this version the active version. We can easily
		// add a conditional to the builder to opt out of this when
		// necessary.
		err := t.db.UpdateTemplateActiveVersionByID(sysCtx, database.UpdateTemplateActiveVersionByIDParams{
			ID:              t.template.ID,
			ActiveVersionID: t.seed.ID,
			UpdatedAt:       dbtime.Now(),
		})
		require.NoError(t.t, err)
	}

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
	})

	t.seed.JobID = job.ID

	NewProvisionerJobResourcesBuilder(t.t, t.db, job.ID, "", t.resources...).Do()

	for i, param := range t.params {
		param.TemplateVersionID = version.ID
		t.params[i] = dbgen.TemplateVersionParameter(t.t, t.db, param)
	}

	return version
}

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}
