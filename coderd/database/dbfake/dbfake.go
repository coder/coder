package dbfake

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

var ownerCtx = dbauthz.As(context.Background(), rbac.Subject{
	ID:     "owner",
	Roles:  rbac.Roles(must(rbac.RoleIdentifiers{rbac.RoleOwner()}.Expand())),
	Groups: []string{},
	Scope:  rbac.ExpandableScope(rbac.ScopeAll),
})

type WorkspaceResponse struct {
	Workspace  database.WorkspaceTable
	Build      database.WorkspaceBuild
	AgentToken string
	TemplateVersionResponse
	Task database.Task
}

// WorkspaceBuildBuilder generates workspace builds and associated
// resources.
type WorkspaceBuildBuilder struct {
	t          testing.TB
	logger     slog.Logger
	db         database.Store
	ps         pubsub.Pubsub
	ws         database.WorkspaceTable
	seed       database.WorkspaceBuild
	resources  []*sdkproto.Resource
	params     []database.WorkspaceBuildParameter
	agentToken string
	jobStatus  database.ProvisionerJobStatus
	taskAppID  uuid.UUID
	taskSeed   database.TaskTable
}

// WorkspaceBuild generates a workspace build for the provided workspace.
// Pass a database.Workspace{} with a nil ID to also generate a new workspace.
// Omitting the template ID on a workspace will also generate a new template
// with a template version.
func WorkspaceBuild(t testing.TB, db database.Store, ws database.WorkspaceTable) WorkspaceBuildBuilder {
	return WorkspaceBuildBuilder{
		t: t, db: db, ws: ws,
		logger: slogtest.Make(t, &slogtest.Options{}).Named("dbfake").Leveled(slog.LevelDebug),
	}
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
		Id:   uuid.NewString(),
		Name: "dev",
		Auth: &sdkproto.Agent_Token{
			Token: b.agentToken,
		},
		Env: map[string]string{},
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

func (b WorkspaceBuildBuilder) WithTask(taskSeed database.TaskTable, appSeed *sdkproto.App) WorkspaceBuildBuilder {
	//nolint:revive // returns modified struct
	b.taskSeed = taskSeed

	if appSeed == nil {
		appSeed = &sdkproto.App{}
	}

	var err error
	//nolint: revive // returns modified struct
	b.taskAppID, err = uuid.Parse(takeFirst(appSeed.Id, uuid.NewString()))
	require.NoError(b.t, err)

	return b.Params(database.WorkspaceBuildParameter{
		Name:  codersdk.AITaskPromptParameterName,
		Value: b.taskSeed.Prompt,
	}).WithAgent(func(a []*sdkproto.Agent) []*sdkproto.Agent {
		a[0].Apps = []*sdkproto.App{
			{
				Id:   b.taskAppID.String(),
				Slug: takeFirst(appSeed.Slug, "task-app"),
				Url:  takeFirst(appSeed.Url, ""),
			},
		}
		return a
	})
}

func (b WorkspaceBuildBuilder) Starting() WorkspaceBuildBuilder {
	b.jobStatus = database.ProvisionerJobStatusRunning
	return b
}

func (b WorkspaceBuildBuilder) Pending() WorkspaceBuildBuilder {
	b.jobStatus = database.ProvisionerJobStatusPending
	return b
}

func (b WorkspaceBuildBuilder) Canceled() WorkspaceBuildBuilder {
	b.jobStatus = database.ProvisionerJobStatusCanceled
	return b
}

// Do generates all the resources associated with a workspace build.
// Template and TemplateVersion will be optionally populated if no
// TemplateID is set on the provided workspace.
// Workspace will be optionally populated if no ID is set on the provided
// workspace.
func (b WorkspaceBuildBuilder) Do() WorkspaceResponse {
	var resp WorkspaceResponse
	// Use transaction, like real wsbuilder.
	err := b.db.InTx(func(tx database.Store) error {
		//nolint:revive // calls do on modified struct
		b.db = tx
		resp = b.doInTX()
		return nil
	}, nil)
	require.NoError(b.t, err)
	return resp
}

func (b WorkspaceBuildBuilder) doInTX() WorkspaceResponse {
	b.t.Helper()
	jobID := uuid.New()
	b.seed.ID = uuid.New()
	b.seed.JobID = jobID

	if b.taskAppID != uuid.Nil {
		b.seed.HasAITask = sql.NullBool{
			Bool:  true,
			Valid: true,
		}
		b.seed.AITaskSidebarAppID = uuid.NullUUID{UUID: b.taskAppID, Valid: true}
	}

	resp := WorkspaceResponse{
		AgentToken: b.agentToken,
	}
	if b.ws.TemplateID == uuid.Nil {
		b.logger.Debug(context.Background(), "creating template and version")
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
		b.logger.Debug(context.Background(), "assuming active template version")
		template, err := b.db.GetTemplateByID(ownerCtx, b.ws.TemplateID)
		require.NoError(b.t, err)
		require.NotNil(b.t, template.ActiveVersionID, "active version ID unexpectedly nil")
		b.seed.TemplateVersionID = template.ActiveVersionID
	}

	// No ID on the workspace implies we should generate an entry.
	if b.ws.ID == uuid.Nil {
		// nolint: revive
		b.ws = dbgen.Workspace(b.t, b.db, b.ws)
		b.logger.Debug(context.Background(), "created workspace",
			slog.F("name", b.ws.Name),
			slog.F("workspace_id", b.ws.ID))
	}
	resp.Workspace = b.ws
	b.seed.WorkspaceID = b.ws.ID
	b.seed.InitiatorID = takeFirst(b.seed.InitiatorID, b.ws.OwnerID)

	// If a task was requested, ensure it exists and is associated with this
	// workspace.
	if b.taskAppID != uuid.Nil {
		b.logger.Debug(context.Background(), "creating or updating task", "task_id", b.taskSeed.ID)
		b.taskSeed.OrganizationID = takeFirst(b.taskSeed.OrganizationID, b.ws.OrganizationID)
		b.taskSeed.OwnerID = takeFirst(b.taskSeed.OwnerID, b.ws.OwnerID)
		b.taskSeed.Name = takeFirst(b.taskSeed.Name, b.ws.Name)
		b.taskSeed.WorkspaceID = uuid.NullUUID{UUID: takeFirst(b.taskSeed.WorkspaceID.UUID, b.ws.ID), Valid: true}
		b.taskSeed.TemplateVersionID = takeFirst(b.taskSeed.TemplateVersionID, b.seed.TemplateVersionID)

		// Try to fetch existing task and update its workspace ID.
		if task, err := b.db.GetTaskByID(ownerCtx, b.taskSeed.ID); err == nil {
			if !task.WorkspaceID.Valid {
				b.logger.Info(context.Background(), "updating task workspace id", "task_id", b.taskSeed.ID, "workspace_id", b.ws.ID)
				_, err = b.db.UpdateTaskWorkspaceID(ownerCtx, database.UpdateTaskWorkspaceIDParams{
					ID:          b.taskSeed.ID,
					WorkspaceID: uuid.NullUUID{UUID: b.ws.ID, Valid: true},
				})
				require.NoError(b.t, err, "update task workspace id")
			} else if task.WorkspaceID.UUID != b.ws.ID {
				require.Fail(b.t, "task already has a workspace id, mismatch", task.WorkspaceID.UUID, b.ws.ID)
			}
		} else if errors.Is(err, sql.ErrNoRows) {
			task := dbgen.Task(b.t, b.db, b.taskSeed)
			b.taskSeed.ID = task.ID
			b.logger.Info(context.Background(), "created new task", "task_id", b.taskSeed.ID)
		} else {
			require.NoError(b.t, err, "get task by id")
		}
	}

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
		LogsOverflowed: false,
	})
	require.NoError(b.t, err, "insert job")
	b.logger.Debug(context.Background(), "inserted provisioner job", slog.F("job_id", job.ID))

	switch b.jobStatus {
	case database.ProvisionerJobStatusPending:
		// Provisioner jobs are created in 'pending' status
		b.logger.Debug(context.Background(), "pending the provisioner job")
	case database.ProvisionerJobStatusRunning:
		// might need to do this multiple times if we got a template version
		// import job as well
		b.logger.Debug(context.Background(), "looping to acquire provisioner job")
		for {
			j, err := b.db.AcquireProvisionerJob(ownerCtx, database.AcquireProvisionerJobParams{
				OrganizationID: job.OrganizationID,
				StartedAt: sql.NullTime{
					Time:  dbtime.Now(),
					Valid: true,
				},
				WorkerID: uuid.NullUUID{
					UUID:  uuid.New(),
					Valid: true,
				},
				Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
				ProvisionerTags: []byte(`{"scope": "organization"}`),
			})
			require.NoError(b.t, err, "acquire starting job")
			if j.ID == job.ID {
				b.logger.Debug(context.Background(), "acquired provisioner job", slog.F("job_id", job.ID))
				break
			}
		}
	case database.ProvisionerJobStatusCanceled:
		// Set provisioner job status to 'canceled'
		b.logger.Debug(context.Background(), "canceling the provisioner job")
		err = b.db.UpdateProvisionerJobWithCancelByID(ownerCtx, database.UpdateProvisionerJobWithCancelByIDParams{
			ID: jobID,
			CanceledAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
			CompletedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
		})
		require.NoError(b.t, err, "cancel job")
	default:
		// By default, consider jobs in 'succeeded' status
		b.logger.Debug(context.Background(), "completing the provisioner job")
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
	b.logger.Debug(context.Background(), "created workspace build",
		slog.F("build_id", resp.Build.ID),
		slog.F("workspace_id", resp.Workspace.ID),
		slog.F("build_number", resp.Build.BuildNumber))

	// If this is a task workspace, link it to the workspace build.
	task, err := b.db.GetTaskByWorkspaceID(ownerCtx, resp.Workspace.ID)
	if err != nil {
		if b.taskAppID != uuid.Nil {
			require.Fail(b.t, "task app configured but failed to get task by workspace id", err)
		}
	} else {
		if b.taskAppID == uuid.Nil {
			require.Fail(b.t, "task app not configured but workspace is a task workspace")
		}

		app := mustWorkspaceAppByWorkspaceAndBuildAndAppID(ownerCtx, b.t, b.db, resp.Workspace.ID, resp.Build.BuildNumber, b.taskAppID)
		_, err = b.db.UpsertTaskWorkspaceApp(ownerCtx, database.UpsertTaskWorkspaceAppParams{
			TaskID:               task.ID,
			WorkspaceBuildNumber: resp.Build.BuildNumber,
			WorkspaceAgentID:     uuid.NullUUID{UUID: app.AgentID, Valid: true},
			WorkspaceAppID:       uuid.NullUUID{UUID: app.ID, Valid: true},
		})
		require.NoError(b.t, err, "upsert task workspace app")
		b.logger.Debug(context.Background(), "linked task to workspace build",
			slog.F("task_id", task.ID),
			slog.F("build_number", resp.Build.BuildNumber))

		// Update task after linking.
		task, err = b.db.GetTaskByID(ownerCtx, task.ID)
		require.NoError(b.t, err, "get task by id")
		resp.Task = task
	}

	for i := range b.params {
		b.params[i].WorkspaceBuildID = resp.Build.ID
	}
	params := dbgen.WorkspaceBuildParameters(b.t, b.db, b.params)
	b.logger.Debug(context.Background(), "created workspace build parameters", slog.F("count", len(params)))

	if b.ws.Deleted {
		err = b.db.UpdateWorkspaceDeletedByID(ownerCtx, database.UpdateWorkspaceDeletedByIDParams{
			ID:      b.ws.ID,
			Deleted: true,
		})
		require.NoError(b.t, err)
		b.logger.Debug(context.Background(), "deleted workspace", slog.F("workspace_id", resp.Workspace.ID))
	}

	if b.ps != nil {
		msg, err := json.Marshal(wspubsub.WorkspaceEvent{
			Kind:        wspubsub.WorkspaceEventKindStateChange,
			WorkspaceID: resp.Workspace.ID,
		})
		require.NoError(b.t, err)
		err = b.ps.Publish(wspubsub.WorkspaceEventChannel(resp.Workspace.OwnerID), msg)
		require.NoError(b.t, err)
		b.logger.Debug(context.Background(), "published workspace event",
			slog.F("owner_id", resp.Workspace.ID),
			slog.F("owner_id", resp.Workspace.OwnerID))
	}

	agents, err := b.db.GetWorkspaceAgentsByWorkspaceAndBuildNumber(ownerCtx, database.GetWorkspaceAgentsByWorkspaceAndBuildNumberParams{
		WorkspaceID: resp.Workspace.ID,
		BuildNumber: resp.Build.BuildNumber,
	})
	if !errors.Is(err, sql.ErrNoRows) {
		require.NoError(b.t, err, "get workspace agents")
		// Insert deleted subagent test antagonists for the workspace build.
		// See also `dbgen.WorkspaceAgent()`.
		for _, agent := range agents {
			subAgent := dbgen.WorkspaceSubAgent(b.t, b.db, agent, database.WorkspaceAgent{
				TroubleshootingURL: "I AM A TEST ANTAGONIST AND I AM HERE TO MESS UP YOUR TESTS. IF YOU SEE ME, SOMETHING IS WRONG AND SUB AGENT DELETION MAY NOT BE HANDLED CORRECTLY IN A QUERY.",
			})
			err = b.db.DeleteWorkspaceSubAgentByID(ownerCtx, subAgent.ID)
			require.NoError(b.t, err, "delete workspace agent subagent antagonist")

			b.logger.Debug(context.Background(), "inserted deleted subagent antagonist",
				slog.F("subagent_name", subAgent.Name),
				slog.F("subagent_id", subAgent.ID),
				slog.F("agent_name", agent.Name),
				slog.F("agent_id", agent.ID),
			)
		}
	}

	return resp
}

type ProvisionerJobResourcesBuilder struct {
	t          testing.TB
	logger     slog.Logger
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
		logger:     slogtest.Make(t, &slogtest.Options{}).Named("dbfake").Leveled(slog.LevelDebug).With(slog.F("job_id", jobID)),
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
		b.logger.Debug(context.Background(), "setting default transition to start")
		transition = database.WorkspaceTransitionStart
	}
	for _, resource := range b.resources {
		//nolint:gocritic // This is only used by tests.
		err := provisionerdserver.InsertWorkspaceResource(ownerCtx, b.db, b.jobID, transition, resource, &telemetry.Snapshot{})
		require.NoError(b.t, err)
		b.logger.Debug(context.Background(), "created workspace resource",
			slog.F("resource_name", resource.Name),
			slog.F("agent_count", len(resource.Agents)),
		)
	}
}

type TemplateVersionResponse struct {
	Template        database.Template
	TemplateVersion database.TemplateVersion
}

type TemplateVersionBuilder struct {
	t                  testing.TB
	logger             slog.Logger
	db                 database.Store
	seed               database.TemplateVersion
	fileID             uuid.UUID
	ps                 pubsub.Pubsub
	resources          []*sdkproto.Resource
	params             []database.TemplateVersionParameter
	presets            []database.TemplateVersionPreset
	presetParams       []database.TemplateVersionPresetParameter
	promote            bool
	autoCreateTemplate bool
}

// TemplateVersion generates a template version and optionally a parent
// template if no template ID is set on the seed.
func TemplateVersion(t testing.TB, db database.Store) TemplateVersionBuilder {
	return TemplateVersionBuilder{
		t:                  t,
		logger:             slogtest.Make(t, &slogtest.Options{}).Named("dbfake").Leveled(slog.LevelDebug),
		db:                 db,
		promote:            true,
		autoCreateTemplate: true,
	}
}

func (t TemplateVersionBuilder) Seed(v database.TemplateVersion) TemplateVersionBuilder {
	// nolint: revive // returns modified struct
	t.seed = v
	return t
}

func (t TemplateVersionBuilder) FileID(fid uuid.UUID) TemplateVersionBuilder {
	// nolint: revive // returns modified struct
	t.fileID = fid
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

func (t TemplateVersionBuilder) Preset(preset database.TemplateVersionPreset, params ...database.TemplateVersionPresetParameter) TemplateVersionBuilder {
	// nolint: revive // returns modified struct
	t.presets = append(t.presets, preset)
	t.presetParams = append(t.presetParams, params...)
	return t
}

func (t TemplateVersionBuilder) SkipCreateTemplate() TemplateVersionBuilder {
	// nolint: revive // returns modified struct
	t.autoCreateTemplate = false
	t.promote = false
	return t
}

func (t TemplateVersionBuilder) Do() TemplateVersionResponse {
	t.t.Helper()

	t.seed.OrganizationID = takeFirst(t.seed.OrganizationID, uuid.New())
	t.seed.ID = takeFirst(t.seed.ID, uuid.New())
	t.seed.CreatedBy = takeFirst(t.seed.CreatedBy, uuid.New())
	// nolint: revive
	t.fileID = takeFirst(t.fileID, uuid.New())

	var resp TemplateVersionResponse
	if t.seed.TemplateID.UUID == uuid.Nil && t.autoCreateTemplate {
		resp.Template = dbgen.Template(t.t, t.db, database.Template{
			ActiveVersionID: t.seed.ID,
			OrganizationID:  t.seed.OrganizationID,
			CreatedBy:       t.seed.CreatedBy,
		})
		t.seed.TemplateID = uuid.NullUUID{
			Valid: true,
			UUID:  resp.Template.ID,
		}
		t.logger.Debug(context.Background(), "created template",
			slog.F("organization_id", resp.Template.OrganizationID),
			slog.F("template_id", resp.Template.CreatedBy),
		)
	}

	version := dbgen.TemplateVersion(t.t, t.db, t.seed)
	t.logger.Debug(context.Background(), "created template version",
		slog.F("template_version_id", version.ID),
	)
	if t.promote {
		err := t.db.UpdateTemplateActiveVersionByID(ownerCtx, database.UpdateTemplateActiveVersionByIDParams{
			ID:              t.seed.TemplateID.UUID,
			ActiveVersionID: t.seed.ID,
			UpdatedAt:       dbtime.Now(),
		})
		require.NoError(t.t, err)
		t.logger.Debug(context.Background(), "promoted template version",
			slog.F("template_version_id", t.seed.ID),
		)
	}

	for _, preset := range t.presets {
		prst := dbgen.Preset(t.t, t.db, database.InsertPresetParams{
			ID:                  preset.ID,
			TemplateVersionID:   version.ID,
			Name:                preset.Name,
			CreatedAt:           version.CreatedAt,
			DesiredInstances:    preset.DesiredInstances,
			InvalidateAfterSecs: preset.InvalidateAfterSecs,
			SchedulingTimezone:  preset.SchedulingTimezone,
			IsDefault:           false,
			Description:         preset.Description,
			Icon:                preset.Icon,
			LastInvalidatedAt:   preset.LastInvalidatedAt,
		})
		t.logger.Debug(context.Background(), "added preset",
			slog.F("preset_id", prst.ID),
			slog.F("preset_name", prst.Name),
		)
	}

	for _, presetParam := range t.presetParams {
		prm := dbgen.PresetParameter(t.t, t.db, database.InsertPresetParametersParams{
			TemplateVersionPresetID: presetParam.TemplateVersionPresetID,
			Names:                   []string{presetParam.Name},
			Values:                  []string{presetParam.Value},
		})
		t.logger.Debug(context.Background(), "added preset parameter", slog.F("param_name", prm[0].Name))
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
		CompletedAt: sql.NullTime{
			Time:  dbtime.Now(),
			Valid: true,
		},
		FileID: t.fileID,
	})
	t.logger.Debug(context.Background(), "added template version import job", slog.F("job_id", job.ID))

	t.seed.JobID = job.ID

	ProvisionerJobResources(t.t, t.db, job.ID, "", t.resources...).Do()

	for i, param := range t.params {
		param.TemplateVersionID = version.ID
		t.params[i] = dbgen.TemplateVersionParameter(t.t, t.db, param)
	}

	// Update response with template and version
	if resp.Template.ID == uuid.Nil && version.TemplateID.Valid {
		template, err := t.db.GetTemplateByID(ownerCtx, version.TemplateID.UUID)
		require.NoError(t.t, err)
		resp.Template = template
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

// mustWorkspaceAppByWorkspaceAndBuildAndAppID finds a workspace app by
// workspace ID, build number, and app ID. It returns the workspace app
// if found, otherwise fails the test.
func mustWorkspaceAppByWorkspaceAndBuildAndAppID(ctx context.Context, t testing.TB, db database.Store, workspaceID uuid.UUID, buildNumber int32, appID uuid.UUID) database.WorkspaceApp {
	t.Helper()

	agents, err := db.GetWorkspaceAgentsByWorkspaceAndBuildNumber(ctx, database.GetWorkspaceAgentsByWorkspaceAndBuildNumberParams{
		WorkspaceID: workspaceID,
		BuildNumber: buildNumber,
	})
	require.NoError(t, err, "get workspace agents")
	require.NotEmpty(t, agents, "no agents found for workspace")

	for _, agent := range agents {
		apps, err := db.GetWorkspaceAppsByAgentID(ctx, agent.ID)
		require.NoError(t, err, "get workspace apps")
		for _, app := range apps {
			if app.ID == appID {
				return app
			}
		}
	}

	require.FailNow(t, "could not find workspace app", "workspaceID=%s buildNumber=%d appID=%s", workspaceID, buildNumber, appID)
	return database.WorkspaceApp{} // Unreachable.
}
