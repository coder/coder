package coderd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/coderd/provisionerdserver"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/examples"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
)

// @Summary Get template version by ID
// @ID get-template-version-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200 {object} codersdk.TemplateVersion
// @Router /templateversions/{templateversion} [get]
func (api *API) templateVersion(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var (
		templateVersion = httpmw.TemplateVersionParam(r)
		template        = httpmw.TemplateParam(r)
	)

	if !api.Authorize(r, rbac.ActionRead, templateVersion.RBACObject(template)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	job, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	user, err := api.Database.GetUserByID(ctx, templateVersion.CreatedBy)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error on fetching user.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertTemplateVersion(templateVersion, convertProvisionerJob(job), user))
}

// @Summary Cancel template version by ID
// @ID cancel-template-version-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200 {object} codersdk.Response
// @Router /templateversions/{templateversion}/cancel [patch]
func (api *API) patchCancelTemplateVersion(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var (
		templateVersion = httpmw.TemplateVersionParam(r)
		template        = httpmw.TemplateParam(r)
	)
	if !api.Authorize(r, rbac.ActionUpdate, templateVersion.RBACObject(template)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	job, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if job.CompletedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Job has already completed!",
		})
		return
	}
	if job.CanceledAt.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Job has already been marked as canceled!",
		})
		return
	}
	err = api.Database.UpdateProvisionerJobWithCancelByID(ctx, database.UpdateProvisionerJobWithCancelByIDParams{
		ID: job.ID,
		CanceledAt: sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		},
		CompletedAt: sql.NullTime{
			Time: database.Now(),
			// If the job is running, don't mark it completed!
			Valid: !job.WorkerID.Valid,
		},
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Job has been marked as canceled...",
	})
}

// @Summary Get schema by template version
// @ID get-schema-by-template-version
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200 {array} codersdk.ParameterSchema
// @Router /templateversions/{templateversion}/schema [get]
func (api *API) templateVersionSchema(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var (
		templateVersion = httpmw.TemplateVersionParam(r)
		template        = httpmw.TemplateParam(r)
	)

	if !api.Authorize(r, rbac.ActionRead, templateVersion.RBACObject(template)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	job, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Template version job hasn't completed!",
		})
		return
	}
	schemas, err := api.Database.GetParameterSchemasByJobID(ctx, job.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error listing parameter schemas.",
			Detail:  err.Error(),
		})
		return
	}
	apiSchemas := make([]codersdk.ParameterSchema, 0)
	for _, schema := range schemas {
		apiSchema, err := convertParameterSchema(schema)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: fmt.Sprintf("Internal error converting schema %s.", schema.Name),
				Detail:  err.Error(),
			})
			return
		}
		apiSchemas = append(apiSchemas, apiSchema)
	}
	httpapi.Write(ctx, rw, http.StatusOK, apiSchemas)
}

// @Summary Get rich parameters by template version
// @ID get-rich-parameters-by-template-version
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200 {array} codersdk.TemplateVersionParameter
// @Router /templateversions/{templateversion}/rich-parameters [get]
func (api *API) templateVersionRichParameters(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	templateVersion := httpmw.TemplateVersionParam(r)
	template := httpmw.TemplateParam(r)
	if !api.Authorize(r, rbac.ActionRead, templateVersion.RBACObject(template)) {
		httpapi.ResourceNotFound(rw)
		return
	}
	job, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusPreconditionFailed, codersdk.Response{
			Message: "Job hasn't completed!",
		})
		return
	}
	dbTemplateVersionParameters, err := api.Database.GetTemplateVersionParameters(ctx, templateVersion.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version parameters.",
			Detail:  err.Error(),
		})
		return
	}

	templateVersionParameters, err := convertTemplateVersionParameters(dbTemplateVersionParameters)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting template version parameter.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, templateVersionParameters)
}

// @Summary Get template variables by template version
// @ID get-template-variables-by-template-version
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200 {array} codersdk.TemplateVersionVariable
// @Router /templateversions/{templateversion}/variables [get]
func (api *API) templateVersionVariables(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	templateVersion := httpmw.TemplateVersionParam(r)
	template := httpmw.TemplateParam(r)
	if !api.Authorize(r, rbac.ActionRead, templateVersion.RBACObject(template)) {
		httpapi.ResourceNotFound(rw)
		return
	}
	job, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusPreconditionFailed, codersdk.Response{
			Message: "Job hasn't completed!",
		})
		return
	}
	dbTemplateVersionVariables, err := api.Database.GetTemplateVersionVariables(ctx, templateVersion.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version variables.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertTemplateVersionVariables(dbTemplateVersionVariables))
}

// @Summary Get parameters by template version
// @ID get-parameters-by-template-version
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200 {array} parameter.ComputedValue
// @Router /templateversions/{templateversion}/parameters [get]
func (api *API) templateVersionParameters(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var (
		templateVersion = httpmw.TemplateVersionParam(r)
		template        = httpmw.TemplateParam(r)
	)
	if !api.Authorize(r, rbac.ActionRead, templateVersion.RBACObject(template)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	job, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Job hasn't completed!",
		})
		return
	}
	values, err := parameter.Compute(ctx, api.Database, parameter.ComputeScope{
		TemplateImportJobID: job.ID,
	}, &parameter.ComputeOptions{
		// We *never* want to send the client secret parameter values.
		HideRedisplayValues: true,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error computing values.",
			Detail:  err.Error(),
		})
		return
	}
	if values == nil {
		values = []parameter.ComputedValue{}
	}

	httpapi.Write(ctx, rw, http.StatusOK, values)
}

// @Summary Create template version dry-run
// @ID create-template-version-dry-run
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Param request body codersdk.CreateTemplateVersionDryRunRequest true "Dry-run request"
// @Success 201 {object} codersdk.ProvisionerJob
// @Router /templateversions/{templateversion}/dry-run [post]
func (api *API) postTemplateVersionDryRun(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var (
		apiKey          = httpmw.APIKey(r)
		templateVersion = httpmw.TemplateVersionParam(r)
		template        = httpmw.TemplateParam(r)
	)
	if !api.Authorize(r, rbac.ActionRead, templateVersion.RBACObject(template)) {
		httpapi.ResourceNotFound(rw)
		return
	}
	// We use the workspace RBAC check since we don't want to allow dry runs if
	// the user can't create workspaces.
	if !api.Authorize(r, rbac.ActionCreate,
		rbac.ResourceWorkspace.InOrg(templateVersion.OrganizationID).WithOwner(apiKey.UserID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.CreateTemplateVersionDryRunRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	job, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Template version import job hasn't completed!",
		})
		return
	}

	// Convert parameters from request to parameters for the job
	parameterValues := make([]database.ParameterValue, len(req.ParameterValues))
	for i, v := range req.ParameterValues {
		parameterValues[i] = database.ParameterValue{
			ID:                uuid.Nil,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           uuid.Nil,
			Name:              v.Name,
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       v.SourceValue,
			DestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		}
	}

	richParameterValues := make([]database.WorkspaceBuildParameter, len(req.RichParameterValues))
	for i, v := range req.RichParameterValues {
		richParameterValues[i] = database.WorkspaceBuildParameter{
			WorkspaceBuildID: uuid.Nil,
			Name:             v.Name,
			Value:            v.Value,
		}
	}

	// Marshal template version dry-run job with the parameters from the
	// request.
	input, err := json.Marshal(provisionerdserver.TemplateVersionDryRunJob{
		TemplateVersionID:   templateVersion.ID,
		WorkspaceName:       req.WorkspaceName,
		ParameterValues:     parameterValues,
		RichParameterValues: richParameterValues,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error unmarshalling provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	// Create a dry-run job
	jobID := uuid.New()
	provisionerJob, err := api.Database.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
		ID:             jobID,
		CreatedAt:      database.Now(),
		UpdatedAt:      database.Now(),
		OrganizationID: templateVersion.OrganizationID,
		InitiatorID:    apiKey.UserID,
		Provisioner:    job.Provisioner,
		StorageMethod:  job.StorageMethod,
		FileID:         job.FileID,
		Type:           database.ProvisionerJobTypeTemplateVersionDryRun,
		Input:          input,
		// Copy tags from the previous run.
		Tags: job.Tags,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error inserting provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, convertProvisionerJob(provisionerJob))
}

// @Summary Get template version dry-run by job ID
// @ID get-template-version-dry-run-by-job-id
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Param jobID path string true "Job ID" format(uuid)
// @Success 200 {object} codersdk.ProvisionerJob
// @Router /templateversions/{templateversion}/dry-run/{jobID} [get]
func (api *API) templateVersionDryRun(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	job, ok := api.fetchTemplateVersionDryRunJob(rw, r)
	if !ok {
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertProvisionerJob(job))
}

// @Summary Get template version dry-run resources by job ID
// @ID get-template-version-dry-run-resources-by-job-id
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Param jobID path string true "Job ID" format(uuid)
// @Success 200 {array} codersdk.WorkspaceResource
// @Router /templateversions/{templateversion}/dry-run/{jobID}/resources [get]
func (api *API) templateVersionDryRunResources(rw http.ResponseWriter, r *http.Request) {
	job, ok := api.fetchTemplateVersionDryRunJob(rw, r)
	if !ok {
		return
	}

	api.provisionerJobResources(rw, r, job)
}

// @Summary Get template version dry-run logs by job ID
// @ID get-template-version-dry-run-logs-by-job-id
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Param jobID path string true "Job ID" format(uuid)
// @Param before query int false "Before Unix timestamp"
// @Param after query int false "After Unix timestamp"
// @Param follow query bool false "Follow log stream"
// @Success 200 {array} codersdk.ProvisionerJobLog
// @Router /templateversions/{templateversion}/dry-run/{jobID}/logs [get]
func (api *API) templateVersionDryRunLogs(rw http.ResponseWriter, r *http.Request) {
	job, ok := api.fetchTemplateVersionDryRunJob(rw, r)
	if !ok {
		return
	}

	api.provisionerJobLogs(rw, r, job)
}

// @Summary Cancel template version dry-run by job ID
// @ID cancel-template-version-dry-run-by-job-id
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param jobID path string true "Job ID" format(uuid)
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200 {object} codersdk.Response
// @Router /templateversions/{templateversion}/dry-run/{jobID}/cancel [patch]
func (api *API) patchTemplateVersionDryRunCancel(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	templateVersion := httpmw.TemplateVersionParam(r)

	job, ok := api.fetchTemplateVersionDryRunJob(rw, r)
	if !ok {
		return
	}
	if !api.Authorize(r, rbac.ActionUpdate,
		rbac.ResourceWorkspace.InOrg(templateVersion.OrganizationID).WithOwner(job.InitiatorID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}

	if job.CompletedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Job has already completed.",
		})
		return
	}
	if job.CanceledAt.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Job has already been marked as canceled.",
		})
		return
	}

	err := api.Database.UpdateProvisionerJobWithCancelByID(ctx, database.UpdateProvisionerJobWithCancelByIDParams{
		ID: job.ID,
		CanceledAt: sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		},
		CompletedAt: sql.NullTime{
			Time: database.Now(),
			// If the job is running, don't mark it completed!
			Valid: !job.WorkerID.Valid,
		},
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Job has been marked as canceled.",
	})
}

func (api *API) fetchTemplateVersionDryRunJob(rw http.ResponseWriter, r *http.Request) (database.ProvisionerJob, bool) {
	var (
		ctx             = r.Context()
		templateVersion = httpmw.TemplateVersionParam(r)
		template        = httpmw.TemplateParam(r)
		jobID           = chi.URLParam(r, "jobID")
	)

	if !api.Authorize(r, rbac.ActionRead, templateVersion.RBACObject(template)) {
		httpapi.ResourceNotFound(rw)
		return database.ProvisionerJob{}, false
	}

	jobUUID, err := uuid.Parse(jobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Job ID %q must be a valid UUID.", jobID),
			Detail:  err.Error(),
		})
		return database.ProvisionerJob{}, false
	}

	job, err := api.Database.GetProvisionerJobByID(ctx, jobUUID)
	if xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Provisioner job %q not found.", jobUUID),
		})
		return database.ProvisionerJob{}, false
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return database.ProvisionerJob{}, false
	}
	if job.Type != database.ProvisionerJobTypeTemplateVersionDryRun {
		httpapi.Forbidden(rw)
		return database.ProvisionerJob{}, false
	}

	// Do a workspace resource check since it's basically a workspace dry-run.
	if !api.Authorize(r, rbac.ActionRead,
		rbac.ResourceWorkspace.InOrg(templateVersion.OrganizationID).WithOwner(job.InitiatorID.String())) {
		httpapi.Forbidden(rw)
		return database.ProvisionerJob{}, false
	}

	// Verify that the template version is the one used in the request.
	var input provisionerdserver.TemplateVersionDryRunJob
	err = json.Unmarshal(job.Input, &input)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error unmarshalling job metadata.",
			Detail:  err.Error(),
		})
		return database.ProvisionerJob{}, false
	}
	if input.TemplateVersionID != templateVersion.ID {
		httpapi.Forbidden(rw)
		return database.ProvisionerJob{}, false
	}

	return job, true
}

// @Summary List template versions by template ID
// @ID list-template-versions-by-template-id
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param template path string true "Template ID" format(uuid)
// @Param after_id query string false "After ID" format(uuid)
// @Param limit query int false "Page limit"
// @Param offset query int false "Page offset"
// @Success 200 {array} codersdk.TemplateVersion
// @Router /templates/{template}/versions [get]
func (api *API) templateVersionsByTemplate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	template := httpmw.TemplateParam(r)
	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	paginationParams, ok := parsePagination(rw, r)
	if !ok {
		return
	}

	var err error
	apiVersions := []codersdk.TemplateVersion{}
	err = api.Database.InTx(func(store database.Store) error {
		if paginationParams.AfterID != uuid.Nil {
			// See if the record exists first. If the record does not exist, the pagination
			// query will not work.
			_, err := store.GetTemplateVersionByID(ctx, paginationParams.AfterID)
			if err != nil && xerrors.Is(err, sql.ErrNoRows) {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: fmt.Sprintf("Record at \"after_id\" (%q) does not exists.", paginationParams.AfterID.String()),
				})
				return err
			} else if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching template version at after_id.",
					Detail:  err.Error(),
				})
				return err
			}
		}

		versions, err := store.GetTemplateVersionsByTemplateID(ctx, database.GetTemplateVersionsByTemplateIDParams{
			TemplateID: template.ID,
			AfterID:    paginationParams.AfterID,
			LimitOpt:   int32(paginationParams.Limit),
			OffsetOpt:  int32(paginationParams.Offset),
		})
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusOK, apiVersions)
			return err
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching template versions.",
				Detail:  err.Error(),
			})
			return err
		}

		jobIDs := make([]uuid.UUID, 0, len(versions))
		for _, version := range versions {
			jobIDs = append(jobIDs, version.JobID)
		}
		jobs, err := store.GetProvisionerJobsByIDs(ctx, jobIDs)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching provisioner job.",
				Detail:  err.Error(),
			})
			return err
		}
		jobByID := map[string]database.ProvisionerJob{}
		for _, job := range jobs {
			jobByID[job.ID.String()] = job
		}

		for _, version := range versions {
			job, exists := jobByID[version.JobID.String()]
			if !exists {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: fmt.Sprintf("Job %q doesn't exist for version %q.", version.JobID, version.ID),
				})
				return err
			}
			user, err := store.GetUserByID(ctx, version.CreatedBy)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error on fetching user.",
					Detail:  err.Error(),
				})
				return err
			}
			apiVersions = append(apiVersions, convertTemplateVersion(version, convertProvisionerJob(job), user))
		}

		return nil
	}, nil)
	if err != nil {
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, apiVersions)
}

// @Summary Get template version by template ID and name
// @ID get-template-version-by-template-id-and-name
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param template path string true "Template ID" format(uuid)
// @Param templateversionname path string true "Template version name"
// @Success 200 {array} codersdk.TemplateVersion
// @Router /templates/{template}/versions/{templateversionname} [get]
func (api *API) templateVersionByName(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	template := httpmw.TemplateParam(r)
	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	templateVersionName := chi.URLParam(r, "templateversionname")
	templateVersion, err := api.Database.GetTemplateVersionByTemplateIDAndName(ctx, database.GetTemplateVersionByTemplateIDAndNameParams{
		TemplateID: uuid.NullUUID{
			UUID:  template.ID,
			Valid: true,
		},
		Name: templateVersionName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("No template version found by name %q.", templateVersionName),
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version.",
			Detail:  err.Error(),
		})
		return
	}
	job, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	user, err := api.Database.GetUserByID(ctx, templateVersion.CreatedBy)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error on fetching user.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertTemplateVersion(templateVersion, convertProvisionerJob(job), user))
}

// @Summary Get template version by organization, template, and name
// @ID get-template-version-by-organization-template-and-name
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param organization path string true "Organization ID" format(uuid)
// @Param templatename path string true "Template name"
// @Param templateversionname path string true "Template version name"
// @Success 200 {object} codersdk.TemplateVersion
// @Router /organizations/{organization}/templates/{templatename}/versions/{templateversionname} [get]
func (api *API) templateVersionByOrganizationTemplateAndName(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)
	templateName := chi.URLParam(r, "templatename")

	template, err := api.Database.GetTemplateByOrganizationAndName(ctx, database.GetTemplateByOrganizationAndNameParams{
		OrganizationID: organization.ID,
		Name:           templateName,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template.",
			Detail:  err.Error(),
		})
		return
	}

	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	templateVersionName := chi.URLParam(r, "templateversionname")
	templateVersion, err := api.Database.GetTemplateVersionByTemplateIDAndName(ctx, database.GetTemplateVersionByTemplateIDAndNameParams{
		TemplateID: uuid.NullUUID{
			UUID:  template.ID,
			Valid: true,
		},
		Name: templateVersionName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("No template version found by name %q.", templateVersionName),
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version.",
			Detail:  err.Error(),
		})
		return
	}
	job, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	user, err := api.Database.GetUserByID(ctx, templateVersion.CreatedBy)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error on fetching user.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertTemplateVersion(templateVersion, convertProvisionerJob(job), user))
}

// @Summary Get previous template version by organization, template, and name
// @ID get-previous-template-version-by-organization-template-and-name
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param organization path string true "Organization ID" format(uuid)
// @Param templatename path string true "Template name"
// @Param templateversionname path string true "Template version name"
// @Success 200 {object} codersdk.TemplateVersion
// @Router /organizations/{organization}/templates/{templatename}/versions/{templateversionname}/previous [get]
func (api *API) previousTemplateVersionByOrganizationTemplateAndName(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)
	templateName := chi.URLParam(r, "templatename")
	template, err := api.Database.GetTemplateByOrganizationAndName(ctx, database.GetTemplateByOrganizationAndNameParams{
		OrganizationID: organization.ID,
		Name:           templateName,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template.",
			Detail:  err.Error(),
		})
		return
	}

	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	templateVersionName := chi.URLParam(r, "templateversionname")
	templateVersion, err := api.Database.GetTemplateVersionByTemplateIDAndName(ctx, database.GetTemplateVersionByTemplateIDAndNameParams{
		TemplateID: uuid.NullUUID{
			UUID:  template.ID,
			Valid: true,
		},
		Name: templateVersionName,
	})
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: fmt.Sprintf("No template version found by name %q.", templateVersionName),
			})
			return
		}

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version.",
			Detail:  err.Error(),
		})
		return
	}

	previousTemplateVersion, err := api.Database.GetPreviousTemplateVersion(ctx, database.GetPreviousTemplateVersionParams{
		OrganizationID: organization.ID,
		Name:           templateVersionName,
		TemplateID:     templateVersion.TemplateID,
	})

	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: fmt.Sprintf("No previous template version found for %q.", templateVersionName),
			})
			return
		}

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching the previous template version.",
			Detail:  err.Error(),
		})
		return
	}

	job, err := api.Database.GetProvisionerJobByID(ctx, previousTemplateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	user, err := api.Database.GetUserByID(ctx, templateVersion.CreatedBy)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error on fetching user.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertTemplateVersion(previousTemplateVersion, convertProvisionerJob(job), user))
}

// @Summary Update active template version by template ID
// @ID update-active-template-version-by-template-id
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Templates
// @Param request body codersdk.UpdateActiveTemplateVersion true "Modified template version"
// @Param template path string true "Template ID" format(uuid)
// @Success 200 {object} codersdk.Response
// @Router /templates/{template}/versions [patch]
func (api *API) patchActiveTemplateVersion(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		template          = httpmw.TemplateParam(r)
		auditor           = *api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.Template](rw, &audit.RequestParams{
			Audit:   auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()
	aReq.Old = template

	if !api.Authorize(r, rbac.ActionUpdate, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.UpdateActiveTemplateVersion
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	version, err := api.Database.GetTemplateVersionByID(ctx, req.ID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Template version not found.",
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version.",
			Detail:  err.Error(),
		})
		return
	}
	if version.TemplateID.UUID.String() != template.ID.String() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The provided template version doesn't belong to the specified template.",
		})
		return
	}

	err = api.Database.InTx(func(store database.Store) error {
		err = store.UpdateTemplateActiveVersionByID(ctx, database.UpdateTemplateActiveVersionByIDParams{
			ID:              template.ID,
			ActiveVersionID: req.ID,
			UpdatedAt:       database.Now(),
		})
		if err != nil {
			return xerrors.Errorf("update active version: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating active template version.",
			Detail:  err.Error(),
		})
		return
	}
	newTemplate := template
	newTemplate.ActiveVersionID = req.ID
	aReq.New = newTemplate

	api.publishTemplateUpdate(ctx, template.ID)

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Updated the active template version!",
	})
}

// postTemplateVersionsByOrganization creates a new version of a template. An import job is queued to parse the storage method provided.
//
// @Summary Create template version by organization
// @ID create-template-version-by-organization
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Templates
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.CreateTemplateVersionDryRunRequest true "Create template version request"
// @Success 201 {object} codersdk.TemplateVersion
// @Router /organizations/{organization}/templateversions [post]
func (api *API) postTemplateVersionsByOrganization(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		apiKey            = httpmw.APIKey(r)
		organization      = httpmw.OrganizationParam(r)
		auditor           = *api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.TemplateVersion](rw, &audit.RequestParams{
			Audit:   auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})

		req codersdk.CreateTemplateVersionRequest
	)
	defer commitAudit()

	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var template database.Template
	if req.TemplateID != uuid.Nil {
		var err error
		template, err = api.Database.GetTemplateByID(ctx, req.TemplateID)
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Template does not exist.",
			})
			return
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching template.",
				Detail:  err.Error(),
			})
			return
		}
	}

	if template.ID != uuid.Nil {
		if !api.Authorize(r, rbac.ActionCreate, template) {
			httpapi.ResourceNotFound(rw)
			return
		}
	} else if !api.Authorize(r, rbac.ActionCreate, rbac.ResourceTemplate.InOrg(organization.ID)) {
		// Making a new template version is the same permission as creating a new template.
		httpapi.ResourceNotFound(rw)
		return
	}

	// Ensures the "owner" is properly applied.
	tags := provisionerdserver.MutateTags(apiKey.UserID, req.ProvisionerTags)

	if req.ExampleID != "" && req.FileID != uuid.Nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "You cannot specify both an example_id and a file_id.",
		})
		return
	}

	var file database.File
	var err error
	// if example id is specified we need to copy the embedded tar into a new file in the database
	if req.ExampleID != "" {
		if !api.Authorize(r, rbac.ActionCreate, rbac.ResourceFile.WithOwner(apiKey.UserID.String())) {
			httpapi.Forbidden(rw)
			return
		}
		// ensure we can read the file that either already exists or will be created
		if !api.Authorize(r, rbac.ActionRead, rbac.ResourceFile.WithOwner(apiKey.UserID.String())) {
			httpapi.Forbidden(rw)
			return
		}

		// lookup template tar from embedded examples
		tar, err := examples.Archive(req.ExampleID)
		if err != nil {
			if xerrors.Is(err, examples.ErrNotFound) {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Example not found.",
					Detail:  err.Error(),
				})
				return
			}
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching example.",
				Detail:  err.Error(),
			})
			return
		}

		// upload a copy of the template tar as a file in the database
		hashBytes := sha256.Sum256(tar)
		hash := hex.EncodeToString(hashBytes[:])
		// Check if the file already exists.
		file, err := api.Database.GetFileByHashAndCreator(ctx, database.GetFileByHashAndCreatorParams{
			Hash:      hash,
			CreatedBy: apiKey.UserID,
		})
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching file.",
					Detail:  err.Error(),
				})
				return
			}

			// If the example tar file doesn't exist, create it.
			file, err = api.Database.InsertFile(ctx, database.InsertFileParams{
				ID:        uuid.New(),
				Hash:      hash,
				CreatedBy: apiKey.UserID,
				CreatedAt: database.Now(),
				Mimetype:  tarMimeType,
				Data:      tar,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error creating file.",
					Detail:  err.Error(),
				})
				return
			}
		}

		req.FileID = file.ID
	}

	if req.FileID != uuid.Nil {
		file, err = api.Database.GetFileByID(ctx, req.FileID)
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "File not found.",
			})
			return
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching file.",
				Detail:  err.Error(),
			})
			return
		}
	}

	if !api.Authorize(r, rbac.ActionRead, file) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var templateVersion database.TemplateVersion
	var provisionerJob database.ProvisionerJob
	err = api.Database.InTx(func(tx database.Store) error {
		jobID := uuid.New()
		inherits := make([]uuid.UUID, 0)
		for _, parameterValue := range req.ParameterValues {
			if parameterValue.CloneID != uuid.Nil {
				inherits = append(inherits, parameterValue.CloneID)
			}
		}

		// Expand inherited params
		if len(inherits) > 0 {
			if req.TemplateID == uuid.Nil {
				return xerrors.Errorf("cannot inherit parameters if template_id is not set")
			}

			inheritedParams, err := tx.ParameterValues(ctx, database.ParameterValuesParams{
				IDs: inherits,
			})
			if err != nil {
				return xerrors.Errorf("fetch inherited params: %w", err)
			}
			for _, copy := range inheritedParams {
				// This is a bit inefficient, as we make a new db call for each
				// param.
				version, err := tx.GetTemplateVersionByJobID(ctx, copy.ScopeID)
				if err != nil {
					return xerrors.Errorf("fetch template version for param %q: %w", copy.Name, err)
				}
				if !version.TemplateID.Valid || version.TemplateID.UUID != req.TemplateID {
					return xerrors.Errorf("cannot inherit parameters from other templates")
				}
				if copy.Scope != database.ParameterScopeImportJob {
					return xerrors.Errorf("copy parameter scope is %q, must be %q", copy.Scope, database.ParameterScopeImportJob)
				}
				// Add the copied param to the list to process
				req.ParameterValues = append(req.ParameterValues, codersdk.CreateParameterRequest{
					Name:              copy.Name,
					SourceValue:       copy.SourceValue,
					SourceScheme:      codersdk.ParameterSourceScheme(copy.SourceScheme),
					DestinationScheme: codersdk.ParameterDestinationScheme(copy.DestinationScheme),
				})
			}
		}

		for _, parameterValue := range req.ParameterValues {
			if parameterValue.CloneID != uuid.Nil {
				continue
			}

			_, err = tx.InsertParameterValue(ctx, database.InsertParameterValueParams{
				ID:                uuid.New(),
				Name:              parameterValue.Name,
				CreatedAt:         database.Now(),
				UpdatedAt:         database.Now(),
				Scope:             database.ParameterScopeImportJob,
				ScopeID:           jobID,
				SourceScheme:      database.ParameterSourceScheme(parameterValue.SourceScheme),
				SourceValue:       parameterValue.SourceValue,
				DestinationScheme: database.ParameterDestinationScheme(parameterValue.DestinationScheme),
			})
			if err != nil {
				return xerrors.Errorf("insert parameter value: %w", err)
			}
		}

		templateVersionID := uuid.New()
		jobInput, err := json.Marshal(provisionerdserver.TemplateVersionImportJob{
			TemplateVersionID:  templateVersionID,
			UserVariableValues: req.UserVariableValues,
		})
		if err != nil {
			return xerrors.Errorf("marshal job input: %w", err)
		}

		provisionerJob, err = tx.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:             jobID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			OrganizationID: organization.ID,
			InitiatorID:    apiKey.UserID,
			Provisioner:    database.ProvisionerType(req.Provisioner),
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          jobInput,
			Tags:           tags,
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}

		var templateID uuid.NullUUID
		if req.TemplateID != uuid.Nil {
			templateID = uuid.NullUUID{
				UUID:  req.TemplateID,
				Valid: true,
			}
		}

		if req.Name == "" {
			req.Name = namesgenerator.GetRandomName(1)
		}

		templateVersion, err = tx.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
			ID:             templateVersionID,
			TemplateID:     templateID,
			OrganizationID: organization.ID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			Name:           req.Name,
			Readme:         "",
			JobID:          provisionerJob.ID,
			CreatedBy:      apiKey.UserID,
		})
		if err != nil {
			return xerrors.Errorf("insert template version: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: err.Error(),
		})
		return
	}
	aReq.New = templateVersion

	user, err := api.Database.GetUserByID(ctx, templateVersion.CreatedBy)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error on fetching user.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, convertTemplateVersion(templateVersion, convertProvisionerJob(provisionerJob), user))
}

// templateVersionResources returns the workspace agent resources associated
// with a template version. A template can specify more than one resource to be
// provisioned, each resource can have an agent that dials back to coderd. The
// agents returned are informative of the template version, and do not return
// agents associated with any particular workspace.
//
// @Summary Get resources by template version
// @ID get-resources-by-template-version
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200 {array} codersdk.WorkspaceResource
// @Router /templateversions/{templateversion}/resources [get]
func (api *API) templateVersionResources(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx             = r.Context()
		templateVersion = httpmw.TemplateVersionParam(r)
		template        = httpmw.TemplateParam(r)
	)

	if !api.Authorize(r, rbac.ActionRead, templateVersion.RBACObject(template)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	job, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	api.provisionerJobResources(rw, r, job)
}

// templateVersionLogs returns the logs returned by the provisioner for the given
// template version. These logs are only associated with the template version,
// and not any build logs for a workspace.
// Eg: Logs returned from 'terraform plan' when uploading a new terraform file.
//
// @Summary Get logs by template version
// @ID get-logs-by-template-version
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Param before query int false "Before Unix timestamp"
// @Param after query int false "After Unix timestamp"
// @Param follow query bool false "Follow log stream"
// @Success 200 {array} codersdk.ProvisionerJobLog
// @Router /templateversions/{templateversion}/logs [get]
func (api *API) templateVersionLogs(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx             = r.Context()
		templateVersion = httpmw.TemplateVersionParam(r)
		template        = httpmw.TemplateParam(r)
	)

	if !api.Authorize(r, rbac.ActionRead, templateVersion.RBACObject(template)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	job, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	api.provisionerJobLogs(rw, r, job)
}

func convertTemplateVersion(version database.TemplateVersion, job codersdk.ProvisionerJob, user database.User) codersdk.TemplateVersion {
	createdBy := codersdk.User{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
		Status:    codersdk.UserStatus(user.Status),
		Roles:     []codersdk.Role{},
		AvatarURL: user.AvatarURL.String,
	}

	return codersdk.TemplateVersion{
		ID:             version.ID,
		TemplateID:     &version.TemplateID.UUID,
		OrganizationID: version.OrganizationID,
		CreatedAt:      version.CreatedAt,
		UpdatedAt:      version.UpdatedAt,
		Name:           version.Name,
		Job:            job,
		Readme:         version.Readme,
		CreatedBy:      createdBy,
	}
}

func convertTemplateVersionParameters(dbParams []database.TemplateVersionParameter) ([]codersdk.TemplateVersionParameter, error) {
	params := make([]codersdk.TemplateVersionParameter, 0)
	for _, dbParameter := range dbParams {
		param, err := convertTemplateVersionParameter(dbParameter)
		if err != nil {
			return nil, err
		}
		params = append(params, param)
	}
	return params, nil
}

func convertTemplateVersionParameter(param database.TemplateVersionParameter) (codersdk.TemplateVersionParameter, error) {
	var protoOptions []*sdkproto.RichParameterOption
	err := json.Unmarshal(param.Options, &protoOptions)
	if err != nil {
		return codersdk.TemplateVersionParameter{}, err
	}
	options := make([]codersdk.TemplateVersionParameterOption, 0)
	for _, option := range protoOptions {
		options = append(options, codersdk.TemplateVersionParameterOption{
			Name:        option.Name,
			Description: option.Description,
			Value:       option.Value,
			Icon:        option.Icon,
		})
	}

	descriptionPlaintext, err := parameter.Plaintext(param.Description)
	if err != nil {
		return codersdk.TemplateVersionParameter{}, err
	}
	return codersdk.TemplateVersionParameter{
		Name:                 param.Name,
		Description:          param.Description,
		DescriptionPlaintext: descriptionPlaintext,
		Type:                 param.Type,
		Mutable:              param.Mutable,
		DefaultValue:         param.DefaultValue,
		Icon:                 param.Icon,
		Options:              options,
		ValidationRegex:      param.ValidationRegex,
		ValidationMin:        param.ValidationMin,
		ValidationMax:        param.ValidationMax,
		ValidationError:      param.ValidationError,
		ValidationMonotonic:  codersdk.ValidationMonotonicOrder(param.ValidationMonotonic),
	}, nil
}

func convertTemplateVersionVariables(dbVariables []database.TemplateVersionVariable) []codersdk.TemplateVersionVariable {
	variables := make([]codersdk.TemplateVersionVariable, 0)
	for _, dbVariable := range dbVariables {
		variables = append(variables, convertTemplateVersionVariable(dbVariable))
	}
	return variables
}

func convertTemplateVersionVariable(variable database.TemplateVersionVariable) codersdk.TemplateVersionVariable {
	return codersdk.TemplateVersionVariable{
		Name:         variable.Name,
		Description:  variable.Description,
		Type:         variable.Type,
		Value:        variable.Value,
		DefaultValue: variable.DefaultValue,
		Required:     variable.Required,
		Sensitive:    variable.Sensitive,
	}
}

func watchTemplateChannel(id uuid.UUID) string {
	return fmt.Sprintf("template:%s", id)
}

func (api *API) publishTemplateUpdate(ctx context.Context, templateID uuid.UUID) {
	err := api.Pubsub.Publish(watchTemplateChannel(templateID), []byte{})
	if err != nil {
		api.Logger.Warn(ctx, "failed to publish template update",
			slog.F("template_id", templateID), slog.Error(err))
	}
}
