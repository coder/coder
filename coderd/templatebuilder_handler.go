package coderd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/templatebuilder"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/util/namesgenerator"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/examples"
	"github.com/coder/coder/v2/provisionersdk"
)

// @Summary List template builder base templates
// @ID list-template-builder-base-templates
// @Security CoderSessionToken
// @Produce json
// @Tags TemplateBuilder
// @Success 200 {object} codersdk.TemplateBuilderBasesResponse
// @Router /api/v2/templatebuilder/bases [get]
func (api *API) templateBuilderBases(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, policy.ActionRead, rbac.ResourceTemplate.AnyOrganization()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	exampleList, err := examples.List()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error listing examples.",
			Detail:  err.Error(),
		})
		return
	}

	examplesByID := make(map[string]codersdk.TemplateExample, len(exampleList))
	for _, ex := range exampleList {
		examplesByID[ex.ID] = ex
	}

	bases := make([]codersdk.TemplateBuilderBase, 0, len(templatebuilder.BaseTemplateIDs()))
	for _, id := range templatebuilder.BaseTemplateIDs() {
		ex, ok := examplesByID[id]
		if !ok {
			api.Logger.Warn(ctx, "base template has no matching example",
				slog.F("base_template_id", id))
			continue
		}
		vars := baseVariablesToSDK(templatebuilder.BaseVariables(id))
		bases = append(bases, codersdk.TemplateBuilderBase{
			ID:            ex.ID,
			Name:          ex.Name,
			Description:   ex.Description,
			Icon:          ex.Icon,
			OS:            string(templatebuilder.BaseTemplateOS(id)),
			Variables:     vars,
			Prerequisites: templatebuilder.BasePrerequisites(id),
		})
	}

	sort.Slice(bases, func(i, j int) bool {
		return bases[i].Name < bases[j].Name
	})

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.TemplateBuilderBasesResponse{
		Bases: bases,
	})
}

// baseVariablesToSDK converts base template variables to the SDK type,
// filtering out computed variables that the builder wires automatically.
func baseVariablesToSDK(vars []templatebuilder.ModuleVariable) []codersdk.TemplateBuilderModuleVariable {
	out := make([]codersdk.TemplateBuilderModuleVariable, 0, len(vars))
	for _, v := range vars {
		if v.Computed {
			continue
		}
		out = append(out, codersdk.TemplateBuilderModuleVariable{
			Name:        v.Name,
			Type:        codersdk.TemplateBuilderVariableType(v.Type),
			Description: v.Description,
			Default:     v.Default,
			Required:    v.Required,
			Sensitive:   v.Sensitive,
		})
	}
	return out
}

// @Summary List template builder modules
// @ID list-template-builder-modules
// @Security CoderSessionToken
// @Produce json
// @Tags TemplateBuilder
// @Param base query string false "Base template example ID for OS-compatibility filtering"
// @Success 200 {object} codersdk.TemplateBuilderModulesResponse
// @Router /api/v2/templatebuilder/modules [get]
func (api *API) templateBuilderModules(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, policy.ActionRead, rbac.ResourceTemplate.AnyOrganization()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	manifests, err := templatebuilder.LoadModules()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error loading module catalog.",
			Detail:  err.Error(),
		})
		return
	}

	// Resolve OS filter from the base query param.
	var filterOS templatebuilder.BaseOS
	if base := r.URL.Query().Get("base"); base != "" {
		filterOS = templatebuilder.BaseTemplateOS(base)
		if filterOS == "" {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Unknown base template.",
				Detail:  "The \"base\" query parameter must be a valid base template ID.",
			})
			return
		}
	}

	modules := make([]codersdk.TemplateBuilderModule, 0, len(manifests))
	for _, m := range manifests {
		if filterOS != "" && !m.CompatibleWithOS(string(filterOS)) {
			continue
		}
		modules = append(modules, m.ToSDK())
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.TemplateBuilderModulesResponse{
		Modules: modules,
	})
}

// @Summary Compose template from base and modules
// @ID compose-template-from-base-and-modules
// @Security CoderSessionToken
// @Accept json
// @Produce application/x-tar
// @Tags TemplateBuilder
// @Param request body codersdk.TemplateBuilderComposeRequest true "Compose request"
// @Success 200
// @Router /api/v2/templatebuilder/compose [post]
func (api *API) templateBuilderCompose(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceTemplate.AnyOrganization()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.TemplateBuilderComposeRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.BaseTemplateID == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing base_template_id.",
		})
		return
	}

	composeReq := templatebuilder.ComposeRequest{
		BaseTemplateID:     req.BaseTemplateID,
		BaseVariableValues: req.BaseVariableValues,
		RegistryURL:        api.DeploymentValues.TemplateBuilder.RegistryURL.String(),
	}
	for _, m := range req.Modules {
		composeReq.Modules = append(composeReq.Modules, templatebuilder.ComposeModule{
			ID:        m.ID,
			Variables: m.Variables,
		})
	}

	result, err := templatebuilder.Compose(composeReq)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to compose template.",
			Detail:  err.Error(),
		})
		return
	}

	tarData, err := templatebuilder.BundleTar(result)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error bundling template.",
			Detail:  err.Error(),
		})
		return
	}

	rw.Header().Set("Content-Type", "application/x-tar")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(tarData)
}

// templateBuilderCreateTemplateTimeout is the maximum time the handler waits
// for the provisioner import job to complete.
const templateBuilderCreateTemplateTimeout = 2 * time.Minute

// @Summary Compose and create a template
// @ID compose-and-create-a-template
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags TemplateBuilder
// @Param request body codersdk.TemplateBuilderCreateTemplateRequest true "Create template request"
// @Success 201 {object} codersdk.TemplateBuilderCreateTemplateResponse
// @Failure 400 {object} codersdk.Response
// @Failure 404 {object} codersdk.Response
// @Failure 409 {object} codersdk.Response
// @Failure 504 {object} codersdk.Response
// @Router /api/v2/templatebuilder/compose/template [post]
func (api *API) templateBuilderCreateTemplate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	var req codersdk.TemplateBuilderCreateTemplateRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.BaseTemplateID == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing base_template_id.",
		})
		return
	}

	// Resolve and authorize against the organization.
	organization, err := api.Database.GetOrganizationByID(ctx, req.OrganizationID)
	if httpapi.Is404Error(err) {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Organization not found.",
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching organization.",
			Detail:  err.Error(),
		})
		return
	}

	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceTemplate.InOrg(organization.ID)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Check template name uniqueness early.
	_, err = api.Database.GetTemplateByOrganizationAndName(ctx, database.GetTemplateByOrganizationAndNameParams{
		OrganizationID: organization.ID,
		Name:           req.Name,
		Deleted:        false,
	})
	if err == nil {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "A template with this name already exists in the organization.",
		})
		return
	}
	if !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error checking template name.",
			Detail:  err.Error(),
		})
		return
	}

	// Compose the template.
	composeReq := templatebuilder.ComposeRequest{
		BaseTemplateID:     req.BaseTemplateID,
		BaseVariableValues: req.BaseVariableValues,
		RegistryURL:        api.DeploymentValues.TemplateBuilder.RegistryURL.String(),
	}
	for _, m := range req.Modules {
		composeReq.Modules = append(composeReq.Modules, templatebuilder.ComposeModule{
			ID:        m.ID,
			Variables: m.Variables,
		})
	}

	result, err := templatebuilder.Compose(composeReq)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to compose template.",
			Detail:  err.Error(),
		})
		return
	}

	tarData, err := templatebuilder.BundleTar(result)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error bundling template.",
			Detail:  err.Error(),
		})
		return
	}

	// Insert the tar as a file with hash-based dedup.
	hashBytes := sha256.Sum256(tarData)
	hash := hex.EncodeToString(hashBytes[:])

	file, err := api.Database.GetFileByHashAndCreator(ctx, database.GetFileByHashAndCreatorParams{
		Hash:      hash,
		CreatedBy: apiKey.UserID,
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error checking file.",
			Detail:  err.Error(),
		})
		return
	}
	if xerrors.Is(err, sql.ErrNoRows) {
		file, err = api.Database.InsertFile(ctx, database.InsertFileParams{
			ID:        uuid.New(),
			Hash:      hash,
			CreatedAt: dbtime.Now(),
			CreatedBy: apiKey.UserID,
			Mimetype:  codersdk.ContentTypeTar,
			Data:      tarData,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error saving file.",
				Detail:  err.Error(),
			})
			return
		}
	}

	tags := provisionersdk.MutateTags(apiKey.UserID, nil, req.ProvisionerTags)
	traceMetadataRaw, err := json.Marshal(tracing.MetadataFromContext(ctx))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error marshaling trace metadata.",
			Detail:  err.Error(),
		})
		return
	}

	// Create template version and provisioner import job.
	var (
		provisionerJob  database.ProvisionerJob
		templateVersion database.TemplateVersion
	)
	err = api.Database.InTx(func(tx database.Store) error {
		jobID := uuid.New()
		templateVersionID := uuid.New()

		jobInput, err := json.Marshal(provisionerdserver.TemplateVersionImportJob{
			TemplateVersionID: templateVersionID,
		})
		if err != nil {
			return xerrors.Errorf("marshal job input: %w", err)
		}

		provisionerJob, err = tx.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:             jobID,
			CreatedAt:      dbtime.Now(),
			UpdatedAt:      dbtime.Now(),
			OrganizationID: organization.ID,
			InitiatorID:    apiKey.UserID,
			Provisioner:    database.ProvisionerTypeTerraform,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          jobInput,
			Tags:           tags,
			TraceMetadata: pqtype.NullRawMessage{
				Valid:      true,
				RawMessage: traceMetadataRaw,
			},
			LogsOverflowed: false,
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}

		versionName := namesgenerator.NameDigitWith("_")
		err = tx.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
			ID:              templateVersionID,
			TemplateID:      uuid.NullUUID{},
			OrganizationID:  organization.ID,
			CreatedAt:       dbtime.Now(),
			UpdatedAt:       dbtime.Now(),
			Name:            versionName,
			Message:         "",
			Readme:          string(result.Readme),
			JobID:           provisionerJob.ID,
			CreatedBy:       apiKey.UserID,
			SourceExampleID: sql.NullString{},
		})
		if err != nil {
			return xerrors.Errorf("insert template version: %w", err)
		}

		templateVersion, err = tx.GetTemplateVersionByID(ctx, templateVersionID)
		if err != nil {
			return xerrors.Errorf("get template version: %w", err)
		}

		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating template version.",
			Detail:  err.Error(),
		})
		return
	}

	// Notify provisioner of the new job.
	err = provisionerjobs.PostJob(api.Pubsub, provisionerJob)
	if err != nil {
		api.Logger.Error(ctx, "failed to post provisioner job",
			slog.F("job_id", provisionerJob.ID),
			slog.Error(err))
	}

	// Wait for the import job to complete.
	jobCtx, jobCancel := context.WithTimeout(ctx, templateBuilderCreateTemplateTimeout)
	defer jobCancel()

	completedJob, err := api.waitForProvisionerJob(jobCtx, provisionerJob.ID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			httpapi.Write(ctx, rw, http.StatusGatewayTimeout, codersdk.Response{
				Message: "Timed out waiting for template import to complete.",
				Detail:  "The template version is still being imported. You can check its status manually.",
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error waiting for template import.",
			Detail:  err.Error(),
		})
		return
	}

	// Check if the job was canceled.
	if completedJob.CanceledAt.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Template import was canceled.",
		})
		return
	}

	// Check if the job failed.
	if completedJob.Error.Valid {
		// Fetch logs to help classify the error.
		jobLogs, logErr := api.Database.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
			JobID:        completedJob.ID,
			CreatedAfter: 0,
		})
		var logLines []string
		if logErr == nil {
			for _, l := range jobLogs {
				logLines = append(logLines, l.Output)
			}
		}

		classified := templatebuilder.ClassifyProvisionerError(completedJob.Error.String, logLines)
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Template import failed.",
			Detail:  classified,
		})
		return
	}

	// Audit logging for template and template version creation.
	var (
		auditor                            = *api.Auditor.Load()
		templateAudit, commitTemplateAudit = audit.InitRequest[database.Template](rw, &audit.RequestParams{
			Audit:          auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionCreate,
			OrganizationID: organization.ID,
		})
		templateVersionAudit, commitTemplateVersionAudit = audit.InitRequest[database.TemplateVersion](rw, &audit.RequestParams{
			Audit:          auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: organization.ID,
		})
	)
	defer commitTemplateAudit()
	defer commitTemplateVersionAudit()

	// Import succeeded. Create the template.
	defaultGroups := database.TemplateACL{
		organization.ID.String(): db2sdk.TemplateRoleActions(codersdk.TemplateRoleUse),
	}

	var dbTemplate database.Template
	err = api.Database.InTx(func(tx database.Store) error {
		now := dbtime.Now()
		templateID := uuid.New()

		err = tx.InsertTemplate(ctx, database.InsertTemplateParams{
			ID:                           templateID,
			CreatedAt:                    now,
			UpdatedAt:                    now,
			OrganizationID:               organization.ID,
			Name:                         req.Name,
			Provisioner:                  database.ProvisionerTypeTerraform,
			ActiveVersionID:              templateVersion.ID,
			Description:                  req.Description,
			CreatedBy:                    apiKey.UserID,
			UserACL:                      database.TemplateACL{},
			GroupACL:                     defaultGroups,
			DisplayName:                  req.DisplayName,
			Icon:                         req.Icon,
			AllowUserCancelWorkspaceJobs: false,
			MaxPortSharingLevel:          database.AppSharingLevelOwner,
			UseClassicParameterFlow:      false,
			CorsBehavior:                 database.CorsBehaviorSimple,
		})
		if err != nil {
			if database.IsUniqueViolation(err, database.UniqueTemplatesOrganizationIDNameIndex) {
				httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
					Message: "A template with this name already exists in the organization.",
				})
				return nil
			}
			return xerrors.Errorf("insert template: %w", err)
		}

		dbTemplate, err = tx.GetTemplateByID(ctx, templateID)
		if err != nil {
			return xerrors.Errorf("get template: %w", err)
		}

		dbTemplate, err = (*api.TemplateScheduleStore.Load()).Set(ctx, tx, dbTemplate, schedule.TemplateScheduleOptions{
			UserAutostartEnabled: true,
			UserAutostopEnabled:  true,
		})
		if err != nil {
			return xerrors.Errorf("set template schedule options: %w", err)
		}

		err = tx.UpdateTemplateVersionByID(ctx, database.UpdateTemplateVersionByIDParams{
			ID: templateVersion.ID,
			TemplateID: uuid.NullUUID{
				UUID:  dbTemplate.ID,
				Valid: true,
			},
			UpdatedAt: dbtime.Now(),
			Name:      templateVersion.Name,
			Message:   templateVersion.Message,
		})
		if err != nil {
			return xerrors.Errorf("link template version to template: %w", err)
		}

		templateAudit.New = dbTemplate
		newTemplateVersion := templateVersion
		newTemplateVersion.TemplateID = uuid.NullUUID{
			UUID:  dbTemplate.ID,
			Valid: true,
		}
		templateVersionAudit.New = newTemplateVersion

		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating template.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.TemplateBuilderCreateTemplateResponse{
		Template: api.convertTemplate(dbTemplate),
	})
}

// waitForProvisionerJob polls until the job completes or the context expires.
func (api *API) waitForProvisionerJob(
	ctx context.Context,
	jobID uuid.UUID,
) (database.ProvisionerJob, error) {
	initialIntervals := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		500 * time.Millisecond,
	}
	const steadyInterval = time.Second

	for i := 0; ; i++ {
		var delay time.Duration
		if i < len(initialIntervals) {
			delay = initialIntervals[i]
		} else {
			delay = steadyInterval
		}

		select {
		case <-ctx.Done():
			return database.ProvisionerJob{}, ctx.Err()
		case <-time.After(delay):
		}

		job, err := api.Database.GetProvisionerJobByID(ctx, jobID)
		if err != nil {
			return database.ProvisionerJob{}, xerrors.Errorf("get provisioner job: %w", err)
		}

		if job.CompletedAt.Valid {
			return job, nil
		}
	}
}
