package coderd

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/provisionerkey"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) postProvisionerKey(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)

	var req codersdk.CreateProvisionerKeyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	params, token, err := provisionerkey.New(organization.ID, req.Name)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	_, err = api.Database.InsertProvisionerKey(ctx, params)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.CreateProvisionerKeyResponse{
		Key: token,
	})
}

func (api *API) provisionerKeys(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)

	pks, err := api.Database.ListProvisionerKeysByOrganization(ctx, organization.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertProvisionerKeys(pks))
}

func (api *API) deleteProvisionerKey(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)
	provisionerKey := httpmw.ProvisionerKeyParam(r)

	pk, err := api.Database.GetProvisionerKeyByName(ctx, database.GetProvisionerKeyByNameParams{
		OrganizationID: organization.ID,
		Name:           provisionerKey.Name,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	err = api.Database.DeleteProvisionerKey(ctx, pk.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

func convertProvisionerKeys(dbKeys []database.ListProvisionerKeysByOrganizationRow) []codersdk.ProvisionerKey {
	keys := make([]codersdk.ProvisionerKey, 0, len(dbKeys))
	for _, dbKey := range dbKeys {
		keys = append(keys, codersdk.ProvisionerKey{
			ID:             dbKey.ID,
			CreatedAt:      dbKey.CreatedAt,
			OrganizationID: dbKey.OrganizationID,
			Name:           dbKey.Name,
		})
	}
	return keys
}
