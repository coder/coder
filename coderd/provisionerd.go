package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"golang.org/x/xerrors"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/moby/moby/pkg/namesgenerator"

	"github.com/coder/coder/coderd/projectparameter"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/provisionerd/proto"
	sdkproto "github.com/coder/coder/provisionersdk/proto"

	"nhooyr.io/websocket"
)

type provisionerd struct {
	Database database.Store
}

func (p *provisionerd) listen(rw http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("accept websocket: %s", err),
		})
		return
	}

	daemon, err := p.Database.InsertProvisionerDaemon(r.Context(), database.InsertProvisionerDaemonParams{
		ID:           uuid.New(),
		CreatedAt:    database.Now(),
		Name:         namesgenerator.GetRandomName(1),
		Provisioners: []database.ProvisionerType{database.ProvisionerTypeCdrBasic, database.ProvisionerTypeTerraform},
	})
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, fmt.Sprintf("insert provisioner daemon:% s", err))
		return
	}

	session, err := yamux.Server(websocket.NetConn(r.Context(), conn, websocket.MessageBinary), nil)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, fmt.Sprintf("multiplex server: %s", err))
		return
	}
	mux := drpcmux.New()
	err = proto.DRPCRegisterProvisionerDaemon(mux, &provisionerdServer{
		ID:       daemon.ID,
		Database: p.Database,
	})
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, fmt.Sprintf("drpc register provisioner daemon: %s", err))
		return
	}
	server := drpcserver.New(mux)
	err = server.Serve(r.Context(), session)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, fmt.Sprintf("serve: %s", err))
	}
}

// The input for a "workspace_provision" job.
type workspaceProvisionJob struct {
	WorkspaceHistoryID uuid.UUID `json:"workspace_id"`
}

// The input for a "project_import" job.
type projectImportJob struct {
	ProjectHistoryID uuid.UUID `json:"project_history_id"`
}

// An implementation of the provisionerd protobuf server definition.
type provisionerdServer struct {
	ID       uuid.UUID
	Database database.Store
}

func (s *provisionerdServer) AcquireJob(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
	// This locks the job. No other provisioners can acquire this job.
	job, err := s.Database.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
		StartedAt: sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		},
		WorkerID: uuid.NullUUID{
			UUID:  s.ID,
			Valid: true,
		},
		Types: []database.ProvisionerType{database.ProvisionerTypeTerraform},
	})
	if errors.Is(err, sql.ErrNoRows) {
		// If no jobs are available, an empty struct is sent back.
		return &proto.AcquiredJob{}, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("acquire job: %w", err)
	}
	failJob := func(errorMessage string) error {
		err = s.Database.UpdateProvisionerJobByID(ctx, database.UpdateProvisionerJobByIDParams{
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

	project, err := s.Database.GetProjectByID(ctx, job.ProjectID)
	if err != nil {
		return nil, failJob(fmt.Sprintf("get project: %s", err))
	}

	organization, err := s.Database.GetOrganizationByID(ctx, project.OrganizationID)
	if err != nil {
		return nil, failJob(fmt.Sprintf("get organization: %s", err))
	}

	user, err := s.Database.GetUserByID(ctx, job.InitiatorID)
	if err != nil {
		return nil, failJob(fmt.Sprintf("get user: %s", err))
	}

	acquiredJob := &proto.AcquiredJob{
		JobId:            job.ID.String(),
		CreatedAt:        job.CreatedAt.UnixMilli(),
		Provisioner:      string(job.Provisioner),
		OrganizationName: organization.Name,
		ProjectName:      project.Name,
		UserName:         user.Username,
	}
	var projectHistory database.ProjectHistory
	switch job.Type {
	case database.ProvisionerJobTypeWorkspaceProvision:
		var input workspaceProvisionJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, failJob(fmt.Sprintf("unmarshal job input %q: %s", job.Input, err))
		}
		workspaceHistory, err := s.Database.GetWorkspaceHistoryByID(ctx, input.WorkspaceHistoryID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get workspace history: %s", err))
		}

		workspace, err := s.Database.GetWorkspaceByID(ctx, workspaceHistory.WorkspaceID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get workspace: %s", err))
		}

		projectHistory, err = s.Database.GetProjectHistoryByID(ctx, workspaceHistory.ProjectHistoryID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get project history: %s", err))
		}

		parameters, err := projectparameter.Compute(ctx, s.Database, projectparameter.Scope{
			OrganizationID:     organization.ID,
			ProjectID:          project.ID,
			ProjectHistoryID:   projectHistory.ID,
			UserID:             user.ID,
			WorkspaceID:        workspace.ID,
			WorkspaceHistoryID: workspaceHistory.ID,
		})
		if err != nil {
			return nil, failJob(fmt.Sprintf("compute parameters: %s", err))
		}
		protoParameters := make([]*sdkproto.ParameterValue, 0, len(parameters))
		for _, parameter := range parameters {
			protoParameters = append(protoParameters, parameter.Proto)
		}

		provisionerState := []byte{}
		if workspaceHistory.BeforeID.Valid {
			beforeHistory, err := s.Database.GetWorkspaceHistoryByID(ctx, workspaceHistory.BeforeID.UUID)
			if err != nil {
				return nil, failJob(fmt.Sprintf("get workspace history: %s", err))
			}
			provisionerState = beforeHistory.ProvisionerState
		}

		acquiredJob.Type = &proto.AcquiredJob_WorkspaceProvision_{
			WorkspaceProvision: &proto.AcquiredJob_WorkspaceProvision{
				WorkspaceHistoryId: workspaceHistory.ID.String(),
				WorkspaceName:      workspace.Name,
				State:              provisionerState,
				ParameterValues:    protoParameters,
			},
		}
	case database.ProvisionerJobTypeProjectImport:
		var input projectImportJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, failJob(fmt.Sprintf("unmarshal job input %q: %s", job.Input, err))
		}
		projectHistory, err = s.Database.GetProjectHistoryByID(ctx, input.ProjectHistoryID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get project history: %s", err))
		}
	}
	switch projectHistory.StorageMethod {
	case database.ProjectStorageMethodInlineArchive:
		acquiredJob.ProjectSourceArchive = projectHistory.StorageSource
	default:
		return nil, failJob(fmt.Sprintf("unsupported storage source: %q", projectHistory.StorageMethod))
	}

	return acquiredJob, err
}

func (s *provisionerdServer) UpdateJob(stream proto.DRPCProvisionerDaemon_UpdateJobStream) error {
	for {
		update, err := stream.Recv()
		if err != nil {
			return err
		}
		parsedID, err := uuid.Parse(update.JobId)
		if err != nil {
			return xerrors.Errorf("parse job id: %w", err)
		}
		err = s.Database.UpdateProvisionerJobByID(context.Background(), database.UpdateProvisionerJobByIDParams{
			ID:        parsedID,
			UpdatedAt: database.Now(),
		})
		if err != nil {
			return xerrors.Errorf("update job: %w", err)
		}
	}
}

func (s *provisionerdServer) CancelJob(ctx context.Context, cancelJob *proto.CancelledJob) (*proto.Empty, error) {
	jobID, err := uuid.Parse(cancelJob.JobId)
	if err != nil {
		return nil, xerrors.Errorf("parse job id: %w", err)
	}
	err = s.Database.UpdateProvisionerJobByID(ctx, database.UpdateProvisionerJobByIDParams{
		ID: jobID,
		CancelledAt: sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		},
		UpdatedAt: database.Now(),
		Error: sql.NullString{
			String: cancelJob.Error,
			Valid:  cancelJob.Error != "",
		},
	})
	if err != nil {
		return nil, xerrors.Errorf("update provisioner job: %w", err)
	}
	return &proto.Empty{}, nil
}

// CompleteJob is triggered by a provision daemon to mark a provisioner job as completed.
func (s *provisionerdServer) CompleteJob(ctx context.Context, completed *proto.CompletedJob) (*proto.Empty, error) {
	jobID, err := uuid.Parse(completed.JobId)
	if err != nil {
		return nil, xerrors.Errorf("parse job id: %w", err)
	}
	job, err := s.Database.GetProvisionerJobByID(ctx, jobID)
	if err != nil {
		return nil, xerrors.Errorf("get job by id: %w", err)
	}
	// TODO: Check if the worker ID matches!
	// If it doesn't, a provisioner daemon could be impersonating another job!

	switch jobType := completed.Type.(type) {
	case *proto.CompletedJob_ProjectImport_:
		var input projectImportJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal job data: %w", err)
		}

		// Validate that all parameters send from the provisioner daemon
		// follow the protocol.
		projectParameters := make([]database.InsertProjectParameterParams, 0, len(jobType.ProjectImport.ParameterSchemas))
		for _, protoParameter := range jobType.ProjectImport.ParameterSchemas {
			validationTypeSystem, err := convertValidationTypeSystem(protoParameter.ValidationTypeSystem)
			if err != nil {
				return nil, xerrors.Errorf("convert validation type system for %q: %w", protoParameter.Name, err)
			}

			projectParameter := database.InsertProjectParameterParams{
				ID:                   uuid.New(),
				CreatedAt:            database.Now(),
				ProjectHistoryID:     input.ProjectHistoryID,
				Name:                 protoParameter.Name,
				Description:          protoParameter.Description,
				RedisplayValue:       protoParameter.RedisplayValue,
				ValidationError:      protoParameter.ValidationError,
				ValidationCondition:  protoParameter.ValidationCondition,
				ValidationValueType:  protoParameter.ValidationValueType,
				ValidationTypeSystem: validationTypeSystem,

				AllowOverrideDestination: protoParameter.AllowOverrideDestination,
				AllowOverrideSource:      protoParameter.AllowOverrideSource,
			}

			// It's possible a parameter doesn't define a default source!
			if protoParameter.DefaultSource != nil {
				parameterSourceScheme, err := convertParameterSourceScheme(protoParameter.DefaultSource.Scheme)
				if err != nil {
					return nil, xerrors.Errorf("convert parameter source scheme: %w", err)
				}
				projectParameter.DefaultSourceScheme = parameterSourceScheme
				projectParameter.DefaultSourceValue = sql.NullString{
					String: protoParameter.DefaultSource.Value,
					Valid:  protoParameter.DefaultSource.Value != "",
				}
			}

			// It's possible a parameter doesn't define a default destination!
			if protoParameter.DefaultDestination != nil {
				parameterDestinationScheme, err := convertParameterDestinationScheme(protoParameter.DefaultDestination.Scheme)
				if err != nil {
					return nil, xerrors.Errorf("convert parameter destination scheme: %w", err)
				}
				projectParameter.DefaultDestinationScheme = parameterDestinationScheme
				projectParameter.DefaultDestinationValue = sql.NullString{
					String: protoParameter.DefaultDestination.Value,
					Valid:  protoParameter.DefaultDestination.Value != "",
				}
			}

			projectParameters = append(projectParameters, projectParameter)
		}

		// This must occur in a transaction in case of failure.
		err = s.Database.InTx(func(db database.Store) error {
			err = db.UpdateProvisionerJobByID(ctx, database.UpdateProvisionerJobByIDParams{
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
			for _, projectParameter := range projectParameters {
				_, err = db.InsertProjectParameter(ctx, projectParameter)
				if err != nil {
					return xerrors.Errorf("insert project parameter %q: %w", projectParameter.Name, err)
				}
			}
			return nil
		})
		if err != nil {
			return nil, xerrors.Errorf("complete job: %w", err)
		}
	case *proto.CompletedJob_WorkspaceProvision_:
		var input workspaceProvisionJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal job data: %w", err)
		}

		workspaceHistory, err := s.Database.GetWorkspaceHistoryByID(ctx, input.WorkspaceHistoryID)
		if err != nil {
			return nil, xerrors.Errorf("get workspace history: %w", err)
		}

		err = s.Database.InTx(func(db database.Store) error {
			err = db.UpdateProvisionerJobByID(ctx, database.UpdateProvisionerJobByIDParams{
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
			err = db.UpdateWorkspaceHistoryByID(ctx, database.UpdateWorkspaceHistoryByIDParams{
				ID:               workspaceHistory.ID,
				UpdatedAt:        database.Now(),
				ProvisionerState: jobType.WorkspaceProvision.State,
				CompletedAt: sql.NullTime{
					Time:  database.Now(),
					Valid: true,
				},
			})
			if err != nil {
				return xerrors.Errorf("update workspace history: %w", err)
			}
			for _, protoResource := range jobType.WorkspaceProvision.Resources {
				_, err = db.InsertWorkspaceResource(ctx, database.InsertWorkspaceResourceParams{
					ID:                 uuid.New(),
					CreatedAt:          database.Now(),
					WorkspaceHistoryID: input.WorkspaceHistoryID,
					Type:               protoResource.Type,
					Name:               protoResource.Name,
					// TODO: Generate this at the variable validation phase.
					// Set the value in `default_source`, and disallow overwrite.
					WorkspaceAgentToken: uuid.NewString(),
				})
				if err != nil {
					return xerrors.Errorf("insert workspace resource %q: %w", protoResource.Name, err)
				}
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

func convertValidationTypeSystem(typeSystem sdkproto.ParameterSchema_TypeSystem) (database.ParameterTypeSystem, error) {
	switch typeSystem {
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
