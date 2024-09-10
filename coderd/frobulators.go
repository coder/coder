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

// @Summary Get frobulators
// @ID get-frobulators
// @Security CoderSessionToken
// @Param organization path string true "Organization ID"
// @Param user path string true "User ID, name, or me"
// @Produce json
// @Tags Frobulator
// @Success 200 {array} codersdk.Frobulator
// @Router /organizations/{organization}/members/{user}/frobulators [get]
func (api *API) listFrobulators(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	member := httpmw.OrganizationMemberParam(r)
	org := httpmw.OrganizationParam(r)

	frobs, err := api.Database.GetFrobulators(ctx, database.GetFrobulatorsParams{
		UserID: member.UserID,
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

// @Summary Post frobulator
// @ID post-frobulator
// @Security CoderSessionToken
// @Param request body codersdk.InsertFrobulatorRequest true "Insert Frobulator request"
// @Param organization path string true "Organization ID"
// @Param user path string true "User ID, name, or me"
// @Accept json
// @Produce json
// @Tags Frobulator
// @Success 200 "New frobulator ID"
// @Router /organizations/{organization}/members/{user}/frobulators [post]
func (api *API) createFrobulator(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	member := httpmw.OrganizationMemberParam(r)
	org := httpmw.OrganizationParam(r)

	var req codersdk.InsertFrobulatorRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	frob, err := api.Database.InsertFrobulator(ctx, database.InsertFrobulatorParams{
		ID:          uuid.New(),
		UserID:      member.UserID,
		OrgID:       org.ID,
		ModelNumber: req.ModelNumber,
	})
	if httpapi.Is404Error(err) { // Catches forbidden errors as well
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, frob.ID.String())
}

// @Summary Delete frobulator
// @ID delete-frobulator
// @Security CoderSessionToken
// @Param organization path string true "Organization ID"
// @Param user path string true "User ID, name, or me"
// @Param id path string true "Frobulator ID"
// @Tags Frobulator
// @Success 204
// @Router /organizations/{organization}/members/{user}/frobulators/{id} [delete]
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

	member := httpmw.OrganizationMemberParam(r)
	org := httpmw.OrganizationParam(r)

	err = api.Database.DeleteFrobulator(ctx, database.DeleteFrobulatorParams{
		ID:     id,
		UserID: member.UserID,
		OrgID:  org.ID,
	})
	if httpapi.Is404Error(err) { // Catches forbidden errors as well
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}
