package coderd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/coderd/workspaceapps/previewhost"
	"github.com/coder/coder/v2/codersdk"
)

// postPreviewSignIn returns a URL that signs the browser in to one preview
// host. The dashboard frames it, so every hop stays on the preview's origin.
//
// @Summary Sign in to a workspace preview
// @ID sign-in-to-a-workspace-preview
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body codersdk.PreviewSignInRequest true "Preview sign-in request"
// @Success 200 {object} codersdk.PreviewSignInResponse
// @Router /api/experimental/previews/sign-in [post]
// @x-apidocgen {"skip": true}
func (api *API) postPreviewSignIn(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	if !api.Experiments.Enabled(codersdk.ExperimentOpenPreview) || !api.Authorize(r, policy.ActionCreate, apiKey) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.PreviewSignInRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	workspace, err := api.Database.GetWorkspaceByID(ctx, req.WorkspaceID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	// A preview runs agent-written code as whoever opens it, so only the
	// workspace owner may, even when the workspace is shared.
	if workspace.OwnerID != apiKey.UserID {
		httpapi.ResourceNotFound(rw)
		return
	}

	target, ok := appurl.ParsePreviewSlug(req.PreviewSlug)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Invalid preview slug %q.", req.PreviewSlug),
		})
		return
	}
	if target.Port != 0 {
		agents, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		i := slices.IndexFunc(agents, func(agent database.WorkspaceAgent) bool { return agent.Name == req.AgentName })
		if i < 0 || !slices.Contains(agents[i].DisplayApps, database.DisplayAppPortForwardingHelper) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("The workspace agent %q does not allow port forwarding, so its ports cannot be previewed.", req.AgentName),
			})
			return
		}
	}

	hosting, err := api.previewHostingFor(ctx, req.RegionID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "This region does not serve previews.",
			Detail:  err.Error(),
		})
		return
	}
	previewURL, err := hosting.URL(appurl.ApplicationURL{
		AppSlugOrPort: req.PreviewSlug,
		AgentName:     req.AgentName,
		WorkspaceName: workspace.Name,
		Username:      workspace.OwnerUsername,
	}, req.URLPath)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid preview address.",
			Detail:  err.Error(),
		})
		return
	}

	key, err := api.createAppConnectKey(ctx, apiKey)
	if errors.Is(err, apikey.ErrExceedsCaller) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	payload := workspaceapps.PreviewSignInPayload{
		Host:   previewURL.Host,
		APIKey: key,
	}
	payload.Subject = apiKey.UserID.String()
	payload.Fill(api.Clock.Now())
	token, err := jwtutils.Encrypt(ctx, api.AppEncryptionKeyCache, payload)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	origin := url.URL{Scheme: previewURL.Scheme, Host: previewURL.Host}
	// Append the token so the preview gets its query byte for byte once the
	// preview host strips the token.
	if previewURL.RawQuery != "" {
		previewURL.RawQuery += "&"
	}
	previewURL.RawQuery += previewhost.SignInParam + "=" + url.QueryEscape(token)
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.PreviewSignInResponse{
		URL:     previewURL.String(),
		Origin:  origin.String(),
		Sandbox: hosting.Sandbox(),
	})
}

// previewHostingFor returns how the region with regionID hosts previews: the
// primary for the zero ID or the deployment ID, otherwise a workspace proxy.
// Workspace proxies always host previews in strict mode.
func (api *API) previewHostingFor(ctx context.Context, regionID uuid.UUID) (*previewhost.Hosting, error) {
	if regionID == uuid.Nil || regionID.String() == api.DeploymentID {
		if api.previewHosting == nil {
			return nil, xerrors.New("the deployment has no wildcard app hostname")
		}
		return api.previewHosting, nil
	}

	//nolint:gocritic // Any user may read a proxy's public URLs, as the regions endpoint shows.
	proxy, err := api.Database.GetWorkspaceProxyByID(dbauthz.AsSystemRestricted(ctx), regionID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace proxy: %w", err)
	}
	if proxy.Deleted || proxy.WildcardHostname == "" {
		return nil, xerrors.Errorf("workspace proxy %q has no wildcard app hostname", proxy.Name)
	}
	proxyURL, err := url.Parse(proxy.Url)
	if err != nil {
		return nil, xerrors.Errorf("parse workspace proxy URL: %w", err)
	}
	return previewhost.Strict(proxy.WildcardHostname, proxyURL, api.AccessURL)
}
