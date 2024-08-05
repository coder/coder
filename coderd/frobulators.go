package coderd

import (
	"net/http"

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
// @Router /frobulators/{user} [post]
func (api *API) createFrobulator(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	var req codersdk.InsertFrobulatorRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	newID := uuid.New()
	err := api.Database.InsertFrobulator(ctx, database.InsertFrobulatorParams{
		ID:          newID,
		UserID:      user.ID,
		ModelNumber: req.ModelNumber,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, newID.String())
}

// @Summary Get user frobulators
// @ID get-user-frobulators
// @Security CoderSessionToken
// @Param user path string true "User ID, name, or me"
// @Produce json
// @Tags Frobulator
// @Success 200 {array} codersdk.Frobulator
// @Router /frobulators/{user} [get]
func (api *API) listUserFrobulators(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)
	frobs, err := api.Database.GetUserFrobulators(ctx, user.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	out := make([]codersdk.Frobulator, 0, len(frobs))
	for _, f := range frobs {
		out = append(out, codersdk.Frobulator{
			ID:          f.ID,
			UserID:      f.UserID,
			ModelNumber: f.ModelNumber,
		})
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, out)
}

// @Summary Get all frobulators
// @ID get-all-frobulators
// @Security CoderSessionToken
// @Produce json
// @Tags Frobulator
// @Success 200 {array} codersdk.Frobulator
// @Router /frobulators [get]
func (api *API) listAllFrobulators(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	frobs, err := api.Database.GetAllFrobulators(ctx)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, frobs)
}
