package coderd
import (
	"fmt"
	"errors"
	"context"
	"database/sql"
	"net/http"
	"github.com/google/uuid"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
)
// PrimaryRegion exposes the user facing values of a workspace proxy to
// be used by a user.
func (api *API) PrimaryRegion(ctx context.Context) (codersdk.Region, error) {
	deploymentIDStr, err := api.Database.GetDeploymentID(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		// This shouldn't happen but it's pretty easy to avoid this causing
		// issues by falling back to a nil UUID.
		deploymentIDStr = uuid.Nil.String()
	} else if err != nil {
		return codersdk.Region{}, fmt.Errorf("get deployment ID: %w", err)
	}
	deploymentID, err := uuid.Parse(deploymentIDStr)
	if err != nil {
		// This also shouldn't happen but we fallback to nil UUID.
		deploymentID = uuid.Nil
	}
	proxy, err := api.Database.GetDefaultProxyConfig(ctx)
	if err != nil {
		return codersdk.Region{}, fmt.Errorf("get default proxy config: %w", err)
	}
	return codersdk.Region{
		ID:               deploymentID,
		Name:             "primary",
		DisplayName:      proxy.DisplayName,
		IconURL:          proxy.IconUrl,
		Healthy:          true,
		PathAppURL:       api.AccessURL.String(),
		WildcardHostname: appurl.SubdomainAppHost(api.AppHostname, api.AccessURL),
	}, nil
}
// PrimaryWorkspaceProxy returns the primary workspace proxy for the site.
func (api *API) PrimaryWorkspaceProxy(ctx context.Context) (database.WorkspaceProxy, error) {
	region, err := api.PrimaryRegion(ctx)
	if err != nil {
		return database.WorkspaceProxy{}, err
	}
	// The default proxy is an edge case because these values are computed
	// rather then being stored in the database.
	return database.WorkspaceProxy{
		ID:               region.ID,
		Name:             region.Name,
		DisplayName:      region.DisplayName,
		Icon:             region.IconURL,
		Url:              region.PathAppURL,
		WildcardHostname: region.WildcardHostname,
		Deleted:          false,
	}, nil
}
// @Summary Get site-wide regions for workspace connections
// @ID get-site-wide-regions-for-workspace-connections
// @Security CoderSessionToken
// @Produce json
// @Tags WorkspaceProxies
// @Success 200 {object} codersdk.RegionsResponse[codersdk.Region]
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
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.RegionsResponse[codersdk.Region]{
		Regions: []codersdk.Region{region},
	})
}
