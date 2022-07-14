package coderd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

func (api *API) templateVersion(rw http.ResponseWriter, r *http.Request) {
	templateVersion := httpmw.TemplateVersionParam(r)
	if !api.Authorize(r, rbac.ActionRead, templateVersion) {
		httpapi.ResourceNotFound(rw)
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertTemplateVersion(templateVersion, convertProvisionerJob(job)))
}

func (api *API) patchCancelTemplateVersion(rw http.ResponseWriter, r *http.Request) {
	templateVersion := httpmw.TemplateVersionParam(r)
	if !api.Authorize(r, rbac.ActionUpdate, templateVersion) {
		httpapi.ResourceNotFound(rw)
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if job.CompletedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, codersdk.Response{
			Message: "Job has already completed!",
		})
		return
	}
	if job.CanceledAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, codersdk.Response{
			Message: "Job has already been marked as canceled!",
		})
		return
	}
	err = api.Database.UpdateProvisionerJobWithCancelByID(r.Context(), database.UpdateProvisionerJobWithCancelByIDParams{
		ID: job.ID,
		CanceledAt: sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		},
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, codersdk.Response{
		Message: "Job has been marked as canceled...",
	})
}

func (api *API) templateVersionSchema(rw http.ResponseWriter, r *http.Request) {
	templateVersion := httpmw.TemplateVersionParam(r)
	if !api.Authorize(r, rbac.ActionRead, templateVersion) {
		httpapi.ResourceNotFound(rw)
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, codersdk.Response{
			Message: "Template version job hasn't completed!",
		})
		return
	}
	schemas, err := api.Database.GetParameterSchemasByJobID(r.Context(), job.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error listing parameter schemas.",
			Detail:  err.Error(),
		})
		return
	}
	apiSchemas := make([]codersdk.ParameterSchema, 0)
	for _, schema := range schemas {
		apiSchema, err := convertParameterSchema(schema)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: fmt.Sprintf("Internal error converting schema %s.", schema.Name),
				Detail:  err.Error(),
			})
			return
		}
		apiSchemas = append(apiSchemas, apiSchema)
	}
	httpapi.Write(rw, http.StatusOK, apiSchemas)
}

func (api *API) templateVersionParameters(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	templateVersion := httpmw.TemplateVersionParam(r)
	if !api.Authorize(r, rbac.ActionRead, templateVersion) {
		httpapi.ResourceNotFound(rw)
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, codersdk.Response{
			Message: "Job hasn't completed!",
		})
		return
	}
	values, err := parameter.Compute(r.Context(), api.Database, parameter.ComputeScope{
		TemplateImportJobID: job.ID,
		OrganizationID:      job.OrganizationID,
		UserID:              apiKey.UserID,
	}, &parameter.ComputeOptions{
		// We *never* want to send the client secret parameter values.
		HideRedisplayValues: true,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error computing values.",
			Detail:  err.Error(),
		})
		return
	}
	if values == nil {
		values = []parameter.ComputedValue{}
	}

	httpapi.Write(rw, http.StatusOK, values)
}

func (api *API) postTemplateVersionDryRun(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	templateVersion := httpmw.TemplateVersionParam(r)
	if !api.Authorize(r, rbac.ActionRead, templateVersion) {
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
	if !httpapi.Read(rw, r, &req) {
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, codersdk.Response{
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

	// Marshal template version dry-run job with the parameters from the
	// request.
	input, err := json.Marshal(templateVersionDryRunJob{
		TemplateVersionID: templateVersion.ID,
		WorkspaceName:     req.WorkspaceName,
		ParameterValues:   parameterValues,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error unmarshalling provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	// Create a dry-run job
	jobID := uuid.New()
	provisionerJob, err := api.Database.InsertProvisionerJob(r.Context(), database.InsertProvisionerJobParams{
		ID:             jobID,
		CreatedAt:      database.Now(),
		UpdatedAt:      database.Now(),
		OrganizationID: templateVersion.OrganizationID,
		InitiatorID:    apiKey.UserID,
		Provisioner:    job.Provisioner,
		StorageMethod:  job.StorageMethod,
		StorageSource:  job.StorageSource,
		Type:           database.ProvisionerJobTypeTemplateVersionDryRun,
		Input:          input,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error inserting provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, convertProvisionerJob(provisionerJob))
}

func (api *API) templateVersionDryRun(rw http.ResponseWriter, r *http.Request) {
	job, ok := api.fetchTemplateVersionDryRunJob(rw, r)
	if !ok {
		return
	}

	httpapi.Write(rw, http.StatusOK, convertProvisionerJob(job))
}

func (api *API) templateVersionDryRunResources(rw http.ResponseWriter, r *http.Request) {
	job, ok := api.fetchTemplateVersionDryRunJob(rw, r)
	if !ok {
		return
	}

	api.provisionerJobResources(rw, r, job)
}

func (api *API) templateVersionDryRunLogs(rw http.ResponseWriter, r *http.Request) {
	job, ok := api.fetchTemplateVersionDryRunJob(rw, r)
	if !ok {
		return
	}

	api.provisionerJobLogs(rw, r, job)
}

func (api *API) patchTemplateVersionDryRunCancel(rw http.ResponseWriter, r *http.Request) {
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
		httpapi.Write(rw, http.StatusPreconditionFailed, codersdk.Response{
			Message: "Job has already completed.",
		})
		return
	}
	if job.CanceledAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, codersdk.Response{
			Message: "Job has already been marked as canceled.",
		})
		return
	}

	err := api.Database.UpdateProvisionerJobWithCancelByID(r.Context(), database.UpdateProvisionerJobWithCancelByIDParams{
		ID: job.ID,
		CanceledAt: sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		},
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, codersdk.Response{
		Message: "Job has been marked as canceled.",
	})
}

func (api *API) fetchTemplateVersionDryRunJob(rw http.ResponseWriter, r *http.Request) (database.ProvisionerJob, bool) {
	var (
		templateVersion = httpmw.TemplateVersionParam(r)
		jobID           = chi.URLParam(r, "jobID")
	)
	if !api.Authorize(r, rbac.ActionRead, templateVersion) {
		httpapi.ResourceNotFound(rw)
		return database.ProvisionerJob{}, false
	}

	jobUUID, err := uuid.Parse(jobID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Job ID %q must be a valid UUID.", jobID),
			Detail:  err.Error(),
		})
		return database.ProvisionerJob{}, false
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), jobUUID)
	if xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Provisioner job %q not found.", jobUUID),
		})
		return database.ProvisionerJob{}, false
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return database.ProvisionerJob{}, false
	}
	if job.Type != database.ProvisionerJobTypeTemplateVersionDryRun {
		httpapi.Forbidden(rw)
		return database.ProvisionerJob{}, false
	}
	// Do a workspace resource check since it's basically a workspace dry-run .
	if !api.Authorize(r, rbac.ActionRead,
		rbac.ResourceWorkspace.InOrg(templateVersion.OrganizationID).WithOwner(job.InitiatorID.String())) {
		httpapi.Forbidden(rw)
		return database.ProvisionerJob{}, false
	}

	// Verify that the template version is the one used in the request.
	var input templateVersionDryRunJob
	err = json.Unmarshal(job.Input, &input)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error unmarshaling job metadata.",
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

func (api *API) templateVersionsByTemplate(rw http.ResponseWriter, r *http.Request) {
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
			_, err := store.GetTemplateVersionByID(r.Context(), paginationParams.AfterID)
			if err != nil && xerrors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
					Message: fmt.Sprintf("Record at \"after_id\" (%q) does not exists.", paginationParams.AfterID.String()),
				})
				return err
			} else if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching template version at after_id.",
					Detail:  err.Error(),
				})
				return err
			}
		}

		versions, err := store.GetTemplateVersionsByTemplateID(r.Context(), database.GetTemplateVersionsByTemplateIDParams{
			TemplateID: template.ID,
			AfterID:    paginationParams.AfterID,
			LimitOpt:   int32(paginationParams.Limit),
			OffsetOpt:  int32(paginationParams.Offset),
		})
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(rw, http.StatusOK, apiVersions)
			return err
		}
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching template versions.",
				Detail:  err.Error(),
			})
			return err
		}

		jobIDs := make([]uuid.UUID, 0, len(versions))
		for _, version := range versions {
			jobIDs = append(jobIDs, version.JobID)
		}
		jobs, err := store.GetProvisionerJobsByIDs(r.Context(), jobIDs)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
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
				httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
					Message: fmt.Sprintf("Job %q doesn't exist for version %q.", version.JobID, version.ID),
				})
				return err
			}
			apiVersions = append(apiVersions, convertTemplateVersion(version, convertProvisionerJob(job)))
		}

		return nil
	})
	if err != nil {
		return
	}

	httpapi.Write(rw, http.StatusOK, apiVersions)
}

func (api *API) templateVersionByName(rw http.ResponseWriter, r *http.Request) {
	template := httpmw.TemplateParam(r)
	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	templateVersionName := chi.URLParam(r, "templateversionname")
	templateVersion, err := api.Database.GetTemplateVersionByTemplateIDAndName(r.Context(), database.GetTemplateVersionByTemplateIDAndNameParams{
		TemplateID: uuid.NullUUID{
			UUID:  template.ID,
			Valid: true,
		},
		Name: templateVersionName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("No template version found by name %q.", templateVersionName),
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version.",
			Detail:  err.Error(),
		})
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertTemplateVersion(templateVersion, convertProvisionerJob(job)))
}

func (api *API) patchActiveTemplateVersion(rw http.ResponseWriter, r *http.Request) {
	template := httpmw.TemplateParam(r)
	if !api.Authorize(r, rbac.ActionUpdate, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.UpdateActiveTemplateVersion
	if !httpapi.Read(rw, r, &req) {
		return
	}
	version, err := api.Database.GetTemplateVersionByID(r.Context(), req.ID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, codersdk.Response{
			Message: "Template version not found.",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version.",
			Detail:  err.Error(),
		})
		return
	}
	if version.TemplateID.UUID.String() != template.ID.String() {
		httpapi.Write(rw, http.StatusUnauthorized, codersdk.Response{
			Message: "The provided template version doesn't belong to the specified template.",
		})
		return
	}

	err = api.Database.InTx(func(store database.Store) error {
		err = store.UpdateTemplateActiveVersionByID(r.Context(), database.UpdateTemplateActiveVersionByIDParams{
			ID:              template.ID,
			ActiveVersionID: req.ID,
			UpdatedAt:       database.Now(),
		})
		if err != nil {
			return xerrors.Errorf("update active version: %w", err)
		}
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating active template version.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, codersdk.Response{
		Message: "Updated the active template version!",
	})
}

// Creates a new version of a template. An import job is queued to parse the storage method provided.
func (api *API) postTemplateVersionsByOrganization(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	organization := httpmw.OrganizationParam(r)
	var req codersdk.CreateTemplateVersionRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}

	if req.TemplateID != uuid.Nil {
		_, err := api.Database.GetTemplateByID(r.Context(), req.TemplateID)
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(rw, http.StatusNotFound, codersdk.Response{
				Message: "Template does not exist.",
			})
			return
		}
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching template.",
				Detail:  err.Error(),
			})
			return
		}
	}

	file, err := api.Database.GetFileByHash(r.Context(), req.StorageSource)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, codersdk.Response{
			Message: "File not found.",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching file.",
			Detail:  err.Error(),
		})
		return
	}

	// Making a new template version is the same permission as creating a new template.
	if !api.Authorize(r, rbac.ActionCreate, rbac.ResourceTemplate.InOrg(organization.ID)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	if !api.Authorize(r, rbac.ActionRead, file) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var templateVersion database.TemplateVersion
	var provisionerJob database.ProvisionerJob
	err = api.Database.InTx(func(db database.Store) error {
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

			inheritedParams, err := db.ParameterValues(r.Context(), database.ParameterValuesParams{
				Ids: inherits,
			})
			if err != nil {
				return xerrors.Errorf("fetch inherited params: %w", err)
			}
			for _, copy := range inheritedParams {
				// This is a bit inefficient, as we make a new db call for each
				// param.
				version, err := db.GetTemplateVersionByJobID(r.Context(), copy.ScopeID)
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

			_, err = db.InsertParameterValue(r.Context(), database.InsertParameterValueParams{
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

		provisionerJob, err = db.InsertProvisionerJob(r.Context(), database.InsertProvisionerJobParams{
			ID:             jobID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			OrganizationID: organization.ID,
			InitiatorID:    apiKey.UserID,
			Provisioner:    database.ProvisionerType(req.Provisioner),
			StorageMethod:  database.ProvisionerStorageMethodFile,
			StorageSource:  file.Hash,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          []byte{'{', '}'},
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

		templateVersion, err = db.InsertTemplateVersion(r.Context(), database.InsertTemplateVersionParams{
			ID:             uuid.New(),
			TemplateID:     templateID,
			OrganizationID: organization.ID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			Name:           namesgenerator.GetRandomName(1),
			Readme:         "",
			JobID:          provisionerJob.ID,
		})
		if err != nil {
			return xerrors.Errorf("insert template version: %w", err)
		}
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, convertTemplateVersion(templateVersion, convertProvisionerJob(provisionerJob)))
}

// templateVersionResources returns the workspace agent resources associated
// with a template version. A template can specify more than one resource to be
// provisioned, each resource can have an agent that dials back to coderd.
// The agents returned are informative of the template version, and do not
// return agents associated with any particular workspace.
func (api *API) templateVersionResources(rw http.ResponseWriter, r *http.Request) {
	templateVersion := httpmw.TemplateVersionParam(r)
	if !api.Authorize(r, rbac.ActionRead, templateVersion) {
		httpapi.ResourceNotFound(rw)
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
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
func (api *API) templateVersionLogs(rw http.ResponseWriter, r *http.Request) {
	templateVersion := httpmw.TemplateVersionParam(r)
	if !api.Authorize(r, rbac.ActionRead, templateVersion) {
		httpapi.ResourceNotFound(rw)
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	api.provisionerJobLogs(rw, r, job)
}

func convertTemplateVersion(version database.TemplateVersion, job codersdk.ProvisionerJob) codersdk.TemplateVersion {
	return codersdk.TemplateVersion{
		ID:             version.ID,
		TemplateID:     &version.TemplateID.UUID,
		OrganizationID: version.OrganizationID,
		CreatedAt:      version.CreatedAt,
		UpdatedAt:      version.UpdatedAt,
		Name:           version.Name,
		Job:            job,
		Readme:         version.Readme,
	}
}
