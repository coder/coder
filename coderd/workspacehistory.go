package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
	"github.com/coder/coder/provisionerd/proto"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
)

// WorkspaceHistory is an at-point representation of a workspace state.
// Iterate on before/after to determine a chronological history.
type WorkspaceHistory struct {
	ID               uuid.UUID                    `json:"id"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
	WorkspaceID      uuid.UUID                    `json:"workspace_id"`
	ProjectVersionID uuid.UUID                    `json:"project_version_id"`
	BeforeID         uuid.UUID                    `json:"before_id"`
	AfterID          uuid.UUID                    `json:"after_id"`
	Name             string                       `json:"name"`
	Transition       database.WorkspaceTransition `json:"transition"`
	Initiator        string                       `json:"initiator"`
	ProvisionJobID   uuid.UUID                    `json:"provision_job_id"`
}

// CreateWorkspaceHistoryRequest provides options to update the latest workspace history.
type CreateWorkspaceHistoryRequest struct {
	ProjectVersionID uuid.UUID                    `json:"project_version_id" validate:"required"`
	Transition       database.WorkspaceTransition `json:"transition" validate:"oneof=create start stop delete,required"`
}

type WorkspaceAgent struct {
	ID                  uuid.UUID       `json:"id"`
	WorkspaceHistoryID  uuid.UUID       `json:"workspace_history_id"`
	WorkspaceResourceID uuid.UUID       `json:"workspace_resource_id"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	InstanceMetadata    json.RawMessage `json:"instance_metadata"`
	ResourceMetadata    json.RawMessage `json:"resource_metadata"`
}

// WorkspaceResource represents a resource for workspace history.
type WorkspaceResource struct {
	ID                 uuid.UUID       `json:"id"`
	CreatedAt          time.Time       `json:"created_at"`
	WorkspaceHistoryID uuid.UUID       `json:"workspace_history_id"`
	Type               string          `json:"type"`
	Name               string          `json:"name"`
	Agent              *WorkspaceAgent `json:"agent"`
}

func (api *api) postWorkspaceHistoryByUser(rw http.ResponseWriter, r *http.Request) {
	var createBuild CreateWorkspaceHistoryRequest
	if !httpapi.Read(rw, r, &createBuild) {
		return
	}
	user := httpmw.UserParam(r)
	workspace := httpmw.WorkspaceParam(r)
	projectVersion, err := api.Database.GetProjectVersionByID(r.Context(), createBuild.ProjectVersionID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "project version not found",
			Errors: []httpapi.Error{{
				Field: "project_version_id",
				Code:  "exists",
			}},
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project version: %s", err),
		})
		return
	}
	projectVersionJob, err := api.Database.GetProvisionerJobByID(r.Context(), projectVersion.ImportJobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	projectVersionJobStatus := convertProvisionerJob(projectVersionJob).Status
	switch projectVersionJobStatus {
	case ProvisionerJobStatusPending, ProvisionerJobStatusRunning:
		httpapi.Write(rw, http.StatusNotAcceptable, httpapi.Response{
			Message: fmt.Sprintf("The provided project version is %s. Wait for it to complete importing!", projectVersionJobStatus),
		})
		return
	case ProvisionerJobStatusFailed:
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: fmt.Sprintf("The provided project version %q has failed to import. You cannot create workspaces using it!", projectVersion.Name),
		})
		return
	case ProvisionerJobStatusCancelled:
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "The provided project version was canceled during import. You cannot create workspaces using it!",
		})
		return
	}

	project, err := api.Database.GetProjectByID(r.Context(), projectVersion.ProjectID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project: %s", err),
		})
		return
	}

	// Store prior history ID if it exists to update it after we create new!
	priorHistoryID := uuid.NullUUID{}
	priorHistory, err := api.Database.GetWorkspaceHistoryByWorkspaceIDWithoutAfter(r.Context(), workspace.ID)
	if err == nil {
		priorJob, err := api.Database.GetProvisionerJobByID(r.Context(), priorHistory.ProvisionJobID)
		if err == nil && !convertProvisionerJob(priorJob).Status.Completed() {
			httpapi.Write(rw, http.StatusConflict, httpapi.Response{
				Message: "a workspace build is already active",
			})
			return
		}

		priorHistoryID = uuid.NullUUID{
			UUID:  priorHistory.ID,
			Valid: true,
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get prior workspace history: %s", err),
		})
		return
	}

	parameterSchemas, err := api.Database.GetParameterSchemasByJobID(r.Context(), projectVersion.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get parameter schemas: %s", err),
		})
		return
	}

	var workspaceHistory database.WorkspaceHistory
	// This must happen in a transaction to ensure history can be inserted, and
	// the prior history can update it's "after" column to point at the new.
	err = api.Database.InTx(func(db database.Store) error {
		provisionerJobID := uuid.New()
		err = parameter.Inject(r.Context(), db, parameter.InjectOptions{
			ParameterSchemas: parameterSchemas,
			ProvisionJobID:   provisionerJobID,
			Username:         user.Username,
			Transition:       createBuild.Transition,
		})
		if err != nil {
			return xerrors.Errorf("inject parameters: %w", err)
		}

		workspaceHistory, err = db.InsertWorkspaceHistory(r.Context(), database.InsertWorkspaceHistoryParams{
			ID:               uuid.New(),
			CreatedAt:        database.Now(),
			UpdatedAt:        database.Now(),
			WorkspaceID:      workspace.ID,
			ProjectVersionID: projectVersion.ID,
			BeforeID:         priorHistoryID,
			Name:             namesgenerator.GetRandomName(1),
			Initiator:        user.ID,
			Transition:       createBuild.Transition,
			ProvisionJobID:   provisionerJobID,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace history: %w", err)
		}

		input, err := json.Marshal(workspaceProvisionJob{
			WorkspaceHistoryID: workspaceHistory.ID,
		})
		if err != nil {
			return xerrors.Errorf("marshal provision job: %w", err)
		}

		_, err = db.InsertProvisionerJob(r.Context(), database.InsertProvisionerJobParams{
			ID:             provisionerJobID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			InitiatorID:    user.ID,
			OrganizationID: project.OrganizationID,
			Provisioner:    project.Provisioner,
			Type:           database.ProvisionerJobTypeWorkspaceProvision,
			StorageMethod:  projectVersionJob.StorageMethod,
			StorageSource:  projectVersionJob.StorageSource,
			Input:          input,
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}

		if priorHistoryID.Valid {
			// Update the prior history entries "after" column.
			err = db.UpdateWorkspaceHistoryByID(r.Context(), database.UpdateWorkspaceHistoryByIDParams{
				ID:               priorHistory.ID,
				ProvisionerState: priorHistory.ProvisionerState,
				UpdatedAt:        database.Now(),
				AfterID: uuid.NullUUID{
					UUID:  workspaceHistory.ID,
					Valid: true,
				},
			})
			if err != nil {
				return xerrors.Errorf("update prior workspace history: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertWorkspaceHistory(workspaceHistory))
}

// Returns all workspace history. This is not sorted. Use before/after to chronologically sort.
func (api *api) workspaceHistoryByUser(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)

	history, err := api.Database.GetWorkspaceHistoryByWorkspaceID(r.Context(), workspace.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		history = []database.WorkspaceHistory{}
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace history: %s", err),
		})
		return
	}

	apiHistory := make([]WorkspaceHistory, 0, len(history))
	for _, history := range history {
		apiHistory = append(apiHistory, convertWorkspaceHistory(history))
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiHistory)
}

func (api *api) workspaceHistoryResources(rw http.ResponseWriter, r *http.Request) {
	workspaceHistory := httpmw.WorkspaceHistoryParam(r)
	provisionerJob, err := api.Database.GetProvisionerJobByID(r.Context(), workspaceHistory.ProvisionJobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	if !convertProvisionerJob(provisionerJob).Status.Completed() {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "Job hasn't completed!",
		})
		return
	}

	resources, err := api.Database.GetWorkspaceResourcesByHistoryID(r.Context(), workspaceHistory.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace resources: %s", err),
		})
		return
	}
	resourceIDs := make([]uuid.UUID, 0, len(resources))
	for _, resource := range resources {
		resourceIDs = append(resourceIDs, resource.ID)
	}
	agents, err := api.Database.GetWorkspaceAgentsByResourceIDs(r.Context(), resourceIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace agents: %s", err),
		})
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertWorkspaceResources(resources, agents))
}

func (*api) workspaceHistoryByName(rw http.ResponseWriter, r *http.Request) {
	workspaceHistory := httpmw.WorkspaceHistoryParam(r)
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertWorkspaceHistory(workspaceHistory))
}

func convertWorkspaceResources(workspaceResources []database.WorkspaceResource, workspaceAgents []database.WorkspaceAgent) []WorkspaceResource {
	apiResources := make([]WorkspaceResource, 0)
	for _, resource := range workspaceResources {
		apiResource := WorkspaceResource{
			ID:                 resource.ID,
			CreatedAt:          resource.CreatedAt,
			WorkspaceHistoryID: resource.WorkspaceHistoryID,
			Type:               resource.Type,
			Name:               resource.Name,
		}
		if resource.WorkspaceAgentID.Valid {
			for _, agent := range workspaceAgents {
				if agent.ID.String() != resource.WorkspaceAgentID.UUID.String() {
					continue
				}
				apiResource.Agent = &WorkspaceAgent{
					ID:                  agent.ID,
					WorkspaceHistoryID:  agent.WorkspaceHistoryID,
					WorkspaceResourceID: agent.WorkspaceResourceID,
					CreatedAt:           agent.CreatedAt,
					UpdatedAt:           agent.UpdatedAt.Time,
					InstanceMetadata:    agent.InstanceMetadata,
					ResourceMetadata:    agent.ResourceMetadata,
				}
				break
			}
		}
		apiResources = append(apiResources, apiResource)
	}
	return apiResources
}

// Returns the job data for an acquired workspace provision job!
func fillAcquiredWorkspaceProvisionJob(ctx context.Context, db database.Store, user database.User, job database.ProvisionerJob) (*proto.AcquiredJob_WorkspaceProvision_, error) {
	var input workspaceProvisionJob
	err := json.Unmarshal(job.Input, &input)
	if err != nil {
		return nil, xerrors.Errorf("unmarshal job input %q: %s", job.Input, err)
	}
	workspaceHistory, err := db.GetWorkspaceHistoryByID(ctx, input.WorkspaceHistoryID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace history: %s", err)
	}
	workspace, err := db.GetWorkspaceByID(ctx, workspaceHistory.WorkspaceID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace: %s", err)
	}
	projectVersion, err := db.GetProjectVersionByID(ctx, workspaceHistory.ProjectVersionID)
	if err != nil {
		return nil, xerrors.Errorf("get project version: %s", err)
	}
	project, err := db.GetProjectByID(ctx, projectVersion.ProjectID)
	if err != nil {
		return nil, xerrors.Errorf("get project: %s", err)
	}
	parameterSchemas, err := db.GetParameterSchemasByJobID(ctx, projectVersion.ImportJobID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get parameter schemas: %s", err)
	}

	// Compute parameters for the workspace to consume.
	parameters, err := parameter.Compute(ctx, db, parameter.ComputeOptions{
		Schemas:        parameterSchemas,
		ProvisionJobID: job.ID,
		OrganizationID: job.OrganizationID,
		ProjectID: uuid.NullUUID{
			UUID:  project.ID,
			Valid: true,
		},
		UserID: user.ID,
		WorkspaceID: uuid.NullUUID{
			UUID:  workspace.ID,
			Valid: true,
		},
	})
	if err != nil {
		return nil, xerrors.Errorf("compute parameters: %s", err)
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

	return &proto.AcquiredJob_WorkspaceProvision_{
		WorkspaceProvision: &proto.AcquiredJob_WorkspaceProvision{
			WorkspaceHistoryId: workspaceHistory.ID.String(),
			WorkspaceName:      workspace.Name,
			State:              workspaceHistory.ProvisionerState,
			ParameterValues:    protoParameters,
		},
	}, nil
}

func completeWorkspaceProvisionJob(ctx context.Context, db database.Store, job database.ProvisionerJob, completed *proto.CompletedJob_WorkspaceProvision_) error {
	var input workspaceProvisionJob
	err := json.Unmarshal(job.Input, &input)
	if err != nil {
		return xerrors.Errorf("unmarshal job data: %w", err)
	}

	workspaceHistory, err := db.GetWorkspaceHistoryByID(ctx, input.WorkspaceHistoryID)
	if err != nil {
		return xerrors.Errorf("get workspace history: %w", err)
	}

	err = db.InTx(func(db database.Store) error {
		err = db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID:        job.ID,
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
			ProvisionerState: completed.WorkspaceProvision.State,
		})
		if err != nil {
			return xerrors.Errorf("update workspace history: %w", err)
		}

		// This could be a bulk insert to improve performance.
		for _, protoResource := range completed.WorkspaceProvision.Resources {
			var (
				workspaceResourceID = uuid.New()
				hasAgent            = protoResource.InstanceId != ""
				agentToken          string
			)
			if !hasAgent {
				// The agent token is stored as a parameter value on the workspace provision.
				parameterValues, err := db.GetParameterValuesByScope(ctx, database.GetParameterValuesByScopeParams{
					Scope:   database.ParameterScopeProvisionerJob,
					ScopeID: job.ID.String(),
				})
				if errors.Is(err, sql.ErrNoRows) {
					err = nil
				}
				if err != nil {
					return xerrors.Errorf("get parameter values: %w", err)
				}

				agentToken, hasAgent = parameter.FindAgentToken(parameterValues, protoResource.Type, protoResource.Name)
			}

			var agentID uuid.NullUUID
			if hasAgent {
				agentID = uuid.NullUUID{
					UUID:  uuid.New(),
					Valid: true,
				}
				if agentToken == "" {
					agentToken = uuid.NewString()
				}
				_, err := db.InsertWorkspaceAgent(ctx, database.InsertWorkspaceAgentParams{
					ID:                  agentID.UUID,
					WorkspaceHistoryID:  workspaceHistory.ID,
					WorkspaceResourceID: workspaceResourceID,
					InstanceID: sql.NullString{
						String: protoResource.InstanceId,
						Valid:  true,
					},
					Token:     agentToken,
					CreatedAt: database.Now(),
				})
				if err != nil {
					return xerrors.Errorf("insert workspace agent: %w", err)
				}
			}

			_, err = db.InsertWorkspaceResource(ctx, database.InsertWorkspaceResourceParams{
				ID:                 workspaceResourceID,
				CreatedAt:          database.Now(),
				WorkspaceHistoryID: input.WorkspaceHistoryID,
				Type:               protoResource.Type,
				Name:               protoResource.Name,
				WorkspaceAgentID:   agentID,
			})
			if err != nil {
				return xerrors.Errorf("insert workspace resource %q: %w", protoResource.Name, err)
			}
		}
		return nil
	})
	if err != nil {
		return xerrors.Errorf("complete job: %w", err)
	}
	return nil
}

// Converts the internal history representation to a public external-facing model.
func convertWorkspaceHistory(workspaceHistory database.WorkspaceHistory) WorkspaceHistory {
	//nolint:unconvert
	return WorkspaceHistory(WorkspaceHistory{
		ID:               workspaceHistory.ID,
		CreatedAt:        workspaceHistory.CreatedAt,
		UpdatedAt:        workspaceHistory.UpdatedAt,
		WorkspaceID:      workspaceHistory.WorkspaceID,
		ProjectVersionID: workspaceHistory.ProjectVersionID,
		BeforeID:         workspaceHistory.BeforeID.UUID,
		AfterID:          workspaceHistory.AfterID.UUID,
		Name:             workspaceHistory.Name,
		Transition:       workspaceHistory.Transition,
		Initiator:        workspaceHistory.Initiator,
		ProvisionJobID:   workspaceHistory.ProvisionJobID,
	})
}
