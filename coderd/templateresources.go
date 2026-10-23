package coderd

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

type templateResourceHTTPError struct {
	status  int
	message string
}

func (e *templateResourceHTTPError) Error() string { return e.message }
func templateResourceConflict(message string) error {
	return &templateResourceHTTPError{http.StatusConflict, message}
}

func (api *API) templateResourceError(rw http.ResponseWriter, r *http.Request, err error) {
	status, message := http.StatusInternalServerError, "Internal error processing template resources."
	var requestErr *templateResourceHTTPError
	switch {
	case errors.As(err, &requestErr):
		status, message = requestErr.status, requestErr.message
	case errors.Is(err, sql.ErrNoRows), dbauthz.IsNotAuthorizedError(err):
		status, message = http.StatusNotFound, "Template resource not found."
	default:
		api.Logger.Error(r.Context(), "template resources request", slog.Error(err))
	}
	httpapi.Write(r.Context(), rw, status, codersdk.Response{Message: message})
}

func (api *API) templateResourceAuthorized(rw http.ResponseWriter, r *http.Request) bool {
	if !api.Authorize(r, policy.ActionUpdate, httpmw.TemplateParam(r).RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return false
	}
	return true
}

// @Summary Get template resources
// @ID get-template-resources
// @Security CoderSessionToken
// @Tags Templates
// @Produce json
// @Param template path string true "Template ID" format(uuid)
// @Success 200 {object} codersdk.TemplateResources
// @Router /api/v2/templates/{template}/resources [get]
// @x-apidocgen {"skip": true}
func (api *API) templateResources(rw http.ResponseWriter, r *http.Request) {
	if !api.templateResourceAuthorized(rw, r) {
		return
	}
	template := httpmw.TemplateParam(r)
	result := codersdk.TemplateResources{TemplateID: template.ID, Status: "unconfigured", Revisions: []codersdk.TemplateResourceRevision{}, Runs: []codersdk.TemplateResourceRun{}, Outputs: map[string]interface{}{}}
	err := api.Database.InTx(func(tx database.Store) error {
		state, err := tx.GetTemplateResourceState(r.Context(), template.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		result.Status = state.Status
		if state.ActiveRunID.Valid && state.Status != "unknown" {
			result.Status = "pending"
		}
		result.CurrentRevisionID = templateResourceUUID(state.CurrentRevisionID)
		result.AppliedRevisionID = templateResourceUUID(state.AppliedRevisionID)
		if err := json.Unmarshal(state.Outputs, &result.Outputs); err != nil {
			return err
		}
		revisions, err := tx.GetTemplateResourceRevisionsByTemplateID(r.Context(), template.ID)
		if err != nil {
			return err
		}
		for _, revision := range revisions {
			result.Revisions = append(result.Revisions, convertTemplateResourceRevision(revision))
		}
		runs, err := tx.GetTemplateResourceRunsByTemplateID(r.Context(), template.ID)
		if err != nil {
			return err
		}
		for _, run := range runs {
			job, err := tx.GetProvisionerJobByID(r.Context(), run.JobID)
			if err != nil {
				return err
			}
			result.Runs = append(result.Runs, convertTemplateResourceRun(run, job))
		}
		return nil
	}, &database.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, result)
}

// templateResourceArchive keeps a complete immutable root for eventual destroy.
func templateResourceArchive(source string) ([]byte, error) {
	if strings.TrimSpace(source) == "" || len(source) > 1<<20 {
		return nil, &templateResourceHTTPError{http.StatusBadRequest, "Source must contain between 1 byte and 1 MiB of Terraform configuration."}
	}
	if err := validateTemplateResourceSource(source); err != nil {
		return nil, err
	}
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	if err := tw.WriteHeader(&tar.Header{Name: "main.tf", Mode: 0o600, Size: int64(len(source))}); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(source)); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return archive.Bytes(), nil
}

// @Summary Create template resource revision
// @ID create-template-resource-revision
// @Security CoderSessionToken
// @Tags Templates
// @Accept json
// @Produce json
// @Param template path string true "Template ID" format(uuid)
// @Param request body codersdk.CreateTemplateResourceRevisionRequest true "Source archive or inline main.tf"
// @Success 201 {object} codersdk.TemplateResourceRevision
// @Router /api/v2/templates/{template}/resources/versions [post]
// @x-apidocgen {"skip": true}
func (api *API) postTemplateResourceRevision(rw http.ResponseWriter, r *http.Request) {
	if !api.templateResourceAuthorized(rw, r) {
		return
	}
	var req codersdk.CreateTemplateResourceRevisionRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	if req.FileID != uuid.Nil && req.Source != "" {
		api.templateResourceError(rw, r, &templateResourceHTTPError{http.StatusBadRequest, "Provide file_id or source, not both."})
		return
	}
	var archive []byte
	var err error
	source := req.Source
	if req.FileID == uuid.Nil {
		archive, err = templateResourceArchive(source)
	} else {
		var file database.File
		file, err = api.Database.GetFileByID(r.Context(), req.FileID)
		if err == nil {
			if file.Mimetype != tarMimeType {
				err = &templateResourceHTTPError{http.StatusBadRequest, "File must be a TAR archive."}
			} else {
				archive, source, err = normalizeTemplateResourceArchive(file.Data, HTTPFileMaxBytes)
			}
		}
	}
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	template, userID, ctx := httpmw.TemplateParam(r), httpmw.APIKey(r).UserID, r.Context()
	var revision database.TemplateResourceRevision
	err = api.Database.InTx(func(tx database.Store) error {
		if err := tx.LockTemplateResourceTemplate(ctx, template.ID); err != nil {
			return err
		}
		fresh, err := tx.GetTemplateByID(ctx, template.ID)
		if err != nil {
			return err
		}
		if fresh.Deleted {
			return sql.ErrNoRows
		}
		now := dbtime.Now()
		if _, err := tx.InsertTemplateResourceState(ctx, database.InsertTemplateResourceStateParams{TemplateID: template.ID, OrganizationID: template.OrganizationID, UpdatedAt: now}); err != nil {
			return err
		}
		hash := sha256.Sum256(archive)
		file, err := tx.GetFileByHashAndCreator(ctx, database.GetFileByHashAndCreatorParams{Hash: hex.EncodeToString(hash[:]), CreatedBy: userID})
		if errors.Is(err, sql.ErrNoRows) {
			file, err = tx.InsertFile(ctx, database.InsertFileParams{ID: uuid.New(), Hash: hex.EncodeToString(hash[:]), CreatedBy: userID, CreatedAt: now, Mimetype: tarMimeType, Data: archive})
		}
		if err != nil {
			return err
		}
		revision, err = tx.InsertTemplateResourceRevision(ctx, database.InsertTemplateResourceRevisionParams{ID: uuid.New(), TemplateID: template.ID, OrganizationID: template.OrganizationID, FileID: file.ID, Source: source, CreatedBy: userID, CreatedAt: now})
		if err != nil {
			return err
		}
		_, err = tx.SetTemplateResourceCurrentRevision(ctx, database.SetTemplateResourceCurrentRevisionParams{TemplateID: template.ID, CurrentRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, UpdatedAt: now})
		return err
	}, nil)
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusCreated, convertTemplateResourceRevision(revision))
}

// @Summary Run template resources
// @ID create-template-resource-run
// @Security CoderSessionToken
// @Tags Templates
// @Accept json
// @Produce json
// @Param template path string true "Template ID" format(uuid)
// @Param request body codersdk.CreateTemplateResourceRunRequest true "Run"
// @Success 201 {object} codersdk.TemplateResourceRun
// @Router /api/v2/templates/{template}/resources/runs [post]
// @x-apidocgen {"skip": true}
func (api *API) postTemplateResourceRun(rw http.ResponseWriter, r *http.Request) {
	if !api.templateResourceAuthorized(rw, r) {
		return
	}
	var req codersdk.CreateTemplateResourceRunRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	if req.Action != "plan" && req.Action != "apply" && req.Action != "destroy" {
		api.templateResourceError(rw, r, &templateResourceHTTPError{http.StatusBadRequest, "Action must be plan, apply, or destroy."})
		return
	}
	ctx, template, userID := r.Context(), httpmw.TemplateParam(r), httpmw.APIKey(r).UserID
	var run database.TemplateResourceRun
	var job database.ProvisionerJob
	err := api.Database.InTx(func(tx database.Store) error {
		if err := tx.LockTemplateResourceTemplate(ctx, template.ID); err != nil {
			return err
		}
		fresh, err := tx.GetTemplateByID(ctx, template.ID)
		if err != nil {
			return err
		}
		if fresh.Deleted {
			return sql.ErrNoRows
		}
		state, err := tx.GetTemplateResourceStateForUpdate(ctx, template.ID)
		if err != nil {
			return err
		}
		if state.Status == "unknown" {
			return templateResourceConflict("The previous mutation has an unknown outcome. Manual reconciliation is required; automatic recovery is not supported.")
		}
		if state.ActiveRunID.Valid {
			return templateResourceConflict("A template resource run is already active.")
		}
		revision, err := tx.GetTemplateResourceRevisionByID(ctx, req.RevisionID)
		if err != nil {
			return err
		}
		if revision.TemplateID != template.ID || revision.OrganizationID != template.OrganizationID {
			return sql.ErrNoRows
		}
		// The template ACL was checked above. Count all workspaces, not just those visible to the caller.
		//nolint:gocritic
		systemCtx := dbauthz.AsSystemRestricted(ctx)
		if req.Action == "destroy" {
			if !state.AppliedRevisionID.Valid || state.AppliedRevisionID.UUID != revision.ID {
				return templateResourceConflict("Destroy must use the state-producing revision.")
			}
			workspaces, err := tx.GetWorkspacesByTemplateID(systemCtx, template.ID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if len(workspaces) != 0 {
				return templateResourceConflict("Delete all workspaces before destroying template resources.")
			}
		}
		runID, now := uuid.New(), dbtime.Now()
		input, err := json.Marshal(struct {
			RunID uuid.UUID `json:"template_resource_run_id"`
		}{runID})
		if err != nil {
			return err
		}
		tags := provisionersdk.MutateTags(uuid.Nil, nil)
		tags[provisionersdk.TagTemplateResources] = provisionersdk.TemplateResourcesCapabilityVersion
		job, err = tx.InsertProvisionerJob(systemCtx, database.InsertProvisionerJobParams{ID: uuid.New(), OrganizationID: template.OrganizationID, CreatedAt: now, UpdatedAt: now, InitiatorID: userID, Provisioner: database.ProvisionerTypeTerraform, Type: database.ProvisionerJobTypeTemplateResources, StorageMethod: database.ProvisionerStorageMethodFile, FileID: revision.FileID, Input: input, Tags: tags, TraceMetadata: pqtype.NullRawMessage{}, LogsOverflowed: false})
		if err != nil {
			return err
		}
		run, err = tx.InsertTemplateResourceRun(ctx, database.InsertTemplateResourceRunParams{ID: runID, TemplateID: template.ID, OrganizationID: template.OrganizationID, RevisionID: revision.ID, JobID: job.ID, Action: req.Action, CreatedAt: now})
		if err != nil {
			return err
		}
		_, err = tx.StartTemplateResourceRun(ctx, database.StartTemplateResourceRunParams{TemplateID: template.ID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, UpdatedAt: now})
		return err
	}, nil)
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	if err := provisionerjobs.PostJob(api.Pubsub, job); err != nil {
		api.Logger.Warn(ctx, "publish template resource job", slog.Error(err))
	}
	httpapi.Write(ctx, rw, http.StatusCreated, convertTemplateResourceRun(run, job))
}

// @Summary Cancel template resource run
// @ID cancel-template-resource-run
// @Security CoderSessionToken
// @Tags Templates
// @Produce json
// @Param template path string true "Template ID" format(uuid)
// @Param runID path string true "Run ID" format(uuid)
// @Success 200 {object} codersdk.Response
// @Router /api/v2/templates/{template}/resources/runs/{runID}/cancel [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchCancelTemplateResourceRun(rw http.ResponseWriter, r *http.Request) {
	if !api.templateResourceAuthorized(rw, r) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		api.templateResourceError(rw, r, sql.ErrNoRows)
		return
	}
	ctx, template := r.Context(), httpmw.TemplateParam(r)
	var completedJobID uuid.UUID
	err = api.Database.InTx(func(tx database.Store) error {
		run, err := tx.GetTemplateResourceRunByID(ctx, id)
		if err != nil {
			return err
		}
		if run.TemplateID != template.ID || run.OrganizationID != template.OrganizationID {
			return sql.ErrNoRows
		}
		// Match worker finalization's lock order and fence acquisition.
		job, err := tx.GetProvisionerJobByIDWithLock(ctx, run.JobID)
		if err != nil {
			return err
		}
		if job.CompletedAt.Valid {
			return templateResourceConflict("The template resource run is already complete.")
		}
		if job.CanceledAt.Valid {
			return nil
		}
		state, err := tx.GetTemplateResourceStateForUpdate(ctx, template.ID)
		if err != nil {
			return err
		}
		if !state.ActiveRunID.Valid || state.ActiveRunID.UUID != run.ID {
			return templateResourceConflict("The template resource run reservation is stale.")
		}
		now := sql.NullTime{Time: dbtime.Now(), Valid: true}
		var completedAt sql.NullTime
		if !job.StartedAt.Valid && !job.WorkerID.Valid {
			// A pending job cannot have mutated infrastructure. Acquired jobs
			// retain their reservation until the worker or reaper finalizes.
			//nolint:gocritic // Template update ACL was checked before releasing state.
			systemCtx := dbauthz.AsSystemRestricted(ctx)
			_, err = tx.CompleteTemplateResourceRun(systemCtx, database.CompleteTemplateResourceRunParams{
				TemplateID: template.ID, RunID: state.ActiveRunID, Status: state.Status,
				State: state.State, Outputs: state.Outputs, AppliedRevisionID: state.AppliedRevisionID, UpdatedAt: now.Time,
			})
			if err != nil {
				return err
			}
			completedAt, completedJobID = now, job.ID
		}
		return tx.UpdateProvisionerJobWithCancelByID(ctx, database.UpdateProvisionerJobWithCancelByIDParams{
			ID: job.ID, CanceledAt: now, CompletedAt: completedAt,
		})
	}, nil)
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	if completedJobID != uuid.Nil {
		data, _ := json.Marshal(provisionersdk.ProvisionerJobLogsNotifyMessage{EndOfLogs: true})
		if err := api.Pubsub.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(completedJobID), data); err != nil {
			api.Logger.Warn(ctx, "publish canceled template resource job", slog.Error(err))
		}
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{Message: "Template resource run cancellation requested."})
}

// @Summary Get template resource run logs
// @ID get-template-resource-run-logs
// @Security CoderSessionToken
// @Tags Templates
// @Produce json
// @Param template path string true "Template ID" format(uuid)
// @Param runID path string true "Run ID" format(uuid)
// @Param after query integer false "Return logs after this ID"
// @Success 200 {array} codersdk.ProvisionerJobLog
// @Router /api/v2/templates/{template}/resources/runs/{runID}/logs [get]
// @x-apidocgen {"skip": true}
func (api *API) templateResourceRunLogs(rw http.ResponseWriter, r *http.Request) {
	if !api.templateResourceAuthorized(rw, r) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		api.templateResourceError(rw, r, sql.ErrNoRows)
		return
	}
	run, err := api.Database.GetTemplateResourceRunByID(r.Context(), id)
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	template := httpmw.TemplateParam(r)
	if run.TemplateID != template.ID || run.OrganizationID != template.OrganizationID {
		api.templateResourceError(rw, r, sql.ErrNoRows)
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), run.JobID)
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	api.provisionerJobLogs(rw, r, job)
}

func templateResourceUUID(id uuid.NullUUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	return &id.UUID
}
func convertTemplateResourceRevision(revision database.TemplateResourceRevision) codersdk.TemplateResourceRevision {
	return codersdk.TemplateResourceRevision{ID: revision.ID, TemplateID: revision.TemplateID, CreatedAt: revision.CreatedAt, FileID: revision.FileID, Source: revision.Source}
}
func convertTemplateResourceRun(run database.TemplateResourceRun, job database.ProvisionerJob) codersdk.TemplateResourceRun {
	return codersdk.TemplateResourceRun{ID: run.ID, RevisionID: run.RevisionID, Action: run.Action, CreatedAt: run.CreatedAt, Job: convertProvisionerJob(database.GetProvisionerJobsByIDsWithQueuePositionRow{ProvisionerJob: job})}
}

// guardTemplateResourceDeletionInTx serializes deletion with source creation and runs.
func guardTemplateResourceDeletionInTx(ctx context.Context, tx database.Store, templateID uuid.UUID) error {
	if err := tx.LockTemplateResourceTemplate(ctx, templateID); err != nil {
		return err
	}
	state, err := tx.GetTemplateResourceStateForUpdate(ctx, templateID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("get template resources: %w", err)
	}
	if state.ActiveRunID.Valid || (state.Status != "unconfigured" && state.Status != "destroyed") || state.AppliedRevisionID.Valid {
		return templateResourceConflict("Destroy template resources before deleting the template. Unknown outcomes require manual reconciliation.")
	}
	return nil
}
