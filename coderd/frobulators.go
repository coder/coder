package coderd

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Post frobulator
// @ID post-frobulator
// @Security CoderSessionToken
// @Param request body codersdk.InsertFrobulatorRequest true "Insert Frobulator request"
// @Param user path string true "User ID, name, or me"
// @Accept json
// @Produce json
// @Tags Frobulator
// @Success 200 "New frobulator ID"
// @Router /organizations/{organization}/frobulators/{user} [post]
func (api *API) createFrobulator(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)
	org := httpmw.OrganizationParam(r)

	var req codersdk.InsertFrobulatorRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	frob, err := api.Database.InsertFrobulator(ctx, database.InsertFrobulatorParams{
		UserID:      user.ID,
		OrgID:       org.ID,
		ModelNumber: req.ModelNumber,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, frob.ID.String())
}

// @Summary Get frobulators
// @ID get-frobulators
// @Security CoderSessionToken
// @Param user path string true "User ID, name, or me"
// @Produce json
// @Tags Frobulator
// @Success 200 {array} codersdk.Frobulator
// @Router /organizations/{organization}/frobulators/{user} [get]
func (api *API) listFrobulators(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)
	org := httpmw.OrganizationParam(r)

	frobs, err := api.Database.GetFrobulators(ctx, database.GetFrobulatorsParams{
		UserID: user.ID,
		OrgID:  org.ID,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	out := make([]codersdk.Frobulator, 0, len(frobs))
	for _, f := range frobs {
		out = append(out, codersdk.Frobulator{
			ID:          f.ID,
			UserID:      f.UserID,
			OrgID:       f.OrgID,
			ModelNumber: f.ModelNumber,
		})
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, out)
}

// @Summary Delete frobulator
// @ID delete-frobulator
// @Security CoderSessionToken
// @Produce json
// @Tags Frobulator
// @Success 200
// @Router /organizations/{organization}/frobulators/{user}/{id} [get]
func (api *API) deleteFrobulator(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idParam := chi.URLParam(r, "id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Frobulator ID %q must be a valid UUID.", id),
			Detail:  err.Error(),
		})
		return
	}

	user := httpmw.UserParam(r)
	org := httpmw.OrganizationParam(r)

	err = api.Database.DeleteFrobulator(ctx, database.DeleteFrobulatorParams{
		ID:     id,
		UserID: user.ID,
		OrgID:  org.ID,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, nil)
}
