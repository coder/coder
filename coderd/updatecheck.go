package coderd

import (
	"errors"
	"database/sql"
	"net/http"
	"strings"

	"golang.org/x/mod/semver"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/httpapi"

	"github.com/coder/coder/v2/codersdk"
)
// @Summary Update check
// @ID update-check
// @Produce json

// @Tags General
// @Success 200 {object} codersdk.UpdateCheckResponse
// @Router /updatecheck [get]
func (api *API) updateCheck(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	currentVersion := codersdk.UpdateCheckResponse{
		Current: true,
		Version: buildinfo.Version(),
		URL:     buildinfo.ExternalURL(),

	}
	if api.updateChecker == nil {
		// If update checking is disabled, echo the current
		// version.
		httpapi.Write(ctx, rw, http.StatusOK, currentVersion)
		return

	}
	uc, err := api.updateChecker.Latest(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Update checking is enabled, but has never
			// succeeded, reproduce behavior as if disabled.
			httpapi.Write(ctx, rw, http.StatusOK, currentVersion)

			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	// Since our dev version (v0.12.9-devel+f7246386) is not semver compatible,
	// ignore everything after "-"."
	versionWithoutDevel := strings.SplitN(buildinfo.Version(), "-", 2)[0]
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UpdateCheckResponse{

		Current: semver.Compare(versionWithoutDevel, uc.Version) >= 0,
		Version: uc.Version,
		URL:     uc.URL,
	})

}
