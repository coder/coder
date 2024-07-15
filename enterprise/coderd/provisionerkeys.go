package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/provisionerkey"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Create provisioner key
// @ID create-provisioner-key
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID"
// @Success 201 {object} codersdk.CreateProvisionerKeyResponse
// @Router /organizations/{organization}/provisionerkeys [post]
func (api *API) postProvisionerKey(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)

	var req codersdk.CreateProvisionerKeyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.Name == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Name is required",
			Validations: []codersdk.ValidationError{
				{
					Field:  "name",
					Detail: "Name is required",
				},
			},
		})
		return
	}

	if len(req.Name) > 64 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Name must be at most 64 characters",
			Validations: []codersdk.ValidationError{
				{
					Field:  "name",
					Detail: "Name must be at most 64 characters",
				},
			},
		})
		return
	}

	params, token, err := provisionerkey.New(organization.ID, req.Name)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	_, err = api.Database.InsertProvisionerKey(ctx, params)
	if database.IsUniqueViolation(err, database.UniqueProvisionerKeysOrganizationIDNameIndex) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Provisioner key with name '%s' already exists in organization", req.Name),
		})
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.CreateProvisionerKeyResponse{
		Key: token,
	})
}

// @Summary List provisioner key
// @ID list-provisioner-key
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID"
// @Success 200 {object} []codersdk.ProvisionerKey
// @Router /organizations/{organization}/provisionerkeys [get]
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

// @Summary Delete provisioner key
// @ID delete-provisioner-key
// @Security CoderSessionToken
// @Tags Enterprise
// @Param organization path string true "Organization ID"
// @Param provisionerkey path string true "Provisioner key name"
// @Success 204
// @Router /organizations/{organization}/provisionerkeys/{provisionerkey} [delete]
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

func convertProvisionerKeys(dbKeys []database.ProvisionerKey) []codersdk.ProvisionerKey {
	keys := make([]codersdk.ProvisionerKey, 0, len(dbKeys))
	for _, dbKey := range dbKeys {
		keys = append(keys, codersdk.ProvisionerKey{
			ID:             dbKey.ID,
			CreatedAt:      dbKey.CreatedAt,
			OrganizationID: dbKey.OrganizationID,
			Name:           dbKey.Name,
			// HashedSecret - never include the access token in the API response
		})
	}
	return keys
}
