package coderd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"regexp"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rewrite2026augustlog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

const maxAISandboxNameLength = 64

var aiSandboxNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// @Summary Create or reconcile an AI sandbox
// @ID create-ai-sandbox
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.CreateAISandboxRequest true "Sandbox declaration"
// @Success 200 {object} agentsdk.CreateAISandboxResponse
// @Router /workspaceagents/me/ai-sandboxes [post]
func (api *API) postWorkspaceAgentAISandbox(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	parentAgent := httpmw.WorkspaceAgent(r)

	var req agentsdk.CreateAISandboxRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if validations := validateAISandboxRequest(req); len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI sandbox declaration.",
			Validations: validations,
		})
		return
	}

	// Only a top-level agent may own sandboxes. A sandbox child creating its
	// own sandbox would nest confinement boundaries whose egress ownership
	// and identity lineage this design does not define.
	if parentAgent.ParentID.Valid {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "A sandboxed agent cannot create sandboxes.",
		})
		return
	}

	workspace, err := api.workspaceAgentWorkspace(ctx, r)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	//nolint:gocritic // Sandbox lifecycle rows are system-owned; the handler
	// verifies that the caller is the owning parent agent.
	systemCtx := dbauthz.AsSystemRestricted(ctx)

	// Identity continuity: an AI-bound parent is itself acting for an AI
	// identity, so its sandbox reuses that identity rather than minting a
	// new one. An unbound parent means a human-declared sandbox, which is a
	// human-to-AI boundary and resolves the workspace-origin identity.
	var aiAgentID uuid.UUID
	if parentAgent.AIAgentID.Valid {
		resolved, rerr := aiagentidentity.Resolve(systemCtx, api.Database, parentAgent.AIAgentID.UUID)
		if rerr != nil {
			// Fail closed: a revoked identity or a deleted, suspended, or
			// non-human sponsor must not yield a usable sandbox.
			httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
				Message: "The calling agent's AI identity is no longer valid.",
			})
			return
		}
		aiAgentID = resolved.Ledger.ID
	} else {
		origin, oerr := aiagentidentity.ResolveWorkspaceOrigin(systemCtx, api.Database, workspace)
		if oerr != nil {
			err = oerr
		} else {
			aiAgentID = origin.ID
		}
		if err != nil {
			httpapi.InternalServerError(rw, xerrors.Errorf("resolve workspace AI identity: %w", err))
			return
		}
	}

	existing, err := api.Database.GetAISandboxByParentAgentAndName(systemCtx, database.GetAISandboxByParentAgentAndNameParams{
		ParentAgentID: parentAgent.ID,
		Name:          req.Name,
	})
	switch {
	case err == nil:
		// Reconcile: the parent restarted and is reattaching. The child row
		// and its auth token survive, but the scoped session token's
		// plaintext does not, so it is rotated.
		child, cerr := api.Database.GetWorkspaceAgentByID(systemCtx, existing.ChildAgentID)
		if cerr != nil {
			httpapi.InternalServerError(rw, xerrors.Errorf("get sandbox child agent: %w", cerr))
			return
		}
		token, terr := api.rotateAISandboxSessionToken(systemCtx, workspace.ID, existing)
		if terr != nil {
			httpapi.InternalServerError(rw, terr)
			return
		}
		httpapi.Write(ctx, rw, http.StatusOK, agentsdk.CreateAISandboxResponse{
			ID:           existing.ID,
			ChildAgentID: existing.ChildAgentID,
			AIAgentID:    existing.AIAgentID,
			AgentToken:   child.AuthToken.String(),
			SessionToken: token,
			Reconciled:   true,
		})
		return
	case errors.Is(err, sql.ErrNoRows):
	default:
		httpapi.InternalServerError(rw, xerrors.Errorf("get AI sandbox: %w", err))
		return
	}

	sandboxID := uuid.New()
	now := dbtime.Now()
	var (
		child database.WorkspaceAgent
		token string
	)
	err = api.Database.InTx(func(tx database.Store) error {
		txCtx := dbauthz.AsSystemRestricted(ctx) //nolint:gocritic // See above.
		created, err := tx.InsertWorkspaceAgent(txCtx, database.InsertWorkspaceAgentParams{
			ID:         uuid.New(),
			ParentID:   uuid.NullUUID{UUID: parentAgent.ID, Valid: true},
			CreatedAt:  now,
			UpdatedAt:  now,
			Name:       req.Name,
			ResourceID: parentAgent.ResourceID,
			AuthToken:  uuid.New(),
			// The child inherits the parent's platform shape but none of
			// its credentials: the binding set here activates credential
			// starvation for every enforcement point, before the row is
			// ever observable.
			AIAgentID:                uuid.NullUUID{UUID: aiAgentID, Valid: true},
			Architecture:             parentAgent.Architecture,
			OperatingSystem:          parentAgent.OperatingSystem,
			Directory:                parentAgent.Directory,
			ConnectionTimeoutSeconds: parentAgent.ConnectionTimeoutSeconds,
			TroubleshootingURL:       parentAgent.TroubleshootingURL,
			APIKeyScope:              parentAgent.APIKeyScope,
			DisplayApps:              []database.DisplayApp{},
			// The child carries no inherited environment, metadata, or
			// MOTD: the create script supplies everything it may see.
			AuthInstanceID:       sql.NullString{},
			EnvironmentVariables: pqtype.NullRawMessage{},
			InstanceMetadata:     pqtype.NullRawMessage{},
			ResourceMetadata:     pqtype.NullRawMessage{},
			MOTDFile:             "",
			DisplayOrder:         0,
		})
		if err != nil {
			return xerrors.Errorf("insert sandbox child agent: %w", err)
		}
		if _, err := tx.InsertAISandbox(txCtx, database.InsertAISandboxParams{
			ID:                sandboxID,
			WorkspaceID:       workspace.ID,
			ParentAgentID:     parentAgent.ID,
			ChildAgentID:      created.ID,
			AIAgentID:         aiAgentID,
			Name:              req.Name,
			EgressEnforcement: string(req.EgressEnforcement),
			CreatedAt:         now,
		}); err != nil {
			return xerrors.Errorf("insert AI sandbox: %w", err)
		}
		rewrite2026augustlog.SandboxCreated(txCtx, rewrite2026augustlog.F{
			"sandbox_id":         sandboxID,
			"workspace_id":       workspace.ID,
			"parent_agent_id":    parentAgent.ID,
			"child_agent_id":     created.ID,
			"ai_agent_user_id":   aiAgentID,
			"name":               req.Name,
			"egress_enforcement": string(req.EgressEnforcement),
		})
		_, minted, err := aiagentidentity.MintKey(txCtx, tx, aiAgentID,
			aiagentidentity.SandboxIdentityProfile(workspace.ID, sandboxID))
		if err != nil {
			return xerrors.Errorf("mint sandbox session token: %w", err)
		}
		child = created
		token = minted
		return nil
	}, nil)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, agentsdk.CreateAISandboxResponse{
		ID:           sandboxID,
		ChildAgentID: child.ID,
		AIAgentID:    aiAgentID,
		AgentToken:   child.AuthToken.String(),
		SessionToken: token,
	})
}

// @Summary List AI sandboxes
// @ID list-ai-sandboxes
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Success 200 {array} agentsdk.AISandbox
// @Router /workspaceagents/me/ai-sandboxes [get]
func (api *API) workspaceAgentAISandboxes(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	parentAgent := httpmw.WorkspaceAgent(r)

	//nolint:gocritic // Sandbox lifecycle rows are system-owned and scoped
	// here to the authenticated parent agent.
	sandboxes, err := api.Database.GetAISandboxesByParentAgentID(dbauthz.AsSystemRestricted(ctx), parentAgent.ID)
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get AI sandboxes: %w", err))
		return
	}

	response := make([]agentsdk.AISandbox, 0, len(sandboxes))
	for _, sandbox := range sandboxes {
		response = append(response, agentsdk.AISandbox{
			ID:                sandbox.ID,
			ChildAgentID:      sandbox.ChildAgentID,
			AIAgentID:         sandbox.AIAgentID,
			Name:              sandbox.Name,
			EgressEnforcement: codersdk.AISandboxEgressEnforcement(sandbox.EgressEnforcement),
		})
	}
	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// @Summary Delete an AI sandbox
// @ID delete-ai-sandbox
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param sandbox path string true "AI sandbox ID" format(uuid)
// @Success 200 {object} codersdk.Response
// @Router /workspaceagents/me/ai-sandboxes/{sandbox} [delete]
func (api *API) deleteWorkspaceAgentAISandbox(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	parentAgent := httpmw.WorkspaceAgent(r)

	sandboxID, parsed := httpmw.ParseUUIDParam(rw, r, "sandbox")
	if !parsed {
		return
	}

	//nolint:gocritic // Sandbox lifecycle rows are system-owned; ownership is
	// verified against the authenticated parent agent below.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	sandbox, err := api.Database.GetAISandboxByID(systemCtx, sandboxID)
	// A sandbox owned by another agent is reported as missing so callers
	// cannot probe for sandboxes they do not own.
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (sandbox.Deleted || sandbox.ParentAgentID != parentAgent.ID)) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get AI sandbox: %w", err))
		return
	}

	// Soft-deleting the child row invalidates its auth token, so the
	// confined agent can no longer authenticate even if the destroy script
	// leaves the process running.
	if err := api.Database.DeleteWorkspaceSubAgentByID(systemCtx, sandbox.ChildAgentID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, xerrors.Errorf("delete sandbox child agent: %w", err))
		return
	}
	if err := api.deleteAISandboxSessionToken(systemCtx, sandbox); err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if err := api.Database.SoftDeleteAISandbox(systemCtx, sandbox.ID); err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("soft delete AI sandbox: %w", err))
		return
	}
	rewrite2026augustlog.SandboxDeleted(ctx, rewrite2026augustlog.F{
		"sandbox_id":       sandbox.ID,
		"workspace_id":     sandbox.WorkspaceID,
		"parent_agent_id":  sandbox.ParentAgentID,
		"child_agent_id":   sandbox.ChildAgentID,
		"ai_agent_user_id": sandbox.AIAgentID,
		"name":             sandbox.Name,
	})

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{Message: "Sandbox deleted."})
}

// rotateAISandboxSessionToken mints a fresh scoped session token for an
// existing sandbox, dropping the previous one. MintKey does not replace keys
// by name, so the stale key is deleted first.
func (api *API) rotateAISandboxSessionToken(ctx context.Context, workspaceID uuid.UUID, sandbox database.AISandbox) (string, error) {
	if err := api.deleteAISandboxSessionToken(ctx, sandbox); err != nil {
		return "", err
	}
	_, token, err := aiagentidentity.MintKey(ctx, api.Database, sandbox.AIAgentID,
		aiagentidentity.SandboxIdentityProfile(workspaceID, sandbox.ID))
	if err != nil {
		return "", xerrors.Errorf("mint sandbox session token: %w", err)
	}
	return token, nil
}

func (api *API) deleteAISandboxSessionToken(ctx context.Context, sandbox database.AISandbox) error {
	profile := aiagentidentity.SandboxIdentityProfile(sandbox.WorkspaceID, sandbox.ID)
	return aiagentidentity.RevokeKey(ctx, api.Database, sandbox.AIAgentID, profile.TokenName)
}

func validateAISandboxRequest(req agentsdk.CreateAISandboxRequest) []codersdk.ValidationError {
	var validations []codersdk.ValidationError
	switch {
	case req.Name == "":
		validations = append(validations, codersdk.ValidationError{
			Field: "name", Detail: "sandbox name cannot be empty",
		})
	case len(req.Name) > maxAISandboxNameLength:
		validations = append(validations, codersdk.ValidationError{
			Field: "name", Detail: "sandbox name is too long",
		})
	case !aiSandboxNameRegex.MatchString(req.Name):
		validations = append(validations, codersdk.ValidationError{
			Field: "name", Detail: "sandbox name must be alphanumeric with dashes or underscores",
		})
	}
	switch req.EgressEnforcement {
	case codersdk.AISandboxEgressEnforcementForced,
		codersdk.AISandboxEgressEnforcementAdvisory,
		codersdk.AISandboxEgressEnforcementNone:
	default:
		validations = append(validations, codersdk.ValidationError{
			Field:  "egress_enforcement",
			Detail: "must be one of forced, advisory, or none",
		})
	}
	return validations
}
