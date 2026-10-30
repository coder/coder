package coderd

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/provisionerkey"
	"github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Create provisioner key
// @Description Creates a provisioner key for an organization, which an external provisioner daemon uses to authenticate when it connects.
// @Description Set `name` (at most 64 characters, unique in the organization, and not `built-in`, `user-auth`, or `psk`) and optionally `tags`.
// @Description The response holds the key secret, which Coder stores only as a hash, so save it when you create the key.
// @Description Requires permission to create provisioner daemons in the organization.
// @Description Requires a Premium license.
// @ID create-provisioner-key
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID, name, or `default` for the default organization."
// @Success 201 {object} codersdk.CreateProvisionerKeyResponse
// @Router /api/v2/organizations/{organization}/provisionerkeys [post]
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
// @Description Returns the provisioner keys in an organization, oldest first, without their secrets.
// @Description The list leaves out the reserved `built-in`, `user-auth`, and `psk` keys, and returns only the keys you can read.
// @Description Requires permission to read provisioner daemons in the organization.
// @Description Requires a Premium license.
// @ID list-provisioner-key
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID, name, or `default` for the default organization."
// @Success 200 {object} []codersdk.ProvisionerKey
// @Router /api/v2/organizations/{organization}/provisionerkeys [get]
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
// @Description Returns each provisioner key in an organization, including the reserved `user-auth` key, with the provisioner daemons that connected with it and were seen recently.
// @Description Use it to check which external provisioner daemons are connected.
// @Description Returns only the keys you can read.
// @Description Requires permission to read provisioner daemons in the organization.
// @Description Requires a Premium license.
// @ID list-provisioner-key-daemons
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID, name, or `default` for the default organization."
// @Success 200 {object} []codersdk.ProvisionerKeyDaemons
// @Router /api/v2/organizations/{organization}/provisionerkeys/daemons [get]
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
// @Description Deletes a provisioner key, and daemons that authenticated with it lose their connection.
// @Description You can't delete the reserved `built-in`, `user-auth`, and `psk` keys.
// @Description Requires permission to delete provisioner daemons in the organization.
// @Description Requires a Premium license.
// @ID delete-provisioner-key
// @Security CoderSessionToken
// @Tags Enterprise
// @Param organization path string true "Organization ID, name, or `default` for the default organization."
// @Param provisionerkey path string true "Name of the provisioner key to delete."
// @Success 204
// @Router /api/v2/organizations/{organization}/provisionerkeys/{provisionerkey} [delete]
func (api *API) deleteProvisionerKey(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	provisionerKey := httpmw.ProvisionerKeyParam(r)

	if codersdk.IsReservedProvisionerKey(provisionerKey.ID) {
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

	// Notify subscribers that this key was deleted so active sessions tear down.
	// Publishing is best effort; a failure does not leave the key usable.
	if err := api.Pubsub.Publish(pubsub.ProvisionerKeyDeletedChannel(provisionerKey.ID), nil); err != nil {
		api.Logger.Warn(ctx, "failed to publish provisioner key deletion",
			slog.F("provisioner_key_id", provisionerKey.ID), slog.Error(err))
	}

	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

// @Summary Fetch provisioner key details
// @Description Returns the details of the provisioner key that you authenticate with, without its secret.
// @Description Authenticates with the key in the `Coder-Provisioner-Daemon-Key` header instead of a session token, and the response describes that key.
// @ID fetch-provisioner-key-details
// @Security CoderProvisionerKey
// @Produce json
// @Tags Enterprise
// @Param provisionerkey path string true "Provisioner key name. Coder ignores this value and returns the key in the `Coder-Provisioner-Daemon-Key` header."
// @Success 200 {object} codersdk.ProvisionerKey
// @Router /api/v2/provisionerkeys/{provisionerkey} [get]
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
