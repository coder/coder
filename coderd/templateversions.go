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
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/parameter"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/examples"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
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
	templateVersion := httpmw.TemplateVersionParam(r)

	jobs, err := api.Database.GetProvisionerJobsByIDsWithQueuePosition(ctx, []uuid.UUID{templateVersion.JobID})
	if err != nil || len(jobs) == 0 {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	schemas, err := api.Database.GetParameterSchemasByJobID(ctx, jobs[0].ProvisionerJob.ID)
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

	var warnings []codersdk.TemplateVersionWarning
	if len(schemas) > 0 {
		warnings = append(warnings, codersdk.TemplateVersionWarningUnsupportedWorkspaces)
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertTemplateVersion(templateVersion, convertProvisionerJob(jobs[0]), warnings))
}

// @Summary Patch template version by ID
// @ID patch-template-version-by-id
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Param request body codersdk.PatchTemplateVersionRequest true "Patch template version request"
// @Success 200 {object} codersdk.TemplateVersion
// @Router /templateversions/{templateversion} [patch]
func (api *API) patchTemplateVersion(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	templateVersion := httpmw.TemplateVersionParam(r)

	var params codersdk.PatchTemplateVersionRequest
	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}

	updateParams := database.UpdateTemplateVersionByIDParams{
		ID:         templateVersion.ID,
		TemplateID: templateVersion.TemplateID,
		UpdatedAt:  dbtime.Now(),
		Name:       templateVersion.Name,
		Message:    templateVersion.Message,
	}

	if params.Name != "" {
		updateParams.Name = params.Name
	}

	if params.Message != nil {
		updateParams.Message = *params.Message
	}

	errTemplateVersionNameConflict := xerrors.New("template version name must be unique for a template")

	var updatedTemplateVersion database.TemplateVersion
	err := api.Database.InTx(func(tx database.Store) error {
		if templateVersion.TemplateID.Valid && templateVersion.Name != updateParams.Name {
			// User wants to rename the template version

			_, err := tx.GetTemplateVersionByTemplateIDAndName(ctx, database.GetTemplateVersionByTemplateIDAndNameParams{
				TemplateID: templateVersion.TemplateID,
				Name:       updateParams.Name,
			})
			if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
				return xerrors.Errorf("error on retrieving conflicting template version: %v", err)
			}
			if err == nil {
				return errTemplateVersionNameConflict
			}
		}

		// It is not allowed to "patch" the template ID, and reassign it.
		var err error
		err = tx.UpdateTemplateVersionByID(ctx, updateParams)
		if err != nil {
			return xerrors.Errorf("error on patching template version: %v", err)
		}

		updatedTemplateVersion, err = tx.GetTemplateVersionByID(ctx, updateParams.ID)
		if err != nil {
			return xerrors.Errorf("error on fetching patched template version: %v", err)
		}

		return nil
	}, nil)
	if errors.Is(err, errTemplateVersionNameConflict) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: err.Error(),
			Validations: []codersdk.ValidationError{
				{Field: "name", Detail: "Name is already used"},
			},
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: err.Error(),
		})
		return
	}

	jobs, err := api.Database.GetProvisionerJobsByIDsWithQueuePosition(ctx, []uuid.UUID{templateVersion.JobID})
	if err != nil || len(jobs) == 0 {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertTemplateVersion(updatedTemplateVersion, convertProvisionerJob(jobs[0]), nil))
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
	templateVersion := httpmw.TemplateVersionParam(r)

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
			Time:  dbtime.Now(),
			Valid: true,
		},
		CompletedAt: sql.NullTime{
			Time: dbtime.Now(),
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

	job, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
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

// @Summary Get external auth by template version
// @ID get-external-auth-by-template-version
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200 {array} codersdk.TemplateVersionExternalAuth
// @Router /templateversions/{templateversion}/external-auth [get]
func (api *API) templateVersionExternalAuth(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var (
		apiKey          = httpmw.APIKey(r)
		templateVersion = httpmw.TemplateVersionParam(r)
	)

	rawProviders := templateVersion.ExternalAuthProviders
	providers := make([]codersdk.TemplateVersionExternalAuth, 0)
	for _, rawProvider := range rawProviders {
		var config *externalauth.Config
		for _, provider := range api.ExternalAuthConfigs {
			if provider.ID == rawProvider {
				config = provider
				break
			}
		}
		if config == nil {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: fmt.Sprintf("The template version references a Git auth provider %q that no longer exists.", rawProvider),
				Detail:  "You'll need to update the template version to use a different provider.",
			})
			return
		}

		// This is the URL that will redirect the user with a state token.
		redirectURL, err := api.AccessURL.Parse(fmt.Sprintf("/external-auth/%s", config.ID))
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to parse access URL.",
				Detail:  err.Error(),
			})
			return
		}

		provider := codersdk.TemplateVersionExternalAuth{
			ID:              config.ID,
			Type:            config.Type,
			AuthenticateURL: redirectURL.String(),
			DisplayName:     config.DisplayName,
			DisplayIcon:     config.DisplayIcon,
		}

		authLink, err := api.Database.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
			ProviderID: config.ID,
			UserID:     apiKey.UserID,
		})
		// If there isn't an auth link, then the user just isn't authenticated.
		if errors.Is(err, sql.ErrNoRows) {
			providers = append(providers, provider)
			continue
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching external auth link.",
				Detail:  err.Error(),
			})
			return
		}

		_, updated, err := config.RefreshToken(ctx, api.Database, authLink)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to refresh external auth token.",
				Detail:  err.Error(),
			})
			return
		}
		// If the token couldn't be validated, then we assume the user isn't
		// authenticated and return early.
		if !updated {
			providers = append(providers, provider)
			continue
		}
		provider.Authenticated = true
		providers = append(providers, provider)
	}

	httpapi.Write(ctx, rw, http.StatusOK, providers)
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

	job, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
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
	)

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
		RichParameterValues: richParameterValues,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error unmarshalling provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	metadataRaw, err := json.Marshal(tracing.MetadataFromContext(ctx))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error unmarshalling metadata.",
			Detail:  err.Error(),
		})
		return
	}

	// Create a dry-run job
	jobID := uuid.New()
	provisionerJob, err := api.Database.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
		ID:             jobID,
		CreatedAt:      dbtime.Now(),
		UpdatedAt:      dbtime.Now(),
		OrganizationID: templateVersion.OrganizationID,
		InitiatorID:    apiKey.UserID,
		Provisioner:    job.Provisioner,
		StorageMethod:  job.StorageMethod,
		FileID:         job.FileID,
		Type:           database.ProvisionerJobTypeTemplateVersionDryRun,
		Input:          input,
		// Copy tags from the previous run.
		Tags: job.Tags,
		TraceMetadata: pqtype.NullRawMessage{
			Valid:      true,
			RawMessage: metadataRaw,
		},
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error inserting provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	err = provisionerjobs.PostJob(api.Pubsub, provisionerJob)
	if err != nil {
		// Client probably doesn't care about this error, so just log it.
		api.Logger.Error(ctx, "failed to post provisioner job to pubsub", slog.Error(err))
	}

	httpapi.Write(ctx, rw, http.StatusCreated, convertProvisionerJob(database.GetProvisionerJobsByIDsWithQueuePositionRow{
		ProvisionerJob: provisionerJob,
		QueuePosition:  0,
	}))
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

	api.provisionerJobResources(rw, r, job.ProvisionerJob)
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

	api.provisionerJobLogs(rw, r, job.ProvisionerJob)
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
		rbac.ResourceWorkspace.InOrg(templateVersion.OrganizationID).WithOwner(job.ProvisionerJob.InitiatorID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}

	if job.ProvisionerJob.CompletedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Job has already completed.",
		})
		return
	}
	if job.ProvisionerJob.CanceledAt.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Job has already been marked as canceled.",
		})
		return
	}

	err := api.Database.UpdateProvisionerJobWithCancelByID(ctx, database.UpdateProvisionerJobWithCancelByIDParams{
		ID: job.ProvisionerJob.ID,
		CanceledAt: sql.NullTime{
			Time:  dbtime.Now(),
			Valid: true,
		},
		CompletedAt: sql.NullTime{
			Time: dbtime.Now(),
			// If the job is running, don't mark it completed!
			Valid: !job.ProvisionerJob.WorkerID.Valid,
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

func (api *API) fetchTemplateVersionDryRunJob(rw http.ResponseWriter, r *http.Request) (database.GetProvisionerJobsByIDsWithQueuePositionRow, bool) {
	var (
		ctx             = r.Context()
		templateVersion = httpmw.TemplateVersionParam(r)
		jobID           = chi.URLParam(r, "jobID")
	)

	jobUUID, err := uuid.Parse(jobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Job ID %q must be a valid UUID.", jobID),
			Detail:  err.Error(),
		})
		return database.GetProvisionerJobsByIDsWithQueuePositionRow{}, false
	}

	jobs, err := api.Database.GetProvisionerJobsByIDsWithQueuePosition(ctx, []uuid.UUID{jobUUID})
	if httpapi.Is404Error(err) {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Provisioner job %q not found.", jobUUID),
		})
		return database.GetProvisionerJobsByIDsWithQueuePositionRow{}, false
	}
	if err != nil || len(jobs) == 0 {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return database.GetProvisionerJobsByIDsWithQueuePositionRow{}, false
	}
	job := jobs[0]
	if job.ProvisionerJob.Type != database.ProvisionerJobTypeTemplateVersionDryRun {
		httpapi.Forbidden(rw)
		return database.GetProvisionerJobsByIDsWithQueuePositionRow{}, false
	}

	// Do a workspace resource check since it's basically a workspace dry-run.
	if !api.Authorize(r, rbac.ActionRead,
		rbac.ResourceWorkspace.InOrg(templateVersion.OrganizationID).WithOwner(job.ProvisionerJob.InitiatorID.String())) {
		httpapi.Forbidden(rw)
		return database.GetProvisionerJobsByIDsWithQueuePositionRow{}, false
	}

	// Verify that the template version is the one used in the request.
	var input provisionerdserver.TemplateVersionDryRunJob
	err = json.Unmarshal(job.ProvisionerJob.Input, &input)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error unmarshalling job metadata.",
			Detail:  err.Error(),
		})
		return database.GetProvisionerJobsByIDsWithQueuePositionRow{}, false
	}
	if input.TemplateVersionID != templateVersion.ID {
		httpapi.Forbidden(rw)
		return database.GetProvisionerJobsByIDsWithQueuePositionRow{}, false
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
		jobs, err := store.GetProvisionerJobsByIDsWithQueuePosition(ctx, jobIDs)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching provisioner job.",
				Detail:  err.Error(),
			})
			return err
		}
		jobByID := map[string]database.GetProvisionerJobsByIDsWithQueuePositionRow{}
		for _, job := range jobs {
			jobByID[job.ProvisionerJob.ID.String()] = job
		}

		for _, version := range versions {
			job, exists := jobByID[version.JobID.String()]
			if !exists {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: fmt.Sprintf("Job %q doesn't exist for version %q.", version.JobID, version.ID),
				})
				return err
			}

			apiVersions = append(apiVersions, convertTemplateVersion(version, convertProvisionerJob(job), nil))
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

	templateVersionName := chi.URLParam(r, "templateversionname")
	templateVersion, err := api.Database.GetTemplateVersionByTemplateIDAndName(ctx, database.GetTemplateVersionByTemplateIDAndNameParams{
		TemplateID: uuid.NullUUID{
			UUID:  template.ID,
			Valid: true,
		},
		Name: templateVersionName,
	})
	if httpapi.Is404Error(err) {
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
	jobs, err := api.Database.GetProvisionerJobsByIDsWithQueuePosition(ctx, []uuid.UUID{templateVersion.JobID})
	if err != nil || len(jobs) == 0 {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertTemplateVersion(templateVersion, convertProvisionerJob(jobs[0]), nil))
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
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template.",
			Detail:  err.Error(),
		})
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
	if httpapi.Is404Error(err) {
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
	jobs, err := api.Database.GetProvisionerJobsByIDsWithQueuePosition(ctx, []uuid.UUID{templateVersion.JobID})
	if err != nil || len(jobs) == 0 {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertTemplateVersion(templateVersion, convertProvisionerJob(jobs[0]), nil))
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
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template.",
			Detail:  err.Error(),
		})
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
		if httpapi.Is404Error(err) {
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
		if httpapi.Is404Error(err) {
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

	jobs, err := api.Database.GetProvisionerJobsByIDsWithQueuePosition(ctx, []uuid.UUID{previousTemplateVersion.JobID})
	if err != nil || len(jobs) == 0 {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertTemplateVersion(previousTemplateVersion, convertProvisionerJob(jobs[0]), nil))
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

	var req codersdk.UpdateActiveTemplateVersion
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	version, err := api.Database.GetTemplateVersionByID(ctx, req.ID)
	if httpapi.Is404Error(err) {
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
	job, err := api.Database.GetProvisionerJobByID(ctx, version.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version job status.",
			Detail:  err.Error(),
		})
		return
	}
	if job.JobStatus != database.ProvisionerJobStatusSucceeded {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Only versions that have been built successfully can be promoted.",
			Detail:  fmt.Sprintf("Attempted to promote a version with a %s build", job.JobStatus),
		})
		return
	}

	err = api.Database.InTx(func(store database.Store) error {
		err = store.UpdateTemplateActiveVersionByID(ctx, database.UpdateTemplateActiveVersionByIDParams{
			ID:              template.ID,
			ActiveVersionID: req.ID,
			UpdatedAt:       dbtime.Now(),
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
// @Param request body codersdk.CreateTemplateVersionRequest true "Create template version request"
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

	if req.TemplateID != uuid.Nil {
		_, err := api.Database.GetTemplateByID(ctx, req.TemplateID)
		if httpapi.Is404Error(err) {
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
				CreatedAt: dbtime.Now(),
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
		if httpapi.Is404Error(err) {
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

	var templateVersion database.TemplateVersion
	var provisionerJob database.ProvisionerJob
	err = api.Database.InTx(func(tx database.Store) error {
		jobID := uuid.New()

		templateVersionID := uuid.New()
		jobInput, err := json.Marshal(provisionerdserver.TemplateVersionImportJob{
			TemplateVersionID:  templateVersionID,
			UserVariableValues: req.UserVariableValues,
		})
		if err != nil {
			return xerrors.Errorf("marshal job input: %w", err)
		}
		traceMetadataRaw, err := json.Marshal(tracing.MetadataFromContext(ctx))
		if err != nil {
			return xerrors.Errorf("marshal job metadata: %w", err)
		}

		provisionerJob, err = tx.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:             jobID,
			CreatedAt:      dbtime.Now(),
			UpdatedAt:      dbtime.Now(),
			OrganizationID: organization.ID,
			InitiatorID:    apiKey.UserID,
			Provisioner:    database.ProvisionerType(req.Provisioner),
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          jobInput,
			Tags:           tags,
			TraceMetadata: pqtype.NullRawMessage{
				Valid:      true,
				RawMessage: traceMetadataRaw,
			},
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

		err = tx.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
			ID:             templateVersionID,
			TemplateID:     templateID,
			OrganizationID: organization.ID,
			CreatedAt:      dbtime.Now(),
			UpdatedAt:      dbtime.Now(),
			Name:           req.Name,
			Message:        req.Message,
			Readme:         "",
			JobID:          provisionerJob.ID,
			CreatedBy:      apiKey.UserID,
		})
		if err != nil {
			return xerrors.Errorf("insert template version: %w", err)
		}

		templateVersion, err = tx.GetTemplateVersionByID(ctx, templateVersionID)
		if err != nil {
			return xerrors.Errorf("fetched inserted template version: %w", err)
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
	err = provisionerjobs.PostJob(api.Pubsub, provisionerJob)
	if err != nil {
		// Client probably doesn't care about this error, so just log it.
		api.Logger.Error(ctx, "failed to post provisioner job to pubsub", slog.Error(err))
	}

	httpapi.Write(ctx, rw, http.StatusCreated, convertTemplateVersion(templateVersion, convertProvisionerJob(database.GetProvisionerJobsByIDsWithQueuePositionRow{
		ProvisionerJob: provisionerJob,
		QueuePosition:  0,
	}), nil))
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
	)

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
// @Param before query int false "Before log id"
// @Param after query int false "After log id"
// @Param follow query bool false "Follow log stream"
// @Success 200 {array} codersdk.ProvisionerJobLog
// @Router /templateversions/{templateversion}/logs [get]
func (api *API) templateVersionLogs(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx             = r.Context()
		templateVersion = httpmw.TemplateVersionParam(r)
	)

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

func convertTemplateVersion(version database.TemplateVersion, job codersdk.ProvisionerJob, warnings []codersdk.TemplateVersionWarning) codersdk.TemplateVersion {
	return codersdk.TemplateVersion{
		ID:             version.ID,
		TemplateID:     &version.TemplateID.UUID,
		OrganizationID: version.OrganizationID,
		CreatedAt:      version.CreatedAt,
		UpdatedAt:      version.UpdatedAt,
		Name:           version.Name,
		Message:        version.Message,
		Job:            job,
		Readme:         version.Readme,
		CreatedBy: codersdk.MinimalUser{
			ID:        version.CreatedBy,
			Username:  version.CreatedByUsername,
			AvatarURL: version.CreatedByAvatarURL.String,
		},
		Warnings: warnings,
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

	var validationMin, validationMax *int32
	if param.ValidationMin.Valid {
		validationMin = &param.ValidationMin.Int32
	}
	if param.ValidationMax.Valid {
		validationMax = &param.ValidationMax.Int32
	}

	return codersdk.TemplateVersionParameter{
		Name:                 param.Name,
		DisplayName:          param.DisplayName,
		Description:          param.Description,
		DescriptionPlaintext: descriptionPlaintext,
		Type:                 param.Type,
		Mutable:              param.Mutable,
		DefaultValue:         param.DefaultValue,
		Icon:                 param.Icon,
		Options:              options,
		ValidationRegex:      param.ValidationRegex,
		ValidationMin:        validationMin,
		ValidationMax:        validationMax,
		ValidationError:      param.ValidationError,
		ValidationMonotonic:  codersdk.ValidationMonotonicOrder(param.ValidationMonotonic),
		Required:             param.Required,
		Ephemeral:            param.Ephemeral,
	}, nil
}

func convertTemplateVersionVariables(dbVariables []database.TemplateVersionVariable) []codersdk.TemplateVersionVariable {
	variables := make([]codersdk.TemplateVersionVariable, 0)
	for _, dbVariable := range dbVariables {
		variables = append(variables, convertTemplateVersionVariable(dbVariable))
	}
	return variables
}

const redacted = "*redacted*"

func convertTemplateVersionVariable(variable database.TemplateVersionVariable) codersdk.TemplateVersionVariable {
	templateVariable := codersdk.TemplateVersionVariable{
		Name:         variable.Name,
		Description:  variable.Description,
		Type:         variable.Type,
		Value:        variable.Value,
		DefaultValue: variable.DefaultValue,
		Required:     variable.Required,
		Sensitive:    variable.Sensitive,
	}
	if templateVariable.Sensitive {
		templateVariable.Value = redacted
		templateVariable.DefaultValue = redacted
	}
	return templateVariable
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
