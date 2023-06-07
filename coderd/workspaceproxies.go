package coderd

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func (api *API) PrimaryRegion(ctx context.Context) (codersdk.Region, error) {
	deploymentIDStr, err := api.Database.GetDeploymentID(ctx)
	if xerrors.Is(err, sql.ErrNoRows) {
		// This shouldn't happen but it's pretty easy to avoid this causing
		// issues by falling back to a nil UUID.
		deploymentIDStr = uuid.Nil.String()
	} else if err != nil {
		return codersdk.Region{}, xerrors.Errorf("get deployment ID: %w", err)
	}
	deploymentID, err := uuid.Parse(deploymentIDStr)
	if err != nil {
		// This also shouldn't happen but we fallback to nil UUID.
		deploymentID = uuid.Nil
	}

	return codersdk.Region{
		ID:               deploymentID,
		Name:             "primary",
		DisplayName:      "Default",
		IconURL:          "/emojis/1f3e1.png", // House with garden
		Healthy:          true,
		PathAppURL:       api.AccessURL.String(),
		WildcardHostname: api.AppHostname,
	}, nil
}

// @Summary Get site-wide regions for workspace connections
// @ID get-site-wide-regions-for-workspace-connections
// @Security CoderSessionToken
// @Produce json
// @Tags WorkspaceProxies
// @Success 200 {object} codersdk.RegionsResponse
// @Router /regions [get]
func (api *API) regions(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	//nolint:gocritic // this route intentionally requests resources that users
	// cannot usually access in order to give them a full list of available
	// regions.
	ctx = dbauthz.AsSystemRestricted(ctx)

	region, err := api.PrimaryRegion(ctx)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.RegionsResponse{
		Regions: []codersdk.Region{region},
	})
}
