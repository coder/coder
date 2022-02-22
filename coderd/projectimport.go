package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/render"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
	"github.com/coder/coder/provisionerd/proto"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
)

// ParameterSchema represents a parameter parsed from project version source.
type ParameterSchema database.ParameterSchema

// ComputedParameterValue represents a computed parameter value.
type ComputedParameterValue parameter.ComputedValue

// ProjectImportJobResource is a resource created by a project import job.
type ProjectImportJobResource database.ProjectImportJobResource

// CreateProjectImportJobRequest provides options to create a project import job.
type CreateProjectImportJobRequest struct {
	StorageMethod database.ProvisionerStorageMethod `json:"storage_method" validate:"oneof=file,required"`
	StorageSource string                            `json:"storage_source" validate:"required"`
	Provisioner   database.ProvisionerType          `json:"provisioner" validate:"oneof=terraform echo,required"`
	// ParameterValues allows for additional parameters to be provided
	// during the dry-run provision stage.
	ParameterValues []CreateParameterValueRequest `json:"parameter_values"`
}

// Create a new project import job!
func (api *api) postProjectImportByOrganization(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	organization := httpmw.OrganizationParam(r)
	var req CreateProjectImportJobRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}
	file, err := api.Database.GetFileByHash(r.Context(), req.StorageSource)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "file not found",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get file: %s", err),
		})
		return
	}

	jobID := uuid.New()
	for _, parameterValue := range req.ParameterValues {
		_, err = api.Database.InsertParameterValue(r.Context(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterValue.Name,
			CreatedAt:         database.Now(),
			UpdatedAt:         database.Now(),
			Scope:             database.ParameterScopeProvisionerJob,
			ScopeID:           jobID.String(),
			SourceScheme:      parameterValue.SourceScheme,
			SourceValue:       parameterValue.SourceValue,
			DestinationScheme: parameterValue.DestinationScheme,
		})
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("insert parameter value: %s", err),
			})
			return
		}
	}

	job, err := api.Database.InsertProvisionerJob(r.Context(), database.InsertProvisionerJobParams{
		ID:             jobID,
		CreatedAt:      database.Now(),
		UpdatedAt:      database.Now(),
		OrganizationID: organization.ID,
		InitiatorID:    apiKey.UserID,
		Provisioner:    req.Provisioner,
		StorageMethod:  database.ProvisionerStorageMethodFile,
		StorageSource:  file.Hash,
		Type:           database.ProvisionerJobTypeProjectVersionImport,
		Input:          []byte{'{', '}'},
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert provisioner job: %s", err),
		})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertProvisionerJob(job))
}

// Returns imported parameter schemas from a completed job!
func (api *api) projectImportJobSchemasByID(rw http.ResponseWriter, r *http.Request) {
	job := httpmw.ProvisionerJobParam(r)
	if !convertProvisionerJob(job).Status.Completed() {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "Job hasn't completed!",
		})
		return
	}

	schemas, err := api.Database.GetParameterSchemasByJobID(r.Context(), job.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("list parameter schemas: %s", err),
		})
		return
	}
	if schemas == nil {
		schemas = []database.ParameterSchema{}
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, schemas)
}

// Returns computed parameters for an import job by ID.
func (api *api) projectImportJobParametersByID(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	job := httpmw.ProvisionerJobParam(r)
	if !convertProvisionerJob(job).Status.Completed() {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "Job hasn't completed!",
		})
		return
	}
	parameterSchemas, err := api.Database.GetParameterSchemasByJobID(r.Context(), job.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get parameter schemas: %s", err),
		})
		return
	}
	values, err := parameter.Compute(r.Context(), api.Database, parameter.ComputeOptions{
		Schemas: parameterSchemas,

		ProvisionJobID: job.ID,
		OrganizationID: job.OrganizationID,
		UserID:         apiKey.UserID,

		// We *never* want to send the client secret parameter values.
		HideRedisplayValues: true,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("compute values: %s", err),
		})
		return
	}
	if values == nil {
		values = []parameter.ComputedValue{}
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, values)
}

// Returns resources for an import job by ID.
func (api *api) projectImportJobResourcesByID(rw http.ResponseWriter, r *http.Request) {
	job := httpmw.ProvisionerJobParam(r)
	if !convertProvisionerJob(job).Status.Completed() {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "Job hasn't completed!",
		})
		return
	}
	resources, err := api.Database.GetProjectImportJobResourcesByJobID(r.Context(), job.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project import job resources: %s", err),
		})
		return
	}
	if resources == nil {
		resources = []database.ProjectImportJobResource{}
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, resources)
}

func completeProjectImportJob(ctx context.Context, db database.Store, job database.ProvisionerJob, completed *proto.CompletedJob_ProjectImport_) error {
	for transition, resources := range map[database.WorkspaceTransition][]*sdkproto.PlannedResource{
		database.WorkspaceTransitionStart: completed.ProjectImport.StartResources,
		database.WorkspaceTransitionStop:  completed.ProjectImport.StopResources,
	} {
		parameterSchemas, err := db.GetParameterSchemasByJobID(ctx, job.ID)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		if err != nil {
			return xerrors.Errorf("get parameter schemas: %w", err)
		}

		for _, resource := range resources {
			// Resources can report whether they'll have an agent or not.
			//
			// If they don't, we check to see if a parameter was specified
			// for the token. This marks the resource as having an agent.
			if !resource.Agent {
				resource.Agent = parameter.HasAgentToken(parameterSchemas, resource.Type, resource.Name)
			}

			_, err = db.InsertProjectImportJobResource(ctx, database.InsertProjectImportJobResourceParams{
				ID:         uuid.New(),
				CreatedAt:  database.Now(),
				JobID:      job.ID,
				Transition: transition,
				Agent:      resource.Agent,
				Type:       resource.Type,
				Name:       resource.Name,
			})
			if err != nil {
				return xerrors.Errorf("insert resource: %w", err)
			}
		}
	}

	err := db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
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
	return nil
}
