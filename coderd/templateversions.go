package coderd

import (
	"database/sql"
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
	if !api.Authorize(rw, r, rbac.ActionRead, templateVersion) {
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertTemplateVersion(templateVersion, convertProvisionerJob(job)))
}

func (api *API) patchCancelTemplateVersion(rw http.ResponseWriter, r *http.Request) {
	templateVersion := httpmw.TemplateVersionParam(r)
	if !api.Authorize(rw, r, rbac.ActionUpdate, templateVersion) {
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	if job.CompletedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "Job has already completed!",
		})
		return
	}
	if job.CanceledAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
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
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("update provisioner job: %s", err),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, httpapi.Response{
		Message: "Job has been marked as canceled...",
	})
}

func (api *API) templateVersionSchema(rw http.ResponseWriter, r *http.Request) {
	templateVersion := httpmw.TemplateVersionParam(r)
	if !api.Authorize(rw, r, rbac.ActionRead, templateVersion) {
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "Template version job hasn't completed!",
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
	apiSchemas := make([]codersdk.ParameterSchema, 0)
	for _, schema := range schemas {
		apiSchema, err := convertParameterSchema(schema)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("convert: %s", err),
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
	if !api.Authorize(rw, r, rbac.ActionRead, templateVersion) {
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
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
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("compute values: %s", err),
		})
		return
	}
	if values == nil {
		values = []parameter.ComputedValue{}
	}

	httpapi.Write(rw, http.StatusOK, values)
}

func (api *API) templateVersionsByTemplate(rw http.ResponseWriter, r *http.Request) {
	template := httpmw.TemplateParam(r)
	if !api.Authorize(rw, r, rbac.ActionRead, template) {
		return
	}

	paginationParams, ok := parsePagination(rw, r)
	if !ok {
		return
	}

	apiVersion := []codersdk.TemplateVersion{}
	versions, err := api.Database.GetTemplateVersionsByTemplateID(r.Context(), database.GetTemplateVersionsByTemplateIDParams{
		TemplateID: template.ID,
		AfterID:    paginationParams.AfterID,
		LimitOpt:   int32(paginationParams.Limit),
		OffsetOpt:  int32(paginationParams.Offset),
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusOK, apiVersion)
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template version: %s", err),
		})
		return
	}
	jobIDs := make([]uuid.UUID, 0, len(versions))
	for _, version := range versions {
		jobIDs = append(jobIDs, version.JobID)
	}
	jobs, err := api.Database.GetProvisionerJobsByIDs(r.Context(), jobIDs)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get jobs: %s", err),
		})
		return
	}
	jobByID := map[string]database.ProvisionerJob{}
	for _, job := range jobs {
		jobByID[job.ID.String()] = job
	}

	for _, version := range versions {
		job, exists := jobByID[version.JobID.String()]
		if !exists {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("job %q doesn't exist for version %q", version.JobID, version.ID),
			})
			return
		}
		apiVersion = append(apiVersion, convertTemplateVersion(version, convertProvisionerJob(job)))
	}

	httpapi.Write(rw, http.StatusOK, apiVersion)
}

func (api *API) templateVersionByName(rw http.ResponseWriter, r *http.Request) {
	template := httpmw.TemplateParam(r)
	if !api.Authorize(rw, r, rbac.ActionRead, template) {
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
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: fmt.Sprintf("no template version found by name %q", templateVersionName),
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template version by name: %s", err),
		})
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertTemplateVersion(templateVersion, convertProvisionerJob(job)))
}

func (api *API) patchActiveTemplateVersion(rw http.ResponseWriter, r *http.Request) {
	template := httpmw.TemplateParam(r)
	if !api.Authorize(rw, r, rbac.ActionUpdate, template) {
		return
	}

	var req codersdk.UpdateActiveTemplateVersion
	if !httpapi.Read(rw, r, &req) {
		return
	}
	version, err := api.Database.GetTemplateVersionByID(r.Context(), req.ID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "template version not found",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template version: %s", err),
		})
		return
	}
	if version.TemplateID.UUID.String() != template.ID.String() {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "The provided template version doesn't belong to the specified template.",
		})
		return
	}
	err = api.Database.UpdateTemplateActiveVersionByID(r.Context(), database.UpdateTemplateActiveVersionByIDParams{
		ID:              template.ID,
		ActiveVersionID: req.ID,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("update active template version: %s", err),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, httpapi.Response{
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
			httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
				Message: "template does not exist",
			})
			return
		}
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get template: %s", err),
			})
			return
		}
	}

	file, err := api.Database.GetFileByHash(r.Context(), req.StorageSource)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
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

	var templateVersion database.TemplateVersion
	var provisionerJob database.ProvisionerJob
	err = api.Database.InTx(func(db database.Store) error {
		jobID := uuid.New()
		for _, parameterValue := range req.ParameterValues {
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

		provisionerJob, err = api.Database.InsertProvisionerJob(r.Context(), database.InsertProvisionerJobParams{
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

		templateVersion, err = api.Database.InsertTemplateVersion(r.Context(), database.InsertTemplateVersionParams{
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
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
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
	if !api.Authorize(rw, r, rbac.ActionRead, templateVersion) {
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
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
	if !api.Authorize(rw, r, rbac.ActionRead, templateVersion) {
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	api.provisionerJobLogs(rw, r, job)
}

func convertTemplateVersion(version database.TemplateVersion, job codersdk.ProvisionerJob) codersdk.TemplateVersion {
	return codersdk.TemplateVersion{
		ID:         version.ID,
		TemplateID: &version.TemplateID.UUID,
		CreatedAt:  version.CreatedAt,
		UpdatedAt:  version.UpdatedAt,
		Name:       version.Name,
		Job:        job,
		Readme:     version.Readme,
	}
}
