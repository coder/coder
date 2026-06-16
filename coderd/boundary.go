package coderd

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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
// @Router /api/v2/boundary/sessions/{id} [get]
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

	session, err := api.Database.GetBoundarySessionByID(ctx, id)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.BoundarySession{
		ID:              session.ID,
		WorkspaceID:     session.WorkspaceID,
		OwnerID:         session.WorkspaceOwnerID,
		ConfinedProcess: session.ConfinedProcessName,
		StartedAt:       session.StartedAt,
	})
}
