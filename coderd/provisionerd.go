package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/xerrors"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"github.com/google/uuid"
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
	// defer conn.Close(websocket.StatusInternalError, "request closed")

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

	mux := drpcmux.New()
	err = proto.DRPCRegisterProvisionerDaemon(mux, &provisionerdServer{
		ID:       daemon.ID,
		Database: p.Database,
	})
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, fmt.Sprintf("drpc register provisioner daemon: %s", err))
		return
	}
	srv := drpcserver.New(mux)
	fmt.Printf("WE AT LEAST GETTING HERE!\n")
	nc := websocket.NetConn(context.Background(), conn, websocket.MessageBinary)
	// go func() {
	err = srv.ServeOne(context.Background(), nc)
	if err != nil {
		fmt.Printf("WE ARE FAILING TO SERV! %s\n", err)
		_ = conn.Close(websocket.StatusInternalError, fmt.Sprintf("serve: %s", err))
		return
	}
	// }()
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
		return xerrors.Errorf("request job was invalidated: %s", err)
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

		workspace, err := s.Database.GetWorkspaceByID(ctx, workspaceHistory.ID)
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
		// fmt.Printf("WE KISENING FOR JOB\n")
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

func (s *provisionerdServer) CompleteJob(ctx context.Context, completed *proto.CompletedJob) (*proto.Empty, error) {
	return nil, nil
}
