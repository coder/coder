package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func (api *API) serviceBanner(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.ServiceBanner{
		Enabled:         true,
		Message:         "Testing!",
		BackgroundColor: "#FF00FF",
	})
}
