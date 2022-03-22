package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/tabbed/pqtype"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/provisionerd/proto"
	"github.com/coder/coder/provisionersdk"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
)

// Serves the provisioner daemon protobuf API over a WebSocket.
func (api *api) provisionerDaemonsListen(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitGroup.Add(1)
	defer api.websocketWaitGroup.Done()

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("accept websocket: %s", err),
		})
		return
	}

	daemon, err := api.Database.InsertProvisionerDaemon(r.Context(), database.InsertProvisionerDaemonParams{
		ID:           uuid.New(),
		CreatedAt:    database.Now(),
		Name:         namesgenerator.GetRandomName(1),
		Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho, database.ProvisionerTypeTerraform},
	})
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, fmt.Sprintf("insert provisioner daemon:% s", err))
		return
	}

	// Multiplexes the incoming connection using yamux.
	// This allows multiple function calls to occur over
	// the same connection.
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Server(websocket.NetConn(r.Context(), conn, websocket.MessageBinary), config)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, fmt.Sprintf("multiplex server: %s", err))
		return
	}
	mux := drpcmux.New()
	err = proto.DRPCRegisterProvisionerDaemon(mux, &provisionerdServer{
		AccessURL:    api.AccessURL,
		ID:           daemon.ID,
		Database:     api.Database,
		Pubsub:       api.Pubsub,
		Provisioners: daemon.Provisioners,
		Logger:       api.Logger.Named(fmt.Sprintf("provisionerd-%s", daemon.Name)),
	})
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, fmt.Sprintf("drpc register provisioner daemon: %s", err))
		return
	}
	server := drpcserver.New(mux)
	err = server.Serve(r.Context(), session)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, fmt.Sprintf("serve: %s", err))
		return
	}
	_ = conn.Close(websocket.StatusGoingAway, "")
}

// The input for a "workspace_provision" job.
type workspaceProvisionJob struct {
	WorkspaceBuildID uuid.UUID `json:"workspace_build_id"`
	DryRun           bool      `json:"dry_run"`
}

// Implementation of the provisioner daemon protobuf server.
type provisionerdServer struct {
	AccessURL    *url.URL
	ID           uuid.UUID
	Logger       slog.Logger
	Provisioners []database.ProvisionerType
	Database     database.Store
	Pubsub       database.Pubsub
}

// AcquireJob queries the database to lock a job.
func (server *provisionerdServer) AcquireJob(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
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
	})
	if errors.Is(err, sql.ErrNoRows) {
		// The provisioner daemon assumes no jobs are available if
		// an empty struct is returned.
		return &proto.AcquiredJob{}, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("acquire job: %w", err)
	}
	server.Logger.Debug(ctx, "locked job from database", slog.F("id", job.ID))

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

	protoJob := &proto.AcquiredJob{
		JobId:       job.ID.String(),
		CreatedAt:   job.CreatedAt.UnixMilli(),
		Provisioner: string(job.Provisioner),
		UserName:    user.Username,
	}
	switch job.Type {
	case database.ProvisionerJobTypeWorkspaceBuild:
		var input workspaceProvisionJob
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
		projectVersion, err := server.Database.GetProjectVersionByID(ctx, workspaceBuild.ProjectVersionID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get project version: %s", err))
		}
		project, err := server.Database.GetProjectByID(ctx, projectVersion.ProjectID.UUID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get project: %s", err))
		}

		// Compute parameters for the workspace to consume.
		parameters, err := parameter.Compute(ctx, server.Database, parameter.ComputeScope{
			ProjectImportJobID: projectVersion.JobID,
			OrganizationID:     job.OrganizationID,
			ProjectID: uuid.NullUUID{
				UUID:  project.ID,
				Valid: true,
			},
			UserID: user.ID,
			WorkspaceID: uuid.NullUUID{
				UUID:  workspace.ID,
				Valid: true,
			},
		}, nil)
		if err != nil {
			return nil, failJob(fmt.Sprintf("compute parameters: %s", err))
		}
		// Convert parameters to the protobuf type.
		protoParameters := make([]*sdkproto.ParameterValue, 0, len(parameters))
		for _, computedParameter := range parameters {
			converted, err := convertComputedParameterValue(computedParameter)
			if err != nil {
				return nil, failJob(fmt.Sprintf("convert parameter: %s", err))
			}
			protoParameters = append(protoParameters, converted)
		}
		transition, err := convertWorkspaceTransition(workspaceBuild.Transition)
		if err != nil {
			return nil, failJob(fmt.Sprint("convert workspace transition: %w", err))
		}

		protoJob.Type = &proto.AcquiredJob_WorkspaceBuild_{
			WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
				WorkspaceBuildId: workspaceBuild.ID.String(),
				WorkspaceName:    workspace.Name,
				State:            workspaceBuild.ProvisionerState,
				ParameterValues:  protoParameters,
				Metadata: &sdkproto.Provision_Metadata{
					CoderUrl:            server.AccessURL.String(),
					WorkspaceTransition: transition,
				},
			},
		}
	case database.ProvisionerJobTypeProjectVersionImport:
		protoJob.Type = &proto.AcquiredJob_ProjectImport_{
			ProjectImport: &proto.AcquiredJob_ProjectImport{
				Metadata: &sdkproto.Provision_Metadata{
					CoderUrl: server.AccessURL.String(),
				},
			},
		}
	}
	switch job.StorageMethod {
	case database.ProvisionerStorageMethodFile:
		file, err := server.Database.GetFileByHash(ctx, job.StorageSource)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get file by hash: %s", err))
		}
		protoJob.ProjectSourceArchive = file.Data
	default:
		return nil, failJob(fmt.Sprintf("unsupported storage method: %s", job.StorageMethod))
	}

	return protoJob, err
}

func (server *provisionerdServer) UpdateJob(ctx context.Context, request *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
	parsedID, err := uuid.Parse(request.JobId)
	if err != nil {
		return nil, xerrors.Errorf("parse job id: %w", err)
	}
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
	if job.CanceledAt.Valid {
		// Allows for graceful cancelation on the backend!
		return &proto.UpdateJobResponse{
			Canceled: true,
		}, nil
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
			insertParams.ID = append(insertParams.ID, uuid.New())
			insertParams.CreatedAt = append(insertParams.CreatedAt, time.UnixMilli(log.CreatedAt))
			insertParams.Level = append(insertParams.Level, logLevel)
			insertParams.Source = append(insertParams.Source, logSource)
			insertParams.Output = append(insertParams.Output, log.Output)
		}
		logs, err := server.Database.InsertProvisionerJobLogs(context.Background(), insertParams)
		if err != nil {
			return nil, xerrors.Errorf("insert job logs: %w", err)
		}
		data, err := json.Marshal(logs)
		if err != nil {
			return nil, xerrors.Errorf("marshal job log: %w", err)
		}
		err = server.Pubsub.Publish(provisionerJobLogsChannel(parsedID), data)
		if err != nil {
			return nil, xerrors.Errorf("publish job log: %w", err)
		}
	}

	if len(request.ParameterSchemas) > 0 {
		for _, protoParameter := range request.ParameterSchemas {
			validationTypeSystem, err := convertValidationTypeSystem(protoParameter.ValidationTypeSystem)
			if err != nil {
				return nil, xerrors.Errorf("convert validation type system for %q: %w", protoParameter.Name, err)
			}

			parameterSchema := database.InsertParameterSchemaParams{
				ID:                   uuid.New(),
				CreatedAt:            database.Now(),
				JobID:                job.ID,
				Name:                 protoParameter.Name,
				Description:          protoParameter.Description,
				RedisplayValue:       protoParameter.RedisplayValue,
				ValidationError:      protoParameter.ValidationError,
				ValidationCondition:  protoParameter.ValidationCondition,
				ValidationValueType:  protoParameter.ValidationValueType,
				ValidationTypeSystem: validationTypeSystem,

				DefaultSourceScheme:      database.ParameterSourceSchemeNone,
				DefaultDestinationScheme: database.ParameterDestinationSchemeNone,

				AllowOverrideDestination: protoParameter.AllowOverrideDestination,
				AllowOverrideSource:      protoParameter.AllowOverrideSource,
			}

			// It's possible a parameter doesn't define a default source!
			if protoParameter.DefaultSource != nil {
				parameterSourceScheme, err := convertParameterSourceScheme(protoParameter.DefaultSource.Scheme)
				if err != nil {
					return nil, xerrors.Errorf("convert parameter source scheme: %w", err)
				}
				parameterSchema.DefaultSourceScheme = parameterSourceScheme
				parameterSchema.DefaultSourceValue = protoParameter.DefaultSource.Value
			}

			// It's possible a parameter doesn't define a default destination!
			if protoParameter.DefaultDestination != nil {
				parameterDestinationScheme, err := convertParameterDestinationScheme(protoParameter.DefaultDestination.Scheme)
				if err != nil {
					return nil, xerrors.Errorf("convert parameter destination scheme: %w", err)
				}
				parameterSchema.DefaultDestinationScheme = parameterDestinationScheme
			}

			_, err = server.Database.InsertParameterSchema(ctx, parameterSchema)
			if err != nil {
				return nil, xerrors.Errorf("insert parameter schema: %w", err)
			}
		}

		var projectID uuid.NullUUID
		if job.Type == database.ProvisionerJobTypeProjectVersionImport {
			projectVersion, err := server.Database.GetProjectVersionByJobID(ctx, job.ID)
			if err != nil {
				return nil, xerrors.Errorf("get project version by job id: %w", err)
			}
			projectID = projectVersion.ProjectID
		}

		parameters, err := parameter.Compute(ctx, server.Database, parameter.ComputeScope{
			ProjectImportJobID: job.ID,
			ProjectID:          projectID,
			OrganizationID:     job.OrganizationID,
			UserID:             job.InitiatorID,
		}, nil)
		if err != nil {
			return nil, xerrors.Errorf("compute parameters: %w", err)
		}
		// Convert parameters to the protobuf type.
		protoParameters := make([]*sdkproto.ParameterValue, 0, len(parameters))
		for _, computedParameter := range parameters {
			converted, err := convertComputedParameterValue(computedParameter)
			if err != nil {
				return nil, xerrors.Errorf("convert parameter: %s", err)
			}
			protoParameters = append(protoParameters, converted)
		}

		return &proto.UpdateJobResponse{
			ParameterValues: protoParameters,
		}, nil
	}

	return &proto.UpdateJobResponse{}, nil
}

func (server *provisionerdServer) FailJob(ctx context.Context, failJob *proto.FailedJob) (*proto.Empty, error) {
	jobID, err := uuid.Parse(failJob.JobId)
	if err != nil {
		return nil, xerrors.Errorf("parse job id: %w", err)
	}
	job, err := server.Database.GetProvisionerJobByID(ctx, jobID)
	if err != nil {
		return nil, xerrors.Errorf("get provisioner job: %w", err)
	}
	if job.CompletedAt.Valid {
		return nil, xerrors.Errorf("job already completed")
	}
	err = server.Database.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
		ID: jobID,
		CompletedAt: sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		},
		UpdatedAt: database.Now(),
		Error: sql.NullString{
			String: failJob.Error,
			Valid:  failJob.Error != "",
		},
	})
	if err != nil {
		return nil, xerrors.Errorf("update provisioner job: %w", err)
	}
	switch jobType := failJob.Type.(type) {
	case *proto.FailedJob_WorkspaceBuild_:
		if jobType.WorkspaceBuild.State == nil {
			break
		}
		var input workspaceProvisionJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal workspace provision input: %w", err)
		}
		err = server.Database.UpdateWorkspaceBuildByID(ctx, database.UpdateWorkspaceBuildByIDParams{
			ID:               jobID,
			UpdatedAt:        database.Now(),
			ProvisionerState: jobType.WorkspaceBuild.State,
		})
		if err != nil {
			return nil, xerrors.Errorf("update workspace build state: %w", err)
		}
	case *proto.FailedJob_ProjectImport_:
	}
	return &proto.Empty{}, nil
}

// CompleteJob is triggered by a provision daemon to mark a provisioner job as completed.
func (server *provisionerdServer) CompleteJob(ctx context.Context, completed *proto.CompletedJob) (*proto.Empty, error) {
	jobID, err := uuid.Parse(completed.JobId)
	if err != nil {
		return nil, xerrors.Errorf("parse job id: %w", err)
	}
	job, err := server.Database.GetProvisionerJobByID(ctx, jobID)
	if err != nil {
		return nil, xerrors.Errorf("get job by id: %w", err)
	}
	if job.WorkerID.UUID.String() != server.ID.String() {
		return nil, xerrors.Errorf("you don't have permission to update this job")
	}

	switch jobType := completed.Type.(type) {
	case *proto.CompletedJob_ProjectImport_:
		for transition, resources := range map[database.WorkspaceTransition][]*sdkproto.Resource{
			database.WorkspaceTransitionStart: jobType.ProjectImport.StartResources,
			database.WorkspaceTransitionStop:  jobType.ProjectImport.StopResources,
		} {
			addresses, err := provisionersdk.ResourceAddresses(resources)
			if err != nil {
				return nil, xerrors.Errorf("compute resource addresses: %w", err)
			}
			for index, resource := range resources {
				server.Logger.Info(ctx, "inserting project import job resource",
					slog.F("job_id", job.ID.String()),
					slog.F("resource_name", resource.Name),
					slog.F("resource_type", resource.Type),
					slog.F("transition", transition))

				err = insertWorkspaceResource(ctx, server.Database, jobID, transition, resource, addresses[index])
				if err != nil {
					return nil, xerrors.Errorf("insert resource: %w", err)
				}
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
		server.Logger.Debug(ctx, "marked import job as completed", slog.F("job_id", jobID))
		if err != nil {
			return nil, xerrors.Errorf("complete job: %w", err)
		}
	case *proto.CompletedJob_WorkspaceBuild_:
		var input workspaceProvisionJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal job data: %w", err)
		}

		workspaceBuild, err := server.Database.GetWorkspaceBuildByID(ctx, input.WorkspaceBuildID)
		if err != nil {
			return nil, xerrors.Errorf("get workspace build: %w", err)
		}

		err = server.Database.InTx(func(db database.Store) error {
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
			err = db.UpdateWorkspaceBuildByID(ctx, database.UpdateWorkspaceBuildByIDParams{
				ID:               workspaceBuild.ID,
				UpdatedAt:        database.Now(),
				ProvisionerState: jobType.WorkspaceBuild.State,
			})
			if err != nil {
				return xerrors.Errorf("update workspace build: %w", err)
			}
			addresses, err := provisionersdk.ResourceAddresses(jobType.WorkspaceBuild.Resources)
			if err != nil {
				return xerrors.Errorf("compute resource addresses: %w", err)
			}
			// This could be a bulk insert to improve performance.
			for index, protoResource := range jobType.WorkspaceBuild.Resources {
				err = insertWorkspaceResource(ctx, db, job.ID, workspaceBuild.Transition, protoResource, addresses[index])
				if err != nil {
					return xerrors.Errorf("insert provisioner job: %w", err)
				}
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
		})
		if err != nil {
			return nil, xerrors.Errorf("complete job: %w", err)
		}
	default:
		return nil, xerrors.Errorf("unknown job type %q; ensure coderd and provisionerd versions match",
			reflect.TypeOf(completed.Type).String())
	}

	return &proto.Empty{}, nil
}

func insertWorkspaceResource(ctx context.Context, db database.Store, jobID uuid.UUID, transition database.WorkspaceTransition, protoResource *sdkproto.Resource, address string) error {
	resource, err := db.InsertWorkspaceResource(ctx, database.InsertWorkspaceResourceParams{
		ID:         uuid.New(),
		CreatedAt:  database.Now(),
		JobID:      jobID,
		Transition: transition,
		Address:    address,
		Type:       protoResource.Type,
		Name:       protoResource.Name,
		AgentID: uuid.NullUUID{
			UUID:  uuid.New(),
			Valid: protoResource.Agent != nil,
		},
	})
	if err != nil {
		return xerrors.Errorf("insert provisioner job resource %q: %w", protoResource.Name, err)
	}
	if resource.AgentID.Valid {
		var instanceID sql.NullString
		if protoResource.Agent.GetGoogleInstanceIdentity() != nil {
			instanceID = sql.NullString{
				String: protoResource.Agent.GetGoogleInstanceIdentity().InstanceId,
				Valid:  true,
			}
		}
		var env pqtype.NullRawMessage
		if protoResource.Agent.Env != nil {
			data, err := json.Marshal(protoResource.Agent.Env)
			if err != nil {
				return xerrors.Errorf("marshal env: %w", err)
			}
			env = pqtype.NullRawMessage{
				RawMessage: data,
				Valid:      true,
			}
		}
		authToken := uuid.New()
		if protoResource.Agent.GetToken() != "" {
			authToken, err = uuid.Parse(protoResource.Agent.GetToken())
			if err != nil {
				return xerrors.Errorf("invalid auth token format; must be uuid: %w", err)
			}
		}

		_, err := db.InsertWorkspaceAgent(ctx, database.InsertWorkspaceAgentParams{
			ID:                   resource.AgentID.UUID,
			CreatedAt:            database.Now(),
			UpdatedAt:            database.Now(),
			ResourceID:           resource.ID,
			AuthToken:            authToken,
			AuthInstanceID:       instanceID,
			EnvironmentVariables: env,
			StartupScript: sql.NullString{
				String: protoResource.Agent.StartupScript,
				Valid:  protoResource.Agent.StartupScript != "",
			},
		})
		if err != nil {
			return xerrors.Errorf("insert agent: %w", err)
		}
	}
	return nil
}

func convertValidationTypeSystem(typeSystem sdkproto.ParameterSchema_TypeSystem) (database.ParameterTypeSystem, error) {
	switch typeSystem {
	case sdkproto.ParameterSchema_None:
		return database.ParameterTypeSystemNone, nil
	case sdkproto.ParameterSchema_HCL:
		return database.ParameterTypeSystemHCL, nil
	default:
		return database.ParameterTypeSystem(""), xerrors.Errorf("unknown type system: %d", typeSystem)
	}
}

func convertParameterSourceScheme(sourceScheme sdkproto.ParameterSource_Scheme) (database.ParameterSourceScheme, error) {
	switch sourceScheme {
	case sdkproto.ParameterSource_DATA:
		return database.ParameterSourceSchemeData, nil
	default:
		return database.ParameterSourceScheme(""), xerrors.Errorf("unknown parameter source scheme: %d", sourceScheme)
	}
}

func convertParameterDestinationScheme(destinationScheme sdkproto.ParameterDestination_Scheme) (database.ParameterDestinationScheme, error) {
	switch destinationScheme {
	case sdkproto.ParameterDestination_ENVIRONMENT_VARIABLE:
		return database.ParameterDestinationSchemeEnvironmentVariable, nil
	case sdkproto.ParameterDestination_PROVISIONER_VARIABLE:
		return database.ParameterDestinationSchemeProvisionerVariable, nil
	default:
		return database.ParameterDestinationScheme(""), xerrors.Errorf("unknown parameter destination scheme: %d", destinationScheme)
	}
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

func convertComputedParameterValue(param parameter.ComputedValue) (*sdkproto.ParameterValue, error) {
	var scheme sdkproto.ParameterDestination_Scheme
	switch param.DestinationScheme {
	case database.ParameterDestinationSchemeEnvironmentVariable:
		scheme = sdkproto.ParameterDestination_ENVIRONMENT_VARIABLE
	case database.ParameterDestinationSchemeProvisionerVariable:
		scheme = sdkproto.ParameterDestination_PROVISIONER_VARIABLE
	default:
		return nil, xerrors.Errorf("unrecognized destination scheme: %q", param.DestinationScheme)
	}

	return &sdkproto.ParameterValue{
		DestinationScheme: scheme,
		Name:              param.Name,
		Value:             param.SourceValue,
	}, nil
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
