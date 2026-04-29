package coderd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get boundary session by ID
// @ID get-boundary-session-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Boundary
// @Param id path string true "Boundary session ID" format(uuid)
// @Success 200 {object} codersdk.BoundarySession
// @Router /boundary/sessions/{id} [get]
func (api *API) boundarySessionByID(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Boundary session ID is required.",
		})
		return
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid boundary session ID.",
			Detail:  err.Error(),
		})
		return
	}

	// GetBoundarySessionByID enforces ActionRead on
	// ResourceBoundaryLog via dbauthz.
	session, err := api.Database.GetBoundarySessionByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	resp, err := boundarySessionToSDK(ctx, api.Database, session)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// boundarySessionToSDK converts a database BoundarySession to
// the SDK representation. It resolves the workspace and owner
// from the workspace agent relationship.
func boundarySessionToSDK(ctx context.Context, db database.Store, session database.BoundarySession) (codersdk.BoundarySession, error) {
	//nolint:gocritic // System query to resolve workspace from agent ID.
	ws, err := db.GetWorkspaceByAgentID(dbauthz.AsSystemRestricted(ctx), session.WorkspaceAgentID)
	if err != nil {
		return codersdk.BoundarySession{}, err
	}

	return codersdk.BoundarySession{
		ID:              session.ID,
		WorkspaceID:     ws.ID,
		OwnerID:         ws.OwnerID,
		ConfinedProcess: session.ConfinedProcess,
		StartedAt:       session.StartedAt,
	}, nil
}
