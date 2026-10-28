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
	"github.com/coder/coder/v2/coderd/audit"
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
	status, message := http.StatusInternalServerError, "Internal error processing shared resources."
	var requestErr *templateResourceHTTPError
	switch {
	case errors.As(err, &requestErr):
		status, message = requestErr.status, requestErr.message
	case database.IsUniqueViolation(err):
		status, message = http.StatusConflict, "A Foundation with this name or binding already exists."
	case errors.Is(err, sql.ErrNoRows), dbauthz.IsNotAuthorizedError(err):
		status, message = http.StatusNotFound, "Shared resource not found."
	default:
		api.Logger.Error(r.Context(), "shared resources request", slog.Error(err))
	}
	httpapi.Write(r.Context(), rw, status, codersdk.Response{Message: message})
}

func (api *API) foundationParam(rw http.ResponseWriter, r *http.Request, action policy.Action) (database.Foundation, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "foundation"))
	if err != nil {
		api.templateResourceError(rw, r, sql.ErrNoRows)
		return database.Foundation{}, false
	}
	f, err := api.Database.GetFoundationByID(r.Context(), id)
	if err != nil {
		api.templateResourceError(rw, r, err)
		return f, false
	}
	if f.Deleted || f.OrganizationID != httpmw.OrganizationParam(r).ID || !api.Authorize(r, action, f.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return database.Foundation{}, false
	}
	return f, true
}

func foundationSnapshot(ctx context.Context, tx database.Store, foundation database.Foundation) (codersdk.Foundation, error) {
	result := convertFoundation(foundation)
	// Foundation read access permits aggregate impact counts, not identities
	// or details of workspaces that the caller cannot otherwise access.
	//nolint:gocritic
	consumers, err := tx.GetFoundationConsumerCounts(dbauthz.AsSystemRestricted(ctx), foundation.ID)
	if err != nil {
		return result, err
	}
	result.Consumers = codersdk.FoundationConsumers{Total: consumers.Total, Running: consumers.Running, Stopped: consumers.Stopped, Unknown: consumers.Unknown}
	state, err := tx.GetTemplateResourceState(ctx, foundation.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if state.Status != "destroyed" {
		result.Resources, result.ResourcesAvailable = foundationResources(state.State)
	}
	result.Status = state.Status
	if state.ActiveRunID.Valid && state.Status != "unknown" {
		result.Status = "pending"
	}
	result.CurrentRevisionID = templateResourceUUID(state.CurrentRevisionID)
	result.AppliedRevisionID = templateResourceUUID(state.AppliedRevisionID)
	result.CheckpointRevisionID = templateResourceUUID(state.CheckpointRevisionID)
	if err := foundationPublicOutputs(state.Outputs, &result); err != nil {
		return result, err
	}
	revisions, err := tx.GetTemplateResourceRevisionsByFoundationID(ctx, foundation.ID)
	if err != nil {
		return result, err
	}
	for _, revision := range revisions {
		result.Revisions = append(result.Revisions, convertTemplateResourceRevision(revision))
	}
	runs, err := tx.GetTemplateResourceRunsByFoundationID(ctx, foundation.ID)
	if err != nil {
		return result, err
	}
	for _, run := range runs {
		job, err := tx.GetProvisionerJobByID(ctx, run.JobID)
		if err != nil {
			return result, err
		}
		result.Runs = append(result.Runs, convertTemplateResourceRun(run, job))
	}
	if err := enrichFoundationPreviews(ctx, tx, foundation.ID, result.Runs); err != nil {
		return result, err
	}
	return result, nil
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

// @Summary Create shared resource revision
// @ID create-template-resource-revision
// @Security CoderSessionToken
// @Tags Foundations
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param foundation path string true "Foundation ID" format(uuid)
// @Param request body codersdk.CreateTemplateResourceRevisionRequest true "Source archive or inline main.tf"
// @Success 201 {object} codersdk.TemplateResourceRevision
// @Router /api/v2/organizations/{organization}/foundations/{foundation}/versions [post]
// @x-apidocgen {"skip": true}
func (api *API) postTemplateResourceRevision(rw http.ResponseWriter, r *http.Request) {
	foundation, ok := api.foundationParam(rw, r, policy.ActionUpdate)
	if !ok {
		return
	}
	auditFields := map[string]interface{}{"operation": "save_revision"}
	ar, commit := audit.InitRequest[database.Foundation](rw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, OrganizationID: foundation.OrganizationID, Action: database.AuditActionWrite, AdditionalFields: auditFields})
	defer commit()
	ar.Old, ar.New = foundation, foundation

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
	userID, ctx := httpmw.APIKey(r).UserID, r.Context()
	var revision database.TemplateResourceRevision
	err = api.Database.InTx(func(tx database.Store) error {
		if err := tx.LockFoundation(ctx, foundation.ID); err != nil {
			return err
		}
		fresh, err := tx.GetFoundationByID(ctx, foundation.ID)
		if err != nil {
			return err
		}
		if fresh.Deleted {
			return sql.ErrNoRows
		}
		now := dbtime.Now()
		if _, err := tx.InsertTemplateResourceState(ctx, database.InsertTemplateResourceStateParams{FoundationID: foundation.ID, OrganizationID: foundation.OrganizationID, UpdatedAt: now}); err != nil {
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
		revision, err = tx.InsertTemplateResourceRevision(ctx, database.InsertTemplateResourceRevisionParams{ID: uuid.New(), FoundationID: foundation.ID, OrganizationID: foundation.OrganizationID, FileID: file.ID, Source: source, CreatedBy: userID, CreatedAt: now})
		if err != nil {
			return err
		}
		_, err = tx.SetTemplateResourceCurrentRevision(ctx, database.SetTemplateResourceCurrentRevisionParams{FoundationID: foundation.ID, CurrentRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, UpdatedAt: now})
		return err
	}, nil)
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	auditFields["revision_id"] = revision.ID
	httpapi.Write(ctx, rw, http.StatusCreated, convertTemplateResourceRevision(revision))
}

// @Summary Run shared resources
// @ID create-template-resource-run
// @Security CoderSessionToken
// @Tags Foundations
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param foundation path string true "Foundation ID" format(uuid)
// @Param request body codersdk.CreateTemplateResourceRunRequest true "Run"
// @Success 201 {object} codersdk.TemplateResourceRun
// @Router /api/v2/organizations/{organization}/foundations/{foundation}/runs [post]
// @x-apidocgen {"skip": true}
func (api *API) postTemplateResourceRun(rw http.ResponseWriter, r *http.Request) {
	foundation, ok := api.foundationParam(rw, r, policy.ActionRead)
	if !ok {
		return
	}
	auditFields := map[string]interface{}{"operation": "run"}
	ar, commit := audit.InitRequest[database.Foundation](rw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, OrganizationID: foundation.OrganizationID, Action: database.AuditActionStart, AdditionalFields: auditFields})
	defer commit()
	ar.Old, ar.New = foundation, foundation

	var req codersdk.CreateTemplateResourceRunRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	auditFields["action"], auditFields["revision_id"] = req.Action, req.RevisionID
	if req.Action != "plan" && req.Action != "apply" && req.Action != "destroy" {
		api.templateResourceError(rw, r, &templateResourceHTTPError{http.StatusBadRequest, "Action must be plan, apply, or destroy."})
		return
	}
	action := policy.ActionUse
	if req.Action == "plan" {
		action = policy.ActionUpdate
	}
	if !api.Authorize(r, action, foundation.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	ctx, userID := r.Context(), httpmw.APIKey(r).UserID
	var run database.TemplateResourceRun
	var job database.ProvisionerJob
	err := api.Database.InTx(func(tx database.Store) error {
		if err := tx.LockFoundation(ctx, foundation.ID); err != nil {
			return err
		}
		fresh, err := tx.GetFoundationByID(ctx, foundation.ID)
		if err != nil {
			return err
		}
		if fresh.Deleted {
			return sql.ErrNoRows
		}
		state, err := tx.GetTemplateResourceStateForUpdate(ctx, foundation.ID)
		if err != nil {
			return err
		}
		if state.Status == "unknown" {
			return templateResourceConflict("The previous mutation has an unknown outcome. Manual reconciliation is required; automatic recovery is not supported.")
		}
		if state.ActiveRunID.Valid {
			return templateResourceConflict("A shared resource run is already active.")
		}
		revision, err := tx.GetTemplateResourceRevisionByID(ctx, req.RevisionID)
		if err != nil {
			return err
		}
		if revision.FoundationID != foundation.ID || revision.OrganizationID != foundation.OrganizationID {
			return sql.ErrNoRows
		}
		// Foundation permissions were checked above. Count all consumers, including hidden workspaces.
		//nolint:gocritic
		systemCtx := dbauthz.AsSystemRestricted(ctx)
		if req.Action == "destroy" {
			if !state.CheckpointRevisionID.Valid || state.CheckpointRevisionID.UUID != revision.ID {
				return templateResourceConflict("Destroy must use the state-producing revision.")
			}
			if fresh.PreventDestroyWithWorkspaces {
				count, err := tx.GetFoundationWorkspaceCount(systemCtx, fresh.ID)
				if err != nil {
					return err
				}
				if count != 0 {
					return templateResourceConflict("Destroy protection is enabled. Delete all consuming workspaces or disable prevent_destroy_with_workspaces before destroying the Foundation.")
				}
			}
		}
		runID, now := uuid.New(), dbtime.Now()
		input, err := json.Marshal(struct {
			RunID uuid.UUID `json:"template_resource_run_id"`
		}{runID})
		if err != nil {
			return err
		}
		var routing map[string]string
		if err := json.Unmarshal(fresh.ProvisionerTags, &routing); err != nil {
			return err
		}
		tags := provisionersdk.MutateTags(uuid.Nil, routing)
		tags[provisionersdk.TagTemplateResources] = provisionersdk.TemplateResourcesCapabilityVersion
		job, err = tx.InsertProvisionerJob(systemCtx, database.InsertProvisionerJobParams{ID: uuid.New(), OrganizationID: foundation.OrganizationID, CreatedAt: now, UpdatedAt: now, InitiatorID: userID, Provisioner: database.ProvisionerTypeTerraform, Type: database.ProvisionerJobTypeTemplateResources, StorageMethod: database.ProvisionerStorageMethodFile, FileID: revision.FileID, Input: input, Tags: tags, TraceMetadata: pqtype.NullRawMessage{}, LogsOverflowed: false})
		if err != nil {
			return err
		}
		run, err = tx.InsertTemplateResourceRun(ctx, database.InsertTemplateResourceRunParams{ID: runID, FoundationID: foundation.ID, OrganizationID: foundation.OrganizationID, RevisionID: revision.ID, JobID: job.ID, Action: req.Action, CreatedAt: now})
		if err != nil {
			return err
		}
		_, err = tx.StartTemplateResourceRun(ctx, database.StartTemplateResourceRunParams{FoundationID: foundation.ID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, UpdatedAt: now})
		return err
	}, nil)
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	auditFields["run_id"] = run.ID
	if err := provisionerjobs.PostJob(api.Pubsub, job); err != nil {
		api.Logger.Warn(ctx, "publish shared resource job", slog.Error(err))
	}
	httpapi.Write(ctx, rw, http.StatusCreated, convertTemplateResourceRun(run, job))
}

// @Summary Cancel shared resource run
// @ID cancel-template-resource-run
// @Security CoderSessionToken
// @Tags Foundations
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param foundation path string true "Foundation ID" format(uuid)
// @Param runID path string true "Run ID" format(uuid)
// @Success 200 {object} codersdk.Response
// @Router /api/v2/organizations/{organization}/foundations/{foundation}/runs/{runID}/cancel [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchCancelTemplateResourceRun(rw http.ResponseWriter, r *http.Request) {
	foundation, ok := api.foundationParam(rw, r, policy.ActionRead)
	if !ok {
		return
	}
	auditFields := map[string]interface{}{"operation": "cancel"}
	ar, commit := audit.InitRequest[database.Foundation](rw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, OrganizationID: foundation.OrganizationID, Action: database.AuditActionStop, AdditionalFields: auditFields})
	defer commit()
	ar.Old, ar.New = foundation, foundation

	id, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		api.templateResourceError(rw, r, sql.ErrNoRows)
		return
	}
	auditFields["run_id"] = id
	ctx := r.Context()
	var completedJobID uuid.UUID
	err = api.Database.InTx(func(tx database.Store) error {
		run, err := tx.GetTemplateResourceRunByID(ctx, id)
		if err != nil {
			return err
		}
		if run.FoundationID != foundation.ID || run.OrganizationID != foundation.OrganizationID {
			return sql.ErrNoRows
		}
		action := policy.ActionUse
		if run.Action == "plan" {
			action = policy.ActionUpdate
		}
		if !api.Authorize(r, action, foundation.RBACObject()) {
			return sql.ErrNoRows
		}
		// Match worker finalization's lock order and fence acquisition.
		job, err := tx.GetProvisionerJobByIDWithLock(ctx, run.JobID)
		if err != nil {
			return err
		}
		if job.CompletedAt.Valid {
			return templateResourceConflict("The shared resource run is already complete.")
		}
		if job.CanceledAt.Valid {
			return nil
		}
		state, err := tx.GetTemplateResourceStateForUpdate(ctx, foundation.ID)
		if err != nil {
			return err
		}
		if !state.ActiveRunID.Valid || state.ActiveRunID.UUID != run.ID {
			return templateResourceConflict("The shared resource run reservation is stale.")
		}
		now := sql.NullTime{Time: dbtime.Now(), Valid: true}
		var completedAt sql.NullTime
		if !job.StartedAt.Valid && !job.WorkerID.Valid {
			// A pending job cannot have mutated infrastructure. Acquired jobs
			// retain their reservation until the worker or reaper finalizes.
			//nolint:gocritic // Action-specific Foundation permission was checked before releasing state.
			systemCtx := dbauthz.AsSystemRestricted(ctx)
			_, err = tx.CompleteTemplateResourceRun(systemCtx, database.CompleteTemplateResourceRunParams{
				FoundationID: foundation.ID, RunID: state.ActiveRunID, Status: state.Status,
				State: state.State, Outputs: state.Outputs, AppliedRevisionID: state.AppliedRevisionID, CheckpointRevisionID: state.CheckpointRevisionID, UpdatedAt: now.Time,
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
			api.Logger.Warn(ctx, "publish canceled shared resource job", slog.Error(err))
		}
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{Message: "Shared resource run cancellation requested."})
}

// @Summary Get shared resource run logs
// @ID get-template-resource-run-logs
// @Security CoderSessionToken
// @Tags Foundations
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param foundation path string true "Foundation ID" format(uuid)
// @Param runID path string true "Run ID" format(uuid)
// @Param after query integer false "Return logs after this ID"
// @Success 200 {array} codersdk.ProvisionerJobLog
// @Router /api/v2/organizations/{organization}/foundations/{foundation}/runs/{runID}/logs [get]
// @x-apidocgen {"skip": true}
func (api *API) templateResourceRunLogs(rw http.ResponseWriter, r *http.Request) {
	foundation, ok := api.foundationParam(rw, r, policy.ActionRead)
	if !ok {
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
	if run.FoundationID != foundation.ID || run.OrganizationID != foundation.OrganizationID {
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
	return codersdk.TemplateResourceRevision{ID: revision.ID, FoundationID: revision.FoundationID, CreatedAt: revision.CreatedAt, FileID: revision.FileID, Source: revision.Source}
}

func convertTemplateResourceRun(run database.TemplateResourceRun, job database.ProvisionerJob) codersdk.TemplateResourceRun {
	return codersdk.TemplateResourceRun{ID: run.ID, RevisionID: run.RevisionID, Action: run.Action, CreatedAt: run.CreatedAt, Job: convertProvisionerJob(database.GetProvisionerJobsByIDsWithQueuePositionRow{ProvisionerJob: job}), Preview: decodeFoundationPlanPreview(run.PlanPreview.RawMessage)}
}

// guardTemplateResourceDeletionInTx serializes deletion with source creation and runs.
func guardTemplateResourceDeletionInTx(ctx context.Context, tx database.Store, templateID uuid.UUID) error {
	if err := tx.LockTemplateResourceTemplate(ctx, templateID); err != nil {
		return err
	}
	foundation, err := tx.GetFoundationByTemplateID(ctx, uuid.NullUUID{UUID: templateID, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := tx.LockFoundation(ctx, foundation.ID); err != nil {
		return err
	}
	state, err := tx.GetTemplateResourceStateForUpdate(ctx, foundation.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("get shared resources: %w", err)
	}
	if state.ActiveRunID.Valid || (state.Status != "unconfigured" && state.Status != "destroyed") || state.AppliedRevisionID.Valid {
		return templateResourceConflict("Destroy shared resources before deleting the template. Unknown outcomes require manual reconciliation.")
	}
	return nil
}
