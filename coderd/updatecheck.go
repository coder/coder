package coderd

import (
	"net/http"

	"golang.org/x/mod/semver"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func (api *API) updateCheck(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if api.updateChecker == nil {
		// If update checking is disabled, echo the current
		// version.
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.UpdateCheckResponse{
			Current: true,
			Version: buildinfo.Version(),
			URL:     buildinfo.ExternalURL(),
		})
		return
	}

	uc, err := api.updateChecker.Latest(ctx)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UpdateCheckResponse{
		Current: semver.Compare(buildinfo.Version(), uc.Version) >= 0,
		Version: uc.Version,
		URL:     uc.URL,
	})
}
