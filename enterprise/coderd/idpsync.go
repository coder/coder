package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/idpsync"
)

// @Summary Get group IdP Sync settings by organization
// @ID get-group-idp-sync-settings-by-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} idpsync.GroupSyncSettings
// @Router /organizations/{organization}/settings/idpsync/groups [get]
func (api *API) groupIDPSyncSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	rlv := api.Options.RuntimeConfig.OrganizationResolver(api.Database, org.ID)
	runtimeConfigEntry := api.IDPSync.GroupSyncSettings()
	settings, err := runtimeConfigEntry.Resolve(ctx, rlv)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, settings)
}

// @Summary Update group IdP Sync settings by organization
// @ID update-group-idp-sync-settings-by-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} idpsync.GroupSyncSettings
// @Router /organizations/{organization}/settings/idpsync/groups [post]
func (api *API) postGroupIDPSyncSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	var req idpsync.GroupSyncSettings
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	rlv := api.Options.RuntimeConfig.OrganizationResolver(api.Database, org.ID)
	runtimeConfigEntry := api.IDPSync.GroupSyncSettings()

	err := runtimeConfigEntry.SetRuntimeValue(ctx, rlv, &req)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	settings, err := runtimeConfigEntry.Resolve(ctx, rlv)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, settings)
}
