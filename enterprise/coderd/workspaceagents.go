package coderd

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) shouldBlockNonBrowserConnections(rw http.ResponseWriter) bool {
	api.entitlementsMu.RLock()
	browserOnly := api.entitlements.Features[codersdk.FeatureBrowserOnly].Enabled
	api.entitlementsMu.RUnlock()
	if browserOnly {
		httpapi.Write(context.Background(), rw, http.StatusConflict, codersdk.Response{
			Message: "Non-browser connections are disabled for your deployment.",
		})
		return true
	}
	return false
}
