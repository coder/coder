package coderd

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
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

	if slices.ContainsFunc(codersdk.ReservedProvisionerKeyNames(), func(s string) bool {
		return strings.EqualFold(req.Name, s)
	}) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Name cannot be reserved name '%s'", req.Name),
			Validations: []codersdk.ValidationError{
				{
					Field:  "name",
					Detail: fmt.Sprintf("Name cannot be reserved name '%s'", req.Name),
				},
			},
		})
		return
	}

	params, token, err := provisionerkey.New(organization.ID, req.Name, req.Tags)
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

	pks, err := api.Database.ListProvisionerKeysByOrganizationExcludeReserved(ctx, organization.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertProvisionerKeys(pks))
}

// @Summary List provisioner key daemons
// @ID list-provisioner-key-daemons
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID"
// @Success 200 {object} []codersdk.ProvisionerKeyDaemons
// @Router /organizations/{organization}/provisionerkeys/daemons [get]
func (api *API) provisionerKeyDaemons(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)

	pks, err := api.Database.ListProvisionerKeysByOrganization(ctx, organization.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	sdkKeys := convertProvisionerKeys(pks)

	// For the default organization, we insert three rows for the special
	// provisioner key types (built-in, user-auth, and psk). We _don't_ insert
	// those into the database for any other org, but we still need to include the
	// user-auth key in this list, so we just insert it manually.
	if !slices.ContainsFunc(sdkKeys, func(key codersdk.ProvisionerKey) bool {
		return key.ID == codersdk.ProvisionerKeyUUIDUserAuth
	}) {
		sdkKeys = append(sdkKeys, codersdk.ProvisionerKey{
			ID:   codersdk.ProvisionerKeyUUIDUserAuth,
			Name: codersdk.ProvisionerKeyNameUserAuth,
			Tags: map[string]string{},
		})
	}

	daemons, err := api.Database.GetProvisionerDaemonsByOrganization(ctx, database.GetProvisionerDaemonsByOrganizationParams{OrganizationID: organization.ID})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	// provisionerdserver.DefaultHeartbeatInterval*3 matches the healthcheck report staleInterval.
	recentDaemons := db2sdk.RecentProvisionerDaemons(time.Now(), provisionerdserver.DefaultHeartbeatInterval*3, daemons)

	pkDaemons := []codersdk.ProvisionerKeyDaemons{}
	for _, key := range sdkKeys {
		// The key.OrganizationID for the `user-auth` key is hardcoded to
		// the default org in the database and we are overwriting it here
		// to be the correct org we used to query the list.
		// This will be changed when we update the `user-auth` keys to be
		// directly tied to a user ID.
		if key.ID.String() == codersdk.ProvisionerKeyIDUserAuth {
			key.OrganizationID = organization.ID
		}
		daemons := []codersdk.ProvisionerDaemon{}
		for _, daemon := range recentDaemons {
			if daemon.KeyID == key.ID {
				daemons = append(daemons, daemon)
			}
		}
		pkDaemons = append(pkDaemons, codersdk.ProvisionerKeyDaemons{
			Key:     key,
			Daemons: daemons,
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, pkDaemons)
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
	provisionerKey := httpmw.ProvisionerKeyParam(r)

	if provisionerKey.ID.String() == codersdk.ProvisionerKeyIDBuiltIn ||
		provisionerKey.ID.String() == codersdk.ProvisionerKeyIDUserAuth ||
		provisionerKey.ID.String() == codersdk.ProvisionerKeyIDPSK {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Cannot delete reserved '%s' provisioner key", provisionerKey.Name),
		})
		return
	}

	err := api.Database.DeleteProvisionerKey(ctx, provisionerKey.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

// @Summary Fetch provisioner key details
// @ID fetch-provisioner-key-details
// @Security CoderProvisionerKey
// @Produce json
// @Tags Enterprise
// @Param provisionerkey path string true "Provisioner Key"
// @Success 200 {object} codersdk.ProvisionerKey
// @Router /provisionerkeys/{provisionerkey} [get]
func (*API) fetchProvisionerKey(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pk, ok := httpmw.ProvisionerKeyAuthOptional(r)
	// extra check but this one should never happen as it is covered by the auth middleware
	if !ok {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: fmt.Sprintf("unable to auth: please provide the %s header", codersdk.ProvisionerDaemonKey),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertProvisionerKey(pk))
}

func convertProvisionerKey(dbKey database.ProvisionerKey) codersdk.ProvisionerKey {
	return codersdk.ProvisionerKey{
		ID:             dbKey.ID,
		CreatedAt:      dbKey.CreatedAt,
		OrganizationID: dbKey.OrganizationID,
		Name:           dbKey.Name,
		Tags:           codersdk.ProvisionerKeyTags(dbKey.Tags),
		// HashedSecret - never include the access token in the API response
	}
}

func convertProvisionerKeys(dbKeys []database.ProvisionerKey) []codersdk.ProvisionerKey {
	keys := make([]codersdk.ProvisionerKey, 0, len(dbKeys))
	for _, dbKey := range dbKeys {
		keys = append(keys, convertProvisionerKey(dbKey))
	}

	slices.SortFunc(keys, func(key1, key2 codersdk.ProvisionerKey) int {
		return key1.CreatedAt.Compare(key2.CreatedAt)
	})

	return keys
}
