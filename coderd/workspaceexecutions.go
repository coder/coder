package coderd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/workspaceexec"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) registerWorkspaceExecutionRoutes(r chi.Router) {
	r.Post("/workspace-executions", api.acquireWorkspaceExecution)
	r.Get("/workspace-executions/{session}", api.workspaceExecutionSessionStatus)
	r.Post("/workspace-executions/{session}/export", api.exportWorkspaceExecution)
}

// @Summary Acquire workspace execution session
// @ID acquire-workspace-execution-session
// @Security CoderSessionToken
// @Tags Workspaces
// @Produce json
// @Accept json
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.AcquireWorkspaceExecutionRequest true "Acquisition request"
// @Success 201 {object} codersdk.WorkspaceExecutionSession
// @Success 200 {object} codersdk.WorkspaceExecutionSession
// @Router /api/v2/organizations/{organization}/workspace-executions [post]
func (api *API) acquireWorkspaceExecution(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, actorID := httpmw.OrganizationParam(r).ID, httpmw.APIKey(r).UserID
	var req codersdk.AcquireWorkspaceExecutionRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.RequestID == uuid.Nil || req.OwnerID == uuid.Nil {
		httperror.WriteResponseError(ctx, rw, executionRequestError("request_id and owner_id must be nonzero UUIDs."))
		return
	}
	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceWorkspaceExecution.InOrg(orgID).WithOwner(req.OwnerID.String())) {
		httpapi.Forbidden(rw)
		return
	}
	// Canonicalize typed input before lookup. Admission-only freshness checks
	// happen later so a replay after lease expiry still recovers its identity.
	req.LeaseExpiresAt = req.LeaseExpiresAt.UTC()
	if req.Declarations.ExecutionDeadline != nil {
		req.Declarations.ExecutionDeadline = new(dbtime.Time(req.Declarations.ExecutionDeadline.UTC()))
	}
	if req.Declarations.ArtifactExpiresAt != nil {
		req.Declarations.ArtifactExpiresAt = new(req.Declarations.ArtifactExpiresAt.UTC())
	}
	req.Declarations.ResultPaths = append([]string{}, req.Declarations.ResultPaths...)
	slices.Sort(req.Declarations.ResultPaths)
	raw, err := json.Marshal(req)
	if err != nil {
		httperror.WriteResponseError(ctx, rw, err)
		return
	}
	digest := sha256.Sum256(raw)
	key := database.GetWorkspaceExecutionSessionByRequestParams{OrganizationID: orgID, ActorID: actorID, RequestID: req.RequestID}
	replay := func() (database.WorkspaceExecutionSession, bool, error) {
		row, err := api.Database.GetWorkspaceExecutionSessionByRequest(ctx, key)
		if errors.Is(err, sql.ErrNoRows) {
			return row, false, nil
		}
		if err != nil {
			return row, false, err
		}
		if !bytes.Equal(row.InputDigest, digest[:]) {
			return row, false, httperror.NewResponseError(http.StatusConflict, codersdk.Response{Message: "Request identity was already used with different acquisition input."})
		}
		return row, true, nil
	}
	row, found, err := replay()
	if err != nil {
		httperror.WriteResponseError(ctx, rw, err)
		return
	}
	if found {
		api.writeWorkspaceExecutionSession(ctx, rw, http.StatusOK, row)
		return
	}
	if err := validateWorkspaceExecutionRequest(req, api.Clock.Now()); err != nil {
		httperror.WriteResponseError(ctx, rw, err)
		return
	}
	members, err := api.Database.OrganizationMembers(ctx, database.OrganizationMembersParams{OrganizationID: orgID, UserID: req.OwnerID, IncludeSystem: false, GithubUserID: 0})
	if err != nil {
		httperror.WriteResponseError(ctx, rw, err)
		return
	}
	if len(members) != 1 || members[0].Status != database.UserStatusActive {
		httpapi.ResourceNotFound(rw)
		return
	}
	owner := workspaceOwner{ID: req.OwnerID, Username: members[0].Username, AvatarURL: members[0].AvatarURL}
	declarations, err := json.Marshal(req.Declarations)
	if err != nil {
		httperror.WriteResponseError(ctx, rw, err)
		return
	}
	sessionID := uuid.New()
	insert := func(ctx context.Context, tx database.Store, workspaceID, workspaceOwnerID, buildID uuid.UUID) error {
		var err error
		row, err = tx.InsertWorkspaceExecutionSession(ctx, database.InsertWorkspaceExecutionSessionParams{
			ID: sessionID, OrganizationID: orgID, OwnerID: req.OwnerID, ActorID: actorID, RequestID: req.RequestID,
			InputDigest: digest[:], CreatedAt: dbtime.Time(api.Clock.Now()), State: "active",
			Disposable: req.Disposable, Retained: *req.Retained, LeaseExpiresAt: req.LeaseExpiresAt, Declarations: declarations,
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true}, WorkspaceOwnerID: uuid.NullUUID{UUID: workspaceOwnerID, Valid: true},
			AcquisitionBuildID: uuid.NullUUID{UUID: buildID, Valid: true},
		})
		return err
	}
	if req.Create != nil {
		template, preflightErr := api.preflightWorkspaceCreate(ctx, req.OwnerID, *req.Create)
		if preflightErr != nil {
			httperror.WriteResponseError(ctx, rw, preflightErr)
			return
		}
		if template.OrganizationID != orgID {
			httpapi.ResourceNotFound(rw)
			return
		}
		aReq, commitAudit := audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
			Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, Action: database.AuditActionCreate,
			AdditionalFields: audit.AdditionalFields{WorkspaceOwner: owner.Username},
		})
		defer commitAudit()
		_, err = createWorkspace(ctx, aReq, actorID, api, owner, *req.Create, &createWorkspaceOptions{
			remoteAddr: r.RemoteAddr,
			postBuildInTX: func(ctx context.Context, tx database.Store, ws database.Workspace, build database.WorkspaceBuild) error {
				return insert(ctx, tx, ws.ID, ws.OwnerID, build.ID)
			},
		})
	} else {
		err = api.Database.InTx(func(tx database.Store) error {
			if err := workspaceexec.CheckAdmission(ctx, tx, req.WorkspaceID); err != nil {
				return err
			}
			workspace, err := tx.GetWorkspaceByID(ctx, req.WorkspaceID)
			if err != nil {
				return err
			}
			if workspace.OrganizationID != orgID || workspace.OwnerID != req.OwnerID {
				return httperror.ErrResourceNotFound
			}
			if req.Declarations.ResultAgentID != nil {
				agentWorkspace, err := tx.GetWorkspaceByAgentID(ctx, *req.Declarations.ResultAgentID)
				if err != nil {
					if httpapi.Is404Error(err) {
						return executionRequestError("result_agent_id must identify an accessible agent belonging to the existing workspace.")
					}
					return err
				}
				if agentWorkspace.ID != workspace.ID {
					return executionRequestError("result_agent_id must belong to the existing workspace.")
				}
			}
			if !api.HTTPAuth.AuthorizeContext(ctx, policy.ActionSSH, workspace) {
				return httperror.NewResponseError(http.StatusForbidden, codersdk.Response{Message: "Workspace SSH access is required."})
			}
			build, err := tx.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
			if err != nil {
				return err
			}
			return insert(ctx, tx, workspace.ID, workspace.OwnerID, build.ID)
		}, nil)
	}
	if err != nil {
		// A concurrent winner or a post-commit response failure can still have
		// committed the identity. Never recover by adopting a namesake workspace.
		original := err
		recovered, found, replayErr := replay()
		if replayErr != nil {
			httperror.WriteResponseError(ctx, rw, replayErr)
			return
		}
		if found {
			api.writeWorkspaceExecutionSession(ctx, rw, http.StatusOK, recovered)
			return
		}
		if errors.Is(original, workspaceexec.ErrAdmissionClosed) {
			httperror.WriteResponseError(ctx, rw, httperror.NewResponseError(http.StatusConflict, codersdk.Response{Message: "Workspace execution admission is closed."}))
			return
		}
		if httpapi.Is404Error(original) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httperror.WriteResponseError(ctx, rw, original)
		return
	}
	api.writeWorkspaceExecutionSession(ctx, rw, http.StatusCreated, row)
}

func executionRequestError(message string) error {
	return httperror.NewResponseError(http.StatusBadRequest, codersdk.Response{Message: message})
}

func validateWorkspaceExecutionRequest(req codersdk.AcquireWorkspaceExecutionRequest, now time.Time) error {
	if (req.Create == nil) == (req.WorkspaceID == uuid.Nil) {
		return executionRequestError("Specify exactly one of create or workspace_id.")
	}
	if req.Declarations.ResultAgentName != "" && req.Declarations.ResultAgentID != nil {
		return executionRequestError("Specify only one of result_agent_name or result_agent_id.")
	}
	if req.Declarations.ResultAgentID != nil && *req.Declarations.ResultAgentID == uuid.Nil {
		return executionRequestError("result_agent_id must be nonzero when supplied.")
	}
	if req.Retained == nil {
		return executionRequestError("retained must be explicitly true or false.")
	}
	if req.Create == nil && req.Disposable {
		return executionRequestError("An existing workspace cannot be made disposable by acquisition.")
	}
	if req.LeaseExpiresAt.IsZero() || !req.LeaseExpiresAt.After(now) {
		return executionRequestError("lease_expires_at must be explicitly set in the future.")
	}
	for i, result := range req.Declarations.ResultPaths {
		if !path.IsAbs(result) || path.Clean(result) != result || (i > 0 && req.Declarations.ResultPaths[i-1] == result) {
			return executionRequestError("result_paths must be unique, absolute, clean paths.")
		}
	}
	if req.Declarations.ExecutionDeadline != nil && !req.Declarations.ExecutionDeadline.After(now) {
		return executionRequestError("execution_deadline must be in the future when supplied.")
	}
	if req.Declarations.ArtifactExpiresAt != nil && !req.Declarations.ArtifactExpiresAt.After(now) {
		return executionRequestError("artifact_expires_at must be in the future when supplied.")
	}
	return nil
}

// @Summary Get workspace execution session
// @ID get-workspace-execution-session
// @Security CoderSessionToken
// @Tags Workspaces
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param session path string true "Session ID" format(uuid)
// @Success 200 {object} codersdk.WorkspaceExecutionSession
// @Router /api/v2/organizations/{organization}/workspace-executions/{session} [get]
func (api *API) workspaceExecutionSessionStatus(rw http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "session"))
	if err != nil {
		httperror.WriteResponseError(r.Context(), rw, executionRequestError("Invalid session ID."))
		return
	}
	row, err := api.Database.GetWorkspaceExecutionSessionByID(r.Context(), id)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httperror.WriteResponseError(r.Context(), rw, err)
		return
	}
	if row.OrganizationID != httpmw.OrganizationParam(r).ID {
		httpapi.ResourceNotFound(rw)
		return
	}
	api.writeWorkspaceExecutionSession(r.Context(), rw, http.StatusOK, row)
}

func (api *API) writeWorkspaceExecutionSession(ctx context.Context, rw http.ResponseWriter, status int, row database.WorkspaceExecutionSession) {
	result := codersdk.WorkspaceExecutionSession{
		ID: row.ID, OrganizationID: row.OrganizationID, OwnerID: row.OwnerID, ActorID: row.ActorID, RequestID: row.RequestID,
		State: row.State, Disposable: row.Disposable, Retained: row.Retained, LeaseExpiresAt: row.LeaseExpiresAt, Revision: row.Revision, Error: row.Error,
	}
	if row.WorkspaceID.Valid {
		result.WorkspaceID = &row.WorkspaceID.UUID
	}
	if row.AcquisitionBuildID.Valid {
		result.AcquisitionBuildID = &row.AcquisitionBuildID.UUID
	}
	if err := json.Unmarshal(row.Declarations, &result.Declarations); err != nil {
		httperror.WriteResponseError(ctx, rw, err)
		return
	}
	result.EffectiveArtifactExpiresAt = result.Declarations.ArtifactExpiresAt
	if row.RecoveryArtifactExpiresAt.Valid {
		result.EffectiveArtifactExpiresAt = &row.RecoveryArtifactExpiresAt.Time
	}
	if row.AcquisitionBuildID.Valid {
		build, err := api.Database.GetWorkspaceBuildByID(ctx, row.AcquisitionBuildID.UUID)
		switch {
		case httpapi.Is404Error(err):
			result.SourceUnavailable = true
		case err != nil:
			httperror.WriteResponseError(ctx, rw, err)
			return
		default:
			job, err := api.Database.GetProvisionerJobByID(ctx, build.JobID)
			switch {
			case httpapi.Is404Error(err):
				result.SourceUnavailable = true
			case err != nil:
				httperror.WriteResponseError(ctx, rw, err)
				return
			default:
				jobStatus := codersdk.ProvisionerJobStatus(job.JobStatus)
				result.AcquisitionBuild = &codersdk.WorkspaceExecutionBuild{
					ID: build.ID, JobStatus: jobStatus, Status: codersdk.ConvertWorkspaceStatus(jobStatus, codersdk.WorkspaceTransition(build.Transition)),
					Error: job.Error.String, ErrorCode: job.ErrorCode.String,
				}
			}
		}
	}
	httpapi.Write(ctx, rw, status, result)
}
