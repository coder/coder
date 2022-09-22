package coderd

import (
	"context"
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func (api *API) shouldBlockNonBrowserConnections(rw http.ResponseWriter) bool {
	api.entitlementsMu.Lock()
	browserOnly := api.entitlements.browserOnly
	api.entitlementsMu.Unlock()
	if api.BrowserOnly && browserOnly != codersdk.EntitlementNotEntitled {
		httpapi.Write(context.Background(), rw, http.StatusConflict, codersdk.Response{
			Message: "Non-browser connections are disabled for your deployment.",
		})
		return true
	}
	return false
}
