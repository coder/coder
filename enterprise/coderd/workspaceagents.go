package coderd

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) shouldBlockNonBrowserConnections(rw http.ResponseWriter) bool {
	if api.entitlements.Enabled(codersdk.FeatureBrowserOnly) {
		httpapi.Write(context.Background(), rw, http.StatusConflict, codersdk.Response{
			Message: "Non-browser connections are disabled for your deployment.",
		})
		return true
	}
	return false
}
