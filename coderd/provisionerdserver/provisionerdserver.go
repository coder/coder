package provisionerdserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/tabbed/pqtype"
	semconv "go.opentelemetry.io/otel/semconv/v1.14.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"
	protobuf "google.golang.org/protobuf/proto"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/apikey"
	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/database/pubsub"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/schedule"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner"
	"github.com/coder/coder/provisionerd/proto"
	"github.com/coder/coder/provisionersdk"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
)

var (
	lastAcquire      time.Time
	lastAcquireMutex sync.RWMutex
)

type Server struct {
	AccessURL             *url.URL
	ID                    uuid.UUID
	Logger                slog.Logger
	Provisioners          []database.ProvisionerType
	GitAuthConfigs        []*gitauth.Config
	Tags                  json.RawMessage
	Database              database.Store
	Pubsub                pubsub.Pubsub
	Telemetry             telemetry.Reporter
	Tracer                trace.Tracer
	QuotaCommitter        *atomic.Pointer[proto.QuotaCommitter]
	Auditor               *atomic.Pointer[audit.Auditor]
	TemplateScheduleStore *atomic.Pointer[schedule.TemplateScheduleStore]
	DeploymentValues      *codersdk.DeploymentValues

	AcquireJobDebounce time.Duration
	OIDCConfig         httpmw.OAuth2Config
}

// AcquireJob queries the database to lock a job.
func (server *Server) AcquireJob(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
	//nolint:gocritic // Provisionerd has specific authz rules.
	ctx = dbauthz.AsProvisionerd(ctx)
	// This prevents loads of provisioner daemons from consistently
	// querying the database when no jobs are available.
	//
	// The debounce only occurs when no job is returned, so if loads of
	// jobs are added at once, they will start after at most this duration.
	lastAcquireMutex.RLock()
	if !lastAcquire.IsZero() && time.Since(lastAcquire) < server.AcquireJobDebounce {
		lastAcquireMutex.RUnlock()
		return &proto.AcquiredJob{}, nil
	}
	lastAcquireMutex.RUnlock()
	// This marks the job as locked in the database.
	job, err := server.Database.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
		StartedAt: sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		},
		WorkerID: uuid.NullUUID{
			UUID:  server.ID,
			Valid: true,
		},
		Types: server.Provisioners,
		Tags:  server.Tags,
	})
	if errors.Is(err, sql.ErrNoRows) {
		// The provisioner daemon assumes no jobs are available if
		// an empty struct is returned.
		lastAcquireMutex.Lock()
		lastAcquire = time.Now()
		lastAcquireMutex.Unlock()
		return &proto.AcquiredJob{}, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("acquire job: %w", err)
	}
	server.Logger.Debug(ctx, "locked job from database", slog.F("job_id", job.ID))

	// Marks the acquired job as failed with the error message provided.
	failJob := func(errorMessage string) error {
		err = server.Database.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: job.ID,
			CompletedAt: sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			},
			Error: sql.NullString{
				String: errorMessage,
				Valid:  true,
			},
			ErrorCode: job.ErrorCode,
		})
		if err != nil {
			return xerrors.Errorf("update provisioner job: %w", err)
		}
		return xerrors.Errorf("request job was invalidated: %s", errorMessage)
	}

	user, err := server.Database.GetUserByID(ctx, job.InitiatorID)
	if err != nil {
		return nil, failJob(fmt.Sprintf("get user: %s", err))
	}

	jobTraceMetadata := map[string]string{}
	if job.TraceMetadata.Valid {
		err := json.Unmarshal(job.TraceMetadata.RawMessage, &jobTraceMetadata)
		if err != nil {
			return nil, failJob(fmt.Sprintf("unmarshal metadata: %s", err))
		}
	}

	protoJob := &proto.AcquiredJob{
		JobId:         job.ID.String(),
		CreatedAt:     job.CreatedAt.UnixMilli(),
		Provisioner:   string(job.Provisioner),
		UserName:      user.Username,
		TraceMetadata: jobTraceMetadata,
	}

	switch job.Type {
	case database.ProvisionerJobTypeWorkspaceBuild:
		var input WorkspaceProvisionJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, failJob(fmt.Sprintf("unmarshal job input %q: %s", job.Input, err))
		}
		workspaceBuild, err := server.Database.GetWorkspaceBuildByID(ctx, input.WorkspaceBuildID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get workspace build: %s", err))
		}
		workspace, err := server.Database.GetWorkspaceByID(ctx, workspaceBuild.WorkspaceID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get workspace: %s", err))
		}
		templateVersion, err := server.Database.GetTemplateVersionByID(ctx, workspaceBuild.TemplateVersionID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get template version: %s", err))
		}
		templateVariables, err := server.Database.GetTemplateVersionVariables(ctx, templateVersion.ID)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return nil, failJob(fmt.Sprintf("get template version variables: %s", err))
		}
		template, err := server.Database.GetTemplateByID(ctx, templateVersion.TemplateID.UUID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get template: %s", err))
		}
		owner, err := server.Database.GetUserByID(ctx, workspace.OwnerID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get owner: %s", err))
		}
		err = server.Pubsub.Publish(codersdk.WorkspaceNotifyChannel(workspace.ID), []byte{})
		if err != nil {
			return nil, failJob(fmt.Sprintf("publish workspace update: %s", err))
		}

		var workspaceOwnerOIDCAccessToken string
		if server.OIDCConfig != nil {
			workspaceOwnerOIDCAccessToken, err = obtainOIDCAccessToken(ctx, server.Database, server.OIDCConfig, owner.ID)
			if err != nil {
				return nil, failJob(fmt.Sprintf("obtain OIDC access token: %s", err))
			}
		}

		var sessionToken string
		switch workspaceBuild.Transition {
		case database.WorkspaceTransitionStart:
			sessionToken, err = server.regenerateSessionToken(ctx, owner, workspace)
			if err != nil {
				return nil, failJob(fmt.Sprintf("regenerate session token: %s", err))
			}
		case database.WorkspaceTransitionStop, database.WorkspaceTransitionDelete:
			err = deleteSessionToken(ctx, server.Database, workspace)
			if err != nil {
				return nil, failJob(fmt.Sprintf("delete session token: %s", err))
			}
		}

		transition, err := convertWorkspaceTransition(workspaceBuild.Transition)
		if err != nil {
			return nil, failJob(fmt.Sprintf("convert workspace transition: %s", err))
		}

		workspaceBuildParameters, err := server.Database.GetWorkspaceBuildParameters(ctx, workspaceBuild.ID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get workspace build parameters: %s", err))
		}

		gitAuthProviders := []*sdkproto.GitAuthProvider{}
		for _, p := range templateVersion.GitAuthProviders {
			link, err := server.Database.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
				ProviderID: p,
				UserID:     owner.ID,
			})
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, failJob(fmt.Sprintf("acquire git auth link: %s", err))
			}
			var config *gitauth.Config
			for _, c := range server.GitAuthConfigs {
				if c.ID != p {
					continue
				}
				config = c
				break
			}
			// We weren't able to find a matching config for the ID!
			if config == nil {
				server.Logger.Warn(ctx, "workspace build job is missing git provider",
					slog.F("git_provider_id", p),
					slog.F("template_version_id", templateVersion.ID),
					slog.F("workspace_id", workspaceBuild.WorkspaceID))
				continue
			}

			link, valid, err := config.RefreshToken(ctx, server.Database, link)
			if err != nil {
				return nil, failJob(fmt.Sprintf("refresh git auth link %q: %s", p, err))
			}
			if !valid {
				continue
			}
			gitAuthProviders = append(gitAuthProviders, &sdkproto.GitAuthProvider{
				Id:          p,
				AccessToken: link.OAuthAccessToken,
			})
		}

		protoJob.Type = &proto.AcquiredJob_WorkspaceBuild_{
			WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
				WorkspaceBuildId:    workspaceBuild.ID.String(),
				WorkspaceName:       workspace.Name,
				State:               workspaceBuild.ProvisionerState,
				RichParameterValues: convertRichParameterValues(workspaceBuildParameters),
				VariableValues:      asVariableValues(templateVariables),
				GitAuthProviders:    gitAuthProviders,
				Metadata: &sdkproto.Provision_Metadata{
					CoderUrl:                      server.AccessURL.String(),
					WorkspaceTransition:           transition,
					WorkspaceName:                 workspace.Name,
					WorkspaceOwner:                owner.Username,
					WorkspaceOwnerEmail:           owner.Email,
					WorkspaceOwnerOidcAccessToken: workspaceOwnerOIDCAccessToken,
					WorkspaceId:                   workspace.ID.String(),
					WorkspaceOwnerId:              owner.ID.String(),
					TemplateName:                  template.Name,
					TemplateVersion:               templateVersion.Name,
					WorkspaceOwnerSessionToken:    sessionToken,
				},
				LogLevel: input.LogLevel,
			},
		}
	case database.ProvisionerJobTypeTemplateVersionDryRun:
		var input TemplateVersionDryRunJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, failJob(fmt.Sprintf("unmarshal job input %q: %s", job.Input, err))
		}

		templateVersion, err := server.Database.GetTemplateVersionByID(ctx, input.TemplateVersionID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get template version: %s", err))
		}
		templateVariables, err := server.Database.GetTemplateVersionVariables(ctx, templateVersion.ID)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return nil, failJob(fmt.Sprintf("get template version variables: %s", err))
		}

		protoJob.Type = &proto.AcquiredJob_TemplateDryRun_{
			TemplateDryRun: &proto.AcquiredJob_TemplateDryRun{
				RichParameterValues: convertRichParameterValues(input.RichParameterValues),
				VariableValues:      asVariableValues(templateVariables),
				Metadata: &sdkproto.Provision_Metadata{
					CoderUrl:      server.AccessURL.String(),
					WorkspaceName: input.WorkspaceName,
				},
			},
		}
	case database.ProvisionerJobTypeTemplateVersionImport:
		var input TemplateVersionImportJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, failJob(fmt.Sprintf("unmarshal job input %q: %s", job.Input, err))
		}

		userVariableValues, err := server.includeLastVariableValues(ctx, input.TemplateVersionID, input.UserVariableValues)
		if err != nil {
			return nil, failJob(err.Error())
		}

		protoJob.Type = &proto.AcquiredJob_TemplateImport_{
			TemplateImport: &proto.AcquiredJob_TemplateImport{
				UserVariableValues: convertVariableValues(userVariableValues),
				Metadata: &sdkproto.Provision_Metadata{
					CoderUrl: server.AccessURL.String(),
				},
			},
		}
	}
	switch job.StorageMethod {
	case database.ProvisionerStorageMethodFile:
		file, err := server.Database.GetFileByID(ctx, job.FileID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get file by hash: %s", err))
		}
		protoJob.TemplateSourceArchive = file.Data
	default:
		return nil, failJob(fmt.Sprintf("unsupported storage method: %s", job.StorageMethod))
	}
	if protobuf.Size(protoJob) > provisionersdk.MaxMessageSize {
		return nil, failJob(fmt.Sprintf("payload was too big: %d > %d", protobuf.Size(protoJob), provisionersdk.MaxMessageSize))
	}

	return protoJob, err
}

func (server *Server) includeLastVariableValues(ctx context.Context, templateVersionID uuid.UUID, userVariableValues []codersdk.VariableValue) ([]codersdk.VariableValue, error) {
	var values []codersdk.VariableValue
	values = append(values, userVariableValues...)

	if templateVersionID == uuid.Nil {
		return values, nil
	}

	templateVersion, err := server.Database.GetTemplateVersionByID(ctx, templateVersionID)
	if err != nil {
		return nil, xerrors.Errorf("get template version: %w", err)
	}

	if templateVersion.TemplateID.UUID == uuid.Nil {
		return values, nil
	}

	template, err := server.Database.GetTemplateByID(ctx, templateVersion.TemplateID.UUID)
	if err != nil {
		return nil, xerrors.Errorf("get template: %w", err)
	}

	if template.ActiveVersionID == uuid.Nil {
		return values, nil
	}

	templateVariables, err := server.Database.GetTemplateVersionVariables(ctx, template.ActiveVersionID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get template version variables: %w", err)
	}

	for _, templateVariable := range templateVariables {
		var alreadyAdded bool
		for _, uvv := range userVariableValues {
			if uvv.Name == templateVariable.Name {
				alreadyAdded = true
				break
			}
		}

		if alreadyAdded {
			continue
		}

		values = append(values, codersdk.VariableValue{
			Name:  templateVariable.Name,
			Value: templateVariable.Value,
		})
	}
	return values, nil
}

func (server *Server) CommitQuota(ctx context.Context, request *proto.CommitQuotaRequest) (*proto.CommitQuotaResponse, error) {
	ctx, span := server.startTrace(ctx, tracing.FuncName())
	defer span.End()

	//nolint:gocritic // Provisionerd has specific authz rules.
	ctx = dbauthz.AsProvisionerd(ctx)
	jobID, err := uuid.Parse(request.JobId)
	if err != nil {
		return nil, xerrors.Errorf("parse job id: %w", err)
	}

	job, err := server.Database.GetProvisionerJobByID(ctx, jobID)
	if err != nil {
		return nil, xerrors.Errorf("get job: %w", err)
	}
	if !job.WorkerID.Valid {
		return nil, xerrors.New("job isn't running yet")
	}

	if job.WorkerID.UUID.String() != server.ID.String() {
		return nil, xerrors.New("you don't own this job")
	}

	q := server.QuotaCommitter.Load()
	if q == nil {
		// We're probably in community edition or a test.
		return &proto.CommitQuotaResponse{
			Budget: -1,
			Ok:     true,
		}, nil
	}
	return (*q).CommitQuota(ctx, request)
}

func (server *Server) UpdateJob(ctx context.Context, request *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
	ctx, span := server.startTrace(ctx, tracing.FuncName())
	defer span.End()

	//nolint:gocritic // Provisionerd has specific authz rules.
	ctx = dbauthz.AsProvisionerd(ctx)
	parsedID, err := uuid.Parse(request.JobId)
	if err != nil {
		return nil, xerrors.Errorf("parse job id: %w", err)
	}
	server.Logger.Debug(ctx, "stage UpdateJob starting", slog.F("job_id", parsedID))
	job, err := server.Database.GetProvisionerJobByID(ctx, parsedID)
	if err != nil {
		return nil, xerrors.Errorf("get job: %w", err)
	}
	if !job.WorkerID.Valid {
		return nil, xerrors.New("job isn't running yet")
	}
	if job.WorkerID.UUID.String() != server.ID.String() {
		return nil, xerrors.New("you don't own this job")
	}
	err = server.Database.UpdateProvisionerJobByID(ctx, database.UpdateProvisionerJobByIDParams{
		ID:        parsedID,
		UpdatedAt: database.Now(),
	})
	if err != nil {
		return nil, xerrors.Errorf("update job: %w", err)
	}

	if len(request.Logs) > 0 {
		insertParams := database.InsertProvisionerJobLogsParams{
			JobID: parsedID,
		}
		for _, log := range request.Logs {
			logLevel, err := convertLogLevel(log.Level)
			if err != nil {
				return nil, xerrors.Errorf("convert log level: %w", err)
			}
			logSource, err := convertLogSource(log.Source)
			if err != nil {
				return nil, xerrors.Errorf("convert log source: %w", err)
			}
			insertParams.CreatedAt = append(insertParams.CreatedAt, time.UnixMilli(log.CreatedAt))
			insertParams.Level = append(insertParams.Level, logLevel)
			insertParams.Stage = append(insertParams.Stage, log.Stage)
			insertParams.Source = append(insertParams.Source, logSource)
			insertParams.Output = append(insertParams.Output, log.Output)
			server.Logger.Debug(ctx, "job log",
				slog.F("job_id", parsedID),
				slog.F("stage", log.Stage),
				slog.F("output", log.Output))
		}

		logs, err := server.Database.InsertProvisionerJobLogs(ctx, insertParams)
		if err != nil {
			server.Logger.Error(ctx, "failed to insert job logs", slog.F("job_id", parsedID), slog.Error(err))
			return nil, xerrors.Errorf("insert job logs: %w", err)
		}
		// Publish by the lowest log ID inserted so the log stream will fetch
		// everything from that point.
		lowestID := logs[0].ID
		server.Logger.Debug(ctx, "inserted job logs", slog.F("job_id", parsedID))
		data, err := json.Marshal(provisionersdk.ProvisionerJobLogsNotifyMessage{
			CreatedAfter: lowestID - 1,
		})
		if err != nil {
			return nil, xerrors.Errorf("marshal: %w", err)
		}
		err = server.Pubsub.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(parsedID), data)
		if err != nil {
			server.Logger.Error(ctx, "failed to publish job logs", slog.F("job_id", parsedID), slog.Error(err))
			return nil, xerrors.Errorf("publish job logs: %w", err)
		}
		server.Logger.Debug(ctx, "published job logs", slog.F("job_id", parsedID))
	}

	if len(request.Readme) > 0 {
		err := server.Database.UpdateTemplateVersionDescriptionByJobID(ctx, database.UpdateTemplateVersionDescriptionByJobIDParams{
			JobID:     job.ID,
			Readme:    string(request.Readme),
			UpdatedAt: database.Now(),
		})
		if err != nil {
			return nil, xerrors.Errorf("update template version description: %w", err)
		}
	}

	if len(request.TemplateVariables) > 0 {
		templateVersion, err := server.Database.GetTemplateVersionByJobID(ctx, job.ID)
		if err != nil {
			server.Logger.Error(ctx, "failed to get the template version", slog.F("job_id", parsedID), slog.Error(err))
			return nil, xerrors.Errorf("get template version by job id: %w", err)
		}

		var variableValues []*sdkproto.VariableValue
		var variablesWithMissingValues []string
		for _, templateVariable := range request.TemplateVariables {
			server.Logger.Debug(ctx, "insert template variable", slog.F("template_version_id", templateVersion.ID), slog.F("template_variable", redactTemplateVariable(templateVariable)))

			value := templateVariable.DefaultValue
			for _, v := range request.UserVariableValues {
				if v.Name == templateVariable.Name {
					value = v.Value
					break
				}
			}

			if templateVariable.Required && value == "" {
				variablesWithMissingValues = append(variablesWithMissingValues, templateVariable.Name)
			}

			variableValues = append(variableValues, &sdkproto.VariableValue{
				Name:      templateVariable.Name,
				Value:     value,
				Sensitive: templateVariable.Sensitive,
			})

			_, err = server.Database.InsertTemplateVersionVariable(ctx, database.InsertTemplateVersionVariableParams{
				TemplateVersionID: templateVersion.ID,
				Name:              templateVariable.Name,
				Description:       templateVariable.Description,
				Type:              templateVariable.Type,
				DefaultValue:      templateVariable.DefaultValue,
				Required:          templateVariable.Required,
				Sensitive:         templateVariable.Sensitive,
				Value:             value,
			})
			if err != nil {
				return nil, xerrors.Errorf("insert parameter schema: %w", err)
			}
		}

		if len(variablesWithMissingValues) > 0 {
			return nil, xerrors.Errorf("required template variables need values: %s", strings.Join(variablesWithMissingValues, ", "))
		}

		return &proto.UpdateJobResponse{
			Canceled:       job.CanceledAt.Valid,
			VariableValues: variableValues,
		}, nil
	}

	return &proto.UpdateJobResponse{
		Canceled: job.CanceledAt.Valid,
	}, nil
}

func (server *Server) FailJob(ctx context.Context, failJob *proto.FailedJob) (*proto.Empty, error) {
	ctx, span := server.startTrace(ctx, tracing.FuncName())
	defer span.End()

	//nolint:gocritic // Provisionerd has specific authz rules.
	ctx = dbauthz.AsProvisionerd(ctx)
	jobID, err := uuid.Parse(failJob.JobId)
	if err != nil {
		return nil, xerrors.Errorf("parse job id: %w", err)
	}
	server.Logger.Debug(ctx, "stage FailJob starting", slog.F("job_id", jobID))
	job, err := server.Database.GetProvisionerJobByID(ctx, jobID)
	if err != nil {
		return nil, xerrors.Errorf("get provisioner job: %w", err)
	}
	if job.WorkerID.UUID.String() != server.ID.String() {
		return nil, xerrors.New("you don't own this job")
	}
	if job.CompletedAt.Valid {
		return nil, xerrors.Errorf("job already completed")
	}
	job.CompletedAt = sql.NullTime{
		Time:  database.Now(),
		Valid: true,
	}
	job.Error = sql.NullString{
		String: failJob.Error,
		Valid:  failJob.Error != "",
	}
	job.ErrorCode = sql.NullString{
		String: failJob.ErrorCode,
		Valid:  failJob.ErrorCode != "",
	}

	err = server.Database.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
		ID:          jobID,
		CompletedAt: job.CompletedAt,
		UpdatedAt:   database.Now(),
		Error:       job.Error,
		ErrorCode:   job.ErrorCode,
	})
	if err != nil {
		return nil, xerrors.Errorf("update provisioner job: %w", err)
	}
	server.Telemetry.Report(&telemetry.Snapshot{
		ProvisionerJobs: []telemetry.ProvisionerJob{telemetry.ConvertProvisionerJob(job)},
	})

	switch jobType := failJob.Type.(type) {
	case *proto.FailedJob_WorkspaceBuild_:
		if jobType.WorkspaceBuild.State == nil {
			break
		}
		var input WorkspaceProvisionJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal workspace provision input: %w", err)
		}

		var build database.WorkspaceBuild
		err := server.Database.InTx(func(db database.Store) error {
			workspaceBuild, err := db.GetWorkspaceBuildByID(ctx, input.WorkspaceBuildID)
			if err != nil {
				return xerrors.Errorf("get workspace build: %w", err)
			}

			build, err = db.UpdateWorkspaceBuildByID(ctx, database.UpdateWorkspaceBuildByIDParams{
				ID:               input.WorkspaceBuildID,
				UpdatedAt:        database.Now(),
				ProvisionerState: jobType.WorkspaceBuild.State,
				Deadline:         workspaceBuild.Deadline,
				MaxDeadline:      workspaceBuild.MaxDeadline,
			})
			if err != nil {
				return xerrors.Errorf("update workspace build state: %w", err)
			}

			return nil
		}, nil)
		if err != nil {
			return nil, err
		}

		err = server.Pubsub.Publish(codersdk.WorkspaceNotifyChannel(build.WorkspaceID), []byte{})
		if err != nil {
			return nil, xerrors.Errorf("update workspace: %w", err)
		}
	case *proto.FailedJob_TemplateImport_:
	}

	// if failed job is a workspace build, audit the outcome
	if job.Type == database.ProvisionerJobTypeWorkspaceBuild {
		auditor := server.Auditor.Load()
		build, err := server.Database.GetWorkspaceBuildByJobID(ctx, job.ID)
		if err != nil {
			server.Logger.Error(ctx, "audit log - get build", slog.Error(err))
		} else {
			auditAction := auditActionFromTransition(build.Transition)
			workspace, err := server.Database.GetWorkspaceByID(ctx, build.WorkspaceID)
			if err != nil {
				server.Logger.Error(ctx, "audit log - get workspace", slog.Error(err))
			} else {
				previousBuildNumber := build.BuildNumber - 1
				previousBuild, prevBuildErr := server.Database.GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx, database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
					WorkspaceID: workspace.ID,
					BuildNumber: previousBuildNumber,
				})
				if prevBuildErr != nil {
					previousBuild = database.WorkspaceBuild{}
				}

				// We pass the below information to the Auditor so that it
				// can form a friendly string for the user to view in the UI.
				buildResourceInfo := audit.AdditionalFields{
					WorkspaceName: workspace.Name,
					BuildNumber:   strconv.FormatInt(int64(build.BuildNumber), 10),
					BuildReason:   database.BuildReason(string(build.Reason)),
				}

				wriBytes, err := json.Marshal(buildResourceInfo)
				if err != nil {
					server.Logger.Error(ctx, "marshal workspace resource info for failed job", slog.Error(err))
				}

				audit.BuildAudit(ctx, &audit.BuildAuditParams[database.WorkspaceBuild]{
					Audit:            *auditor,
					Log:              server.Logger,
					UserID:           job.InitiatorID,
					JobID:            job.ID,
					Action:           auditAction,
					Old:              previousBuild,
					New:              build,
					Status:           http.StatusInternalServerError,
					AdditionalFields: wriBytes,
				})
			}
		}
	}

	data, err := json.Marshal(provisionersdk.ProvisionerJobLogsNotifyMessage{EndOfLogs: true})
	if err != nil {
		return nil, xerrors.Errorf("marshal job log: %w", err)
	}
	err = server.Pubsub.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(jobID), data)
	if err != nil {
		server.Logger.Error(ctx, "failed to publish end of job logs", slog.F("job_id", jobID), slog.Error(err))
		return nil, xerrors.Errorf("publish end of job logs: %w", err)
	}
	return &proto.Empty{}, nil
}

// CompleteJob is triggered by a provision daemon to mark a provisioner job as completed.
//
//nolint:gocyclo
func (server *Server) CompleteJob(ctx context.Context, completed *proto.CompletedJob) (*proto.Empty, error) {
	ctx, span := server.startTrace(ctx, tracing.FuncName())
	defer span.End()

	//nolint:gocritic // Provisionerd has specific authz rules.
	ctx = dbauthz.AsProvisionerd(ctx)
	jobID, err := uuid.Parse(completed.JobId)
	if err != nil {
		return nil, xerrors.Errorf("parse job id: %w", err)
	}
	server.Logger.Debug(ctx, "stage CompleteJob starting", slog.F("job_id", jobID))
	job, err := server.Database.GetProvisionerJobByID(ctx, jobID)
	if err != nil {
		return nil, xerrors.Errorf("get job by id: %w", err)
	}
	if job.WorkerID.UUID.String() != server.ID.String() {
		return nil, xerrors.Errorf("you don't own this job")
	}

	telemetrySnapshot := &telemetry.Snapshot{}
	// Items are added to this snapshot as they complete!
	defer server.Telemetry.Report(telemetrySnapshot)

	switch jobType := completed.Type.(type) {
	case *proto.CompletedJob_TemplateImport_:
		var input TemplateVersionImportJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, xerrors.Errorf("template version ID is expected: %w", err)
		}

		for transition, resources := range map[database.WorkspaceTransition][]*sdkproto.Resource{
			database.WorkspaceTransitionStart: jobType.TemplateImport.StartResources,
			database.WorkspaceTransitionStop:  jobType.TemplateImport.StopResources,
		} {
			for _, resource := range resources {
				server.Logger.Info(ctx, "inserting template import job resource",
					slog.F("job_id", job.ID.String()),
					slog.F("resource_name", resource.Name),
					slog.F("resource_type", resource.Type),
					slog.F("transition", transition))

				err = InsertWorkspaceResource(ctx, server.Database, jobID, transition, resource, telemetrySnapshot)
				if err != nil {
					return nil, xerrors.Errorf("insert resource: %w", err)
				}
			}
		}

		for _, richParameter := range jobType.TemplateImport.RichParameters {
			server.Logger.Info(ctx, "inserting template import job parameter",
				slog.F("job_id", job.ID.String()),
				slog.F("parameter_name", richParameter.Name),
			)
			options, err := json.Marshal(richParameter.Options)
			if err != nil {
				return nil, xerrors.Errorf("marshal parameter options: %w", err)
			}

			var validationMin, validationMax sql.NullInt32
			if richParameter.ValidationMin != nil {
				validationMin = sql.NullInt32{
					Int32: *richParameter.ValidationMin,
					Valid: true,
				}
			}
			if richParameter.ValidationMax != nil {
				validationMax = sql.NullInt32{
					Int32: *richParameter.ValidationMax,
					Valid: true,
				}
			}

			_, err = server.Database.InsertTemplateVersionParameter(ctx, database.InsertTemplateVersionParameterParams{
				TemplateVersionID:   input.TemplateVersionID,
				Name:                richParameter.Name,
				DisplayName:         richParameter.DisplayName,
				Description:         richParameter.Description,
				Type:                richParameter.Type,
				Mutable:             richParameter.Mutable,
				DefaultValue:        richParameter.DefaultValue,
				Icon:                richParameter.Icon,
				Options:             options,
				ValidationRegex:     richParameter.ValidationRegex,
				ValidationError:     richParameter.ValidationError,
				ValidationMin:       validationMin,
				ValidationMax:       validationMax,
				ValidationMonotonic: richParameter.ValidationMonotonic,
				Required:            richParameter.Required,
				LegacyVariableName:  richParameter.LegacyVariableName,
			})
			if err != nil {
				return nil, xerrors.Errorf("insert parameter: %w", err)
			}
		}

		var completedError sql.NullString

		for _, gitAuthProvider := range jobType.TemplateImport.GitAuthProviders {
			contains := false
			for _, configuredProvider := range server.GitAuthConfigs {
				if configuredProvider.ID == gitAuthProvider {
					contains = true
					break
				}
			}
			if !contains {
				completedError = sql.NullString{
					String: fmt.Sprintf("git auth provider %q is not configured", gitAuthProvider),
					Valid:  true,
				}
				break
			}
		}

		err = server.Database.UpdateTemplateVersionGitAuthProvidersByJobID(ctx, database.UpdateTemplateVersionGitAuthProvidersByJobIDParams{
			JobID:            jobID,
			GitAuthProviders: jobType.TemplateImport.GitAuthProviders,
			UpdatedAt:        database.Now(),
		})
		if err != nil {
			return nil, xerrors.Errorf("update template version git auth providers: %w", err)
		}

		err = server.Database.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID:        jobID,
			UpdatedAt: database.Now(),
			CompletedAt: sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			},
			Error: completedError,
		})
		if err != nil {
			return nil, xerrors.Errorf("update provisioner job: %w", err)
		}
		server.Logger.Debug(ctx, "marked import job as completed", slog.F("job_id", jobID))
		if err != nil {
			return nil, xerrors.Errorf("complete job: %w", err)
		}
	case *proto.CompletedJob_WorkspaceBuild_:
		var input WorkspaceProvisionJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal job data: %w", err)
		}

		workspaceBuild, err := server.Database.GetWorkspaceBuildByID(ctx, input.WorkspaceBuildID)
		if err != nil {
			return nil, xerrors.Errorf("get workspace build: %w", err)
		}

		var workspace database.Workspace
		var getWorkspaceError error

		err = server.Database.InTx(func(db database.Store) error {
			var (
				now = database.Now()
				// deadline is the time when the workspace will be stopped. The
				// value can be bumped by user activity or manually by the user
				// via the UI.
				deadline time.Time
				// maxDeadline is the maximum value for deadline.
				maxDeadline time.Time
			)

			workspace, getWorkspaceError = db.GetWorkspaceByID(ctx, workspaceBuild.WorkspaceID)
			if getWorkspaceError != nil {
				server.Logger.Error(ctx,
					"fetch workspace for build",
					slog.F("workspace_build_id", workspaceBuild.ID),
					slog.F("workspace_id", workspaceBuild.WorkspaceID),
				)
				return getWorkspaceError
			}
			if workspace.Ttl.Valid {
				deadline = now.Add(time.Duration(workspace.Ttl.Int64))
			}

			templateSchedule, err := (*server.TemplateScheduleStore.Load()).GetTemplateScheduleOptions(ctx, db, workspace.TemplateID)
			if err != nil {
				return xerrors.Errorf("get template schedule options: %w", err)
			}
			if !templateSchedule.UserAutostopEnabled {
				// The user is not permitted to set their own TTL, so use the
				// template default.
				deadline = time.Time{}
				if templateSchedule.DefaultTTL > 0 {
					deadline = now.Add(templateSchedule.DefaultTTL)
				}
			}
			if templateSchedule.MaxTTL > 0 {
				maxDeadline = now.Add(templateSchedule.MaxTTL)

				if deadline.IsZero() || maxDeadline.Before(deadline) {
					// If the workspace doesn't have a deadline or the max
					// deadline is sooner than the workspace deadline, use the
					// max deadline as the actual deadline.
					deadline = maxDeadline
				}
			}

			err = db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
				ID:        jobID,
				UpdatedAt: database.Now(),
				CompletedAt: sql.NullTime{
					Time:  database.Now(),
					Valid: true,
				},
			})
			if err != nil {
				return xerrors.Errorf("update provisioner job: %w", err)
			}
			_, err = db.UpdateWorkspaceBuildByID(ctx, database.UpdateWorkspaceBuildByIDParams{
				ID:               workspaceBuild.ID,
				Deadline:         deadline,
				MaxDeadline:      maxDeadline,
				ProvisionerState: jobType.WorkspaceBuild.State,
				UpdatedAt:        now,
			})
			if err != nil {
				return xerrors.Errorf("update workspace build: %w", err)
			}

			agentTimeouts := make(map[time.Duration]bool) // A set of agent timeouts.
			// This could be a bulk insert to improve performance.
			for _, protoResource := range jobType.WorkspaceBuild.Resources {
				for _, protoAgent := range protoResource.Agents {
					dur := time.Duration(protoAgent.GetConnectionTimeoutSeconds()) * time.Second
					agentTimeouts[dur] = true
				}
				err = InsertWorkspaceResource(ctx, db, job.ID, workspaceBuild.Transition, protoResource, telemetrySnapshot)
				if err != nil {
					return xerrors.Errorf("insert provisioner job: %w", err)
				}
			}

			// On start, we want to ensure that workspace agents timeout statuses
			// are propagated. This method is simple and does not protect against
			// notifying in edge cases like when a workspace is stopped soon
			// after being started.
			//
			// Agent timeouts could be minutes apart, resulting in an unresponsive
			// experience, so we'll notify after every unique timeout seconds.
			if !input.DryRun && workspaceBuild.Transition == database.WorkspaceTransitionStart && len(agentTimeouts) > 0 {
				timeouts := maps.Keys(agentTimeouts)
				slices.Sort(timeouts)

				var updates []<-chan time.Time
				for _, d := range timeouts {
					server.Logger.Debug(ctx, "triggering workspace notification after agent timeout",
						slog.F("workspace_build_id", workspaceBuild.ID),
						slog.F("timeout", d),
					)
					// Agents are inserted with `database.Now()`, this triggers a
					// workspace event approximately after created + timeout seconds.
					updates = append(updates, time.After(d))
				}
				go func() {
					for _, wait := range updates {
						// Wait for the next potential timeout to occur. Note that we
						// can't listen on the context here because we will hang around
						// after this function has returned. The server also doesn't
						// have a shutdown signal we can listen to.
						<-wait
						if err := server.Pubsub.Publish(codersdk.WorkspaceNotifyChannel(workspaceBuild.WorkspaceID), []byte{}); err != nil {
							server.Logger.Error(ctx, "workspace notification after agent timeout failed",
								slog.F("workspace_build_id", workspaceBuild.ID),
								slog.Error(err),
							)
						}
					}
				}()
			}

			if workspaceBuild.Transition != database.WorkspaceTransitionDelete {
				// This is for deleting a workspace!
				return nil
			}

			err = db.UpdateWorkspaceDeletedByID(ctx, database.UpdateWorkspaceDeletedByIDParams{
				ID:      workspaceBuild.WorkspaceID,
				Deleted: true,
			})
			if err != nil {
				return xerrors.Errorf("update workspace deleted: %w", err)
			}

			return nil
		}, nil)
		if err != nil {
			return nil, xerrors.Errorf("complete job: %w", err)
		}

		// audit the outcome of the workspace build
		if getWorkspaceError == nil {
			auditor := server.Auditor.Load()
			auditAction := auditActionFromTransition(workspaceBuild.Transition)

			previousBuildNumber := workspaceBuild.BuildNumber - 1
			previousBuild, prevBuildErr := server.Database.GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx, database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
				WorkspaceID: workspace.ID,
				BuildNumber: previousBuildNumber,
			})
			if prevBuildErr != nil {
				previousBuild = database.WorkspaceBuild{}
			}

			// We pass the below information to the Auditor so that it
			// can form a friendly string for the user to view in the UI.
			buildResourceInfo := audit.AdditionalFields{
				WorkspaceName: workspace.Name,
				BuildNumber:   strconv.FormatInt(int64(workspaceBuild.BuildNumber), 10),
				BuildReason:   database.BuildReason(string(workspaceBuild.Reason)),
			}

			wriBytes, err := json.Marshal(buildResourceInfo)
			if err != nil {
				server.Logger.Error(ctx, "marshal resource info for successful job", slog.Error(err))
			}

			audit.BuildAudit(ctx, &audit.BuildAuditParams[database.WorkspaceBuild]{
				Audit:            *auditor,
				Log:              server.Logger,
				UserID:           job.InitiatorID,
				JobID:            job.ID,
				Action:           auditAction,
				Old:              previousBuild,
				New:              workspaceBuild,
				Status:           http.StatusOK,
				AdditionalFields: wriBytes,
			})
		}

		err = server.Pubsub.Publish(codersdk.WorkspaceNotifyChannel(workspaceBuild.WorkspaceID), []byte{})
		if err != nil {
			return nil, xerrors.Errorf("update workspace: %w", err)
		}
	case *proto.CompletedJob_TemplateDryRun_:
		for _, resource := range jobType.TemplateDryRun.Resources {
			server.Logger.Info(ctx, "inserting template dry-run job resource",
				slog.F("job_id", job.ID.String()),
				slog.F("resource_name", resource.Name),
				slog.F("resource_type", resource.Type))

			err = InsertWorkspaceResource(ctx, server.Database, jobID, database.WorkspaceTransitionStart, resource, telemetrySnapshot)
			if err != nil {
				return nil, xerrors.Errorf("insert resource: %w", err)
			}
		}

		err = server.Database.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID:        jobID,
			UpdatedAt: database.Now(),
			CompletedAt: sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			},
		})
		if err != nil {
			return nil, xerrors.Errorf("update provisioner job: %w", err)
		}
		server.Logger.Debug(ctx, "marked template dry-run job as completed", slog.F("job_id", jobID))
		if err != nil {
			return nil, xerrors.Errorf("complete job: %w", err)
		}

	default:
		if completed.Type == nil {
			return nil, xerrors.Errorf("type payload must be provided")
		}
		return nil, xerrors.Errorf("unknown job type %q; ensure coderd and provisionerd versions match",
			reflect.TypeOf(completed.Type).String())
	}

	data, err := json.Marshal(provisionersdk.ProvisionerJobLogsNotifyMessage{EndOfLogs: true})
	if err != nil {
		return nil, xerrors.Errorf("marshal job log: %w", err)
	}
	err = server.Pubsub.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(jobID), data)
	if err != nil {
		server.Logger.Error(ctx, "failed to publish end of job logs", slog.F("job_id", jobID), slog.Error(err))
		return nil, xerrors.Errorf("publish end of job logs: %w", err)
	}

	server.Logger.Debug(ctx, "stage CompleteJob done", slog.F("job_id", jobID))
	return &proto.Empty{}, nil
}

func (server *Server) startTrace(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return server.Tracer.Start(ctx, name, append(opts, trace.WithAttributes(
		semconv.ServiceNameKey.String("coderd.provisionerd"),
	))...)
}

func InsertWorkspaceResource(ctx context.Context, db database.Store, jobID uuid.UUID, transition database.WorkspaceTransition, protoResource *sdkproto.Resource, snapshot *telemetry.Snapshot) error {
	resource, err := db.InsertWorkspaceResource(ctx, database.InsertWorkspaceResourceParams{
		ID:         uuid.New(),
		CreatedAt:  database.Now(),
		JobID:      jobID,
		Transition: transition,
		Type:       protoResource.Type,
		Name:       protoResource.Name,
		Hide:       protoResource.Hide,
		Icon:       protoResource.Icon,
		DailyCost:  protoResource.DailyCost,
		InstanceType: sql.NullString{
			String: protoResource.InstanceType,
			Valid:  protoResource.InstanceType != "",
		},
	})
	if err != nil {
		return xerrors.Errorf("insert provisioner job resource %q: %w", protoResource.Name, err)
	}
	snapshot.WorkspaceResources = append(snapshot.WorkspaceResources, telemetry.ConvertWorkspaceResource(resource))

	var (
		agentNames = make(map[string]struct{})
		appSlugs   = make(map[string]struct{})
	)
	for _, prAgent := range protoResource.Agents {
		if _, ok := agentNames[prAgent.Name]; ok {
			return xerrors.Errorf("duplicate agent name %q", prAgent.Name)
		}
		agentNames[prAgent.Name] = struct{}{}

		var instanceID sql.NullString
		if prAgent.GetInstanceId() != "" {
			instanceID = sql.NullString{
				String: prAgent.GetInstanceId(),
				Valid:  true,
			}
		}
		var env pqtype.NullRawMessage
		if prAgent.Env != nil {
			data, err := json.Marshal(prAgent.Env)
			if err != nil {
				return xerrors.Errorf("marshal env: %w", err)
			}
			env = pqtype.NullRawMessage{
				RawMessage: data,
				Valid:      true,
			}
		}
		authToken := uuid.New()
		if prAgent.GetToken() != "" {
			authToken, err = uuid.Parse(prAgent.GetToken())
			if err != nil {
				return xerrors.Errorf("invalid auth token format; must be uuid: %w", err)
			}
		}

		// Set the default in case it was not provided (e.g. echo provider).
		if prAgent.GetStartupScriptBehavior() == "" {
			prAgent.StartupScriptBehavior = string(codersdk.WorkspaceAgentStartupScriptBehaviorNonBlocking)
		}

		agentID := uuid.New()
		dbAgent, err := db.InsertWorkspaceAgent(ctx, database.InsertWorkspaceAgentParams{
			ID:                   agentID,
			CreatedAt:            database.Now(),
			UpdatedAt:            database.Now(),
			ResourceID:           resource.ID,
			Name:                 prAgent.Name,
			AuthToken:            authToken,
			AuthInstanceID:       instanceID,
			Architecture:         prAgent.Architecture,
			EnvironmentVariables: env,
			Directory:            prAgent.Directory,
			OperatingSystem:      prAgent.OperatingSystem,
			StartupScript: sql.NullString{
				String: prAgent.StartupScript,
				Valid:  prAgent.StartupScript != "",
			},
			ConnectionTimeoutSeconds:    prAgent.GetConnectionTimeoutSeconds(),
			TroubleshootingURL:          prAgent.GetTroubleshootingUrl(),
			MOTDFile:                    prAgent.GetMotdFile(),
			StartupScriptBehavior:       database.StartupScriptBehavior(prAgent.GetStartupScriptBehavior()),
			StartupScriptTimeoutSeconds: prAgent.GetStartupScriptTimeoutSeconds(),
			ShutdownScript: sql.NullString{
				String: prAgent.ShutdownScript,
				Valid:  prAgent.ShutdownScript != "",
			},
			ShutdownScriptTimeoutSeconds: prAgent.GetShutdownScriptTimeoutSeconds(),
		})
		if err != nil {
			return xerrors.Errorf("insert agent: %w", err)
		}
		snapshot.WorkspaceAgents = append(snapshot.WorkspaceAgents, telemetry.ConvertWorkspaceAgent(dbAgent))

		for _, md := range prAgent.Metadata {
			p := database.InsertWorkspaceAgentMetadataParams{
				WorkspaceAgentID: agentID,
				DisplayName:      md.DisplayName,
				Script:           md.Script,
				Key:              md.Key,
				Timeout:          md.Timeout,
				Interval:         md.Interval,
			}
			err := db.InsertWorkspaceAgentMetadata(ctx, p)
			if err != nil {
				return xerrors.Errorf("insert agent metadata: %w, params: %+v", err, p)
			}
		}

		for _, app := range prAgent.Apps {
			slug := app.Slug
			if slug == "" {
				return xerrors.Errorf("app must have a slug or name set")
			}
			if !provisioner.AppSlugRegex.MatchString(slug) {
				return xerrors.Errorf("app slug %q does not match regex %q", slug, provisioner.AppSlugRegex.String())
			}
			if _, exists := appSlugs[slug]; exists {
				return xerrors.Errorf("duplicate app slug, must be unique per template: %q", slug)
			}
			appSlugs[slug] = struct{}{}

			health := database.WorkspaceAppHealthDisabled
			if app.Healthcheck == nil {
				app.Healthcheck = &sdkproto.Healthcheck{}
			}
			if app.Healthcheck.Url != "" {
				health = database.WorkspaceAppHealthInitializing
			}

			sharingLevel := database.AppSharingLevelOwner
			switch app.SharingLevel {
			case sdkproto.AppSharingLevel_AUTHENTICATED:
				sharingLevel = database.AppSharingLevelAuthenticated
			case sdkproto.AppSharingLevel_PUBLIC:
				sharingLevel = database.AppSharingLevelPublic
			}

			dbApp, err := db.InsertWorkspaceApp(ctx, database.InsertWorkspaceAppParams{
				ID:          uuid.New(),
				CreatedAt:   database.Now(),
				AgentID:     dbAgent.ID,
				Slug:        slug,
				DisplayName: app.DisplayName,
				Icon:        app.Icon,
				Command: sql.NullString{
					String: app.Command,
					Valid:  app.Command != "",
				},
				Url: sql.NullString{
					String: app.Url,
					Valid:  app.Url != "",
				},
				External:             app.External,
				Subdomain:            app.Subdomain,
				SharingLevel:         sharingLevel,
				HealthcheckUrl:       app.Healthcheck.Url,
				HealthcheckInterval:  app.Healthcheck.Interval,
				HealthcheckThreshold: app.Healthcheck.Threshold,
				Health:               health,
			})
			if err != nil {
				return xerrors.Errorf("insert app: %w", err)
			}
			snapshot.WorkspaceApps = append(snapshot.WorkspaceApps, telemetry.ConvertWorkspaceApp(dbApp))
		}
	}

	arg := database.InsertWorkspaceResourceMetadataParams{
		WorkspaceResourceID: resource.ID,
		Key:                 []string{},
		Value:               []string{},
		Sensitive:           []bool{},
	}
	for _, metadatum := range protoResource.Metadata {
		if metadatum.IsNull {
			continue
		}
		arg.Key = append(arg.Key, metadatum.Key)
		arg.Value = append(arg.Value, metadatum.Value)
		arg.Sensitive = append(arg.Sensitive, metadatum.Sensitive)
	}
	_, err = db.InsertWorkspaceResourceMetadata(ctx, arg)
	if err != nil {
		return xerrors.Errorf("insert workspace resource metadata: %w", err)
	}

	return nil
}

func workspaceSessionTokenName(workspace database.Workspace) string {
	return fmt.Sprintf("%s_%s_session_token", workspace.OwnerID, workspace.ID)
}

func (server *Server) regenerateSessionToken(ctx context.Context, user database.User, workspace database.Workspace) (string, error) {
	newkey, sessionToken, err := apikey.Generate(apikey.CreateParams{
		UserID:           user.ID,
		LoginType:        user.LoginType,
		DeploymentValues: server.DeploymentValues,
		TokenName:        workspaceSessionTokenName(workspace),
		LifetimeSeconds:  int64(server.DeploymentValues.MaxTokenLifetime.Value().Seconds()),
	})
	if err != nil {
		return "", xerrors.Errorf("generate API key: %w", err)
	}

	err = server.Database.InTx(func(tx database.Store) error {
		err := deleteSessionToken(ctx, tx, workspace)
		if err != nil {
			return xerrors.Errorf("delete session token: %w", err)
		}

		_, err = tx.InsertAPIKey(ctx, newkey)
		if err != nil {
			return xerrors.Errorf("insert API key: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return "", xerrors.Errorf("create API key: %w", err)
	}

	return sessionToken, nil
}

func deleteSessionToken(ctx context.Context, db database.Store, workspace database.Workspace) error {
	err := db.InTx(func(tx database.Store) error {
		key, err := tx.GetAPIKeyByName(ctx, database.GetAPIKeyByNameParams{
			UserID:    workspace.OwnerID,
			TokenName: workspaceSessionTokenName(workspace),
		})
		if err == nil {
			err = tx.DeleteAPIKeyByID(ctx, key.ID)
		}

		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("get api key by name: %w", err)
		}

		return nil
	}, nil)
	if err != nil {
		return xerrors.Errorf("in tx: %w", err)
	}

	return nil
}

// obtainOIDCAccessToken returns a valid OpenID Connect access token
// for the user if it's able to obtain one, otherwise it returns an empty string.
func obtainOIDCAccessToken(ctx context.Context, db database.Store, oidcConfig httpmw.OAuth2Config, userID uuid.UUID) (string, error) {
	link, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
		UserID:    userID,
		LoginType: database.LoginTypeOIDC,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return "", xerrors.Errorf("get owner oidc link: %w", err)
	}

	if link.OAuthExpiry.Before(database.Now()) && !link.OAuthExpiry.IsZero() && link.OAuthRefreshToken != "" {
		token, err := oidcConfig.TokenSource(ctx, &oauth2.Token{
			AccessToken:  link.OAuthAccessToken,
			RefreshToken: link.OAuthRefreshToken,
			Expiry:       link.OAuthExpiry,
		}).Token()
		if err != nil {
			// If OIDC fails to refresh, we return an empty string and don't fail.
			// There isn't a way to hard-opt in to OIDC from a template, so we don't
			// want to fail builds if users haven't authenticated for a while or something.
			return "", nil
		}
		link.OAuthAccessToken = token.AccessToken
		link.OAuthRefreshToken = token.RefreshToken
		link.OAuthExpiry = token.Expiry

		link, err = db.UpdateUserLink(ctx, database.UpdateUserLinkParams{
			UserID:            userID,
			LoginType:         database.LoginTypeOIDC,
			OAuthAccessToken:  link.OAuthAccessToken,
			OAuthRefreshToken: link.OAuthRefreshToken,
			OAuthExpiry:       link.OAuthExpiry,
		})
		if err != nil {
			return "", xerrors.Errorf("update user link: %w", err)
		}
	}

	return link.OAuthAccessToken, nil
}

func convertLogLevel(logLevel sdkproto.LogLevel) (database.LogLevel, error) {
	switch logLevel {
	case sdkproto.LogLevel_TRACE:
		return database.LogLevelTrace, nil
	case sdkproto.LogLevel_DEBUG:
		return database.LogLevelDebug, nil
	case sdkproto.LogLevel_INFO:
		return database.LogLevelInfo, nil
	case sdkproto.LogLevel_WARN:
		return database.LogLevelWarn, nil
	case sdkproto.LogLevel_ERROR:
		return database.LogLevelError, nil
	default:
		return database.LogLevel(""), xerrors.Errorf("unknown log level: %d", logLevel)
	}
}

func convertLogSource(logSource proto.LogSource) (database.LogSource, error) {
	switch logSource {
	case proto.LogSource_PROVISIONER_DAEMON:
		return database.LogSourceProvisionerDaemon, nil
	case proto.LogSource_PROVISIONER:
		return database.LogSourceProvisioner, nil
	default:
		return database.LogSource(""), xerrors.Errorf("unknown log source: %d", logSource)
	}
}

func convertRichParameterValues(workspaceBuildParameters []database.WorkspaceBuildParameter) []*sdkproto.RichParameterValue {
	protoParameters := make([]*sdkproto.RichParameterValue, len(workspaceBuildParameters))
	for i, buildParameter := range workspaceBuildParameters {
		protoParameters[i] = &sdkproto.RichParameterValue{
			Name:  buildParameter.Name,
			Value: buildParameter.Value,
		}
	}
	return protoParameters
}

func convertVariableValues(variableValues []codersdk.VariableValue) []*sdkproto.VariableValue {
	protoVariableValues := make([]*sdkproto.VariableValue, len(variableValues))
	for i, variableValue := range variableValues {
		protoVariableValues[i] = &sdkproto.VariableValue{
			Name:      variableValue.Name,
			Value:     variableValue.Value,
			Sensitive: true, // Without the template variable schema we have to assume that every variable may be sensitive.
		}
	}
	return protoVariableValues
}

func convertWorkspaceTransition(transition database.WorkspaceTransition) (sdkproto.WorkspaceTransition, error) {
	switch transition {
	case database.WorkspaceTransitionStart:
		return sdkproto.WorkspaceTransition_START, nil
	case database.WorkspaceTransitionStop:
		return sdkproto.WorkspaceTransition_STOP, nil
	case database.WorkspaceTransitionDelete:
		return sdkproto.WorkspaceTransition_DESTROY, nil
	default:
		return 0, xerrors.Errorf("unrecognized transition: %q", transition)
	}
}

func auditActionFromTransition(transition database.WorkspaceTransition) database.AuditAction {
	switch transition {
	case database.WorkspaceTransitionStart:
		return database.AuditActionStart
	case database.WorkspaceTransitionStop:
		return database.AuditActionStop
	case database.WorkspaceTransitionDelete:
		return database.AuditActionDelete
	default:
		return database.AuditActionWrite
	}
}

type TemplateVersionImportJob struct {
	TemplateVersionID  uuid.UUID                `json:"template_version_id"`
	UserVariableValues []codersdk.VariableValue `json:"user_variable_values"`
}

// WorkspaceProvisionJob is the payload for the "workspace_provision" job type.
type WorkspaceProvisionJob struct {
	WorkspaceBuildID uuid.UUID `json:"workspace_build_id"`
	DryRun           bool      `json:"dry_run"`
	LogLevel         string    `json:"log_level,omitempty"`
}

// TemplateVersionDryRunJob is the payload for the "template_version_dry_run" job type.
type TemplateVersionDryRunJob struct {
	TemplateVersionID   uuid.UUID                          `json:"template_version_id"`
	WorkspaceName       string                             `json:"workspace_name"`
	RichParameterValues []database.WorkspaceBuildParameter `json:"rich_parameter_values"`
}

func asVariableValues(templateVariables []database.TemplateVersionVariable) []*sdkproto.VariableValue {
	var apiVariableValues []*sdkproto.VariableValue
	for _, v := range templateVariables {
		value := v.Value
		if value == "" && v.DefaultValue != "" {
			value = v.DefaultValue
		}

		if value != "" || v.Required {
			apiVariableValues = append(apiVariableValues, &sdkproto.VariableValue{
				Name:      v.Name,
				Value:     v.Value,
				Sensitive: v.Sensitive,
			})
		}
	}
	return apiVariableValues
}

func redactTemplateVariable(templateVariable *sdkproto.TemplateVariable) *sdkproto.TemplateVariable {
	if templateVariable == nil {
		return nil
	}
	maybeRedacted := &sdkproto.TemplateVariable{
		Name:         templateVariable.Name,
		Description:  templateVariable.Description,
		Type:         templateVariable.Type,
		DefaultValue: templateVariable.DefaultValue,
		Required:     templateVariable.Required,
		Sensitive:    templateVariable.Sensitive,
	}
	if maybeRedacted.Sensitive {
		maybeRedacted.DefaultValue = "*redacted*"
	}
	return maybeRedacted
}
