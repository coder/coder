package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
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

	id, ok := httpmw.ParseUUIDParam(rw, r, "id")
	if !ok {
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
