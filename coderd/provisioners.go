package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"github.com/coder/coder/database"
	"github.com/coder/coder/provisionerd/proto"
)

// The input for a "workspace_provision" job.
type workspaceProvisionJob struct {
	WorkspaceHistoryID uuid.UUID `json:"workspace_id"`
	ProvisionerState   []byte    `json:"provisioner_state"`
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
			return nil, failJob(fmt.Sprintf("unmarshal job input %q: %w", job.Input, err))
		}
		workspaceHistory, err := s.Database.GetWorkspaceHistoryByID(ctx, input.WorkspaceHistoryID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get workspace history: %s", err))
		}

		workspace, err := s.Database.GetWorkspaceByID(ctx, workspaceHistory.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, failJob(fmt.Sprintf("get workspace: %s", err))
		}

		parameterValueMap := map[string]sdkproto.ParameterValue{}
		insertParameters := func(params []database.ParameterValue) {
			for _, param := range params {
				parameterValueMap[param.Name] = param
			}
		}
		paramterValues, err := s.Database.GetParameterValuesByScope(ctx, database.GetParameterValuesByScopeParams{
			Scope:   database.ParameterScopeOrganization,
			ScopeID: organization.ID,
		})
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		if err != nil {
			return nil, failJob(fmt.Sprintf("get parameter values for organization: %w", err))
		}
		for _, parameterValue := range parameterValues {

		}

		acquiredJob.Type = &proto.AcquiredJob_WorkspaceProvision_{
			WorkspaceProvision: &proto.AcquiredJob_WorkspaceProvision{
				WorkspaceHistoryId: workspaceHistory.ID.String(),
				WorkspaceName:      workspace.Name,
				State:              input.ProvisionerState,
				ParameterValues:    nil,
			},
		}
	}

	return acquiredJob, err
}

func (s *provisionerdServer) UpdateJob(stream proto.DRPCProvisionerDaemon_UpdateJobStream) error {
	return nil
}

func (s *provisionerdServer) CompleteJob(ctx context.Context, completed *proto.CompletedJob) (*proto.Empty, error) {
	return nil, nil
}
