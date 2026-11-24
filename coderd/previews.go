package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/net/publicsuffix"
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

// postPreviewAddress returns where a preview loads, without signing in. The
// dashboard frames that URL first and signs in only when the preview host's
// signed-out page asks, so a browser that still has the preview's session
// cookie mints no key.
//
// @Summary Get a workspace preview's address
// @ID get-a-workspace-preview-address
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body codersdk.PreviewSignInRequest true "Preview request"
// @Success 200 {object} codersdk.PreviewSignInResponse
// @Router /api/experimental/previews/address [post]
// @x-apidocgen {"skip": true}
func (api *API) postPreviewAddress(rw http.ResponseWriter, r *http.Request) {
	previewURL, hosting, ok := api.resolvePreview(rw, r)
	if !ok {
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, previewFrame(previewURL, hosting))
}

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
	previewURL, hosting, ok := api.resolvePreview(rw, r)
	if !ok {
		return
	}

	key, err := api.createAppConnectKey(ctx, httpmw.APIKey(r))
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
	payload.Fill(api.Clock.Now())
	token, err := jwtutils.Encrypt(ctx, api.AppEncryptionKeyCache, payload)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// Append the token so the preview gets its query byte for byte once the
	// preview host strips the token.
	if previewURL.RawQuery != "" {
		previewURL.RawQuery += "&"
	}
	previewURL.RawQuery += previewhost.SignInParam + "=" + url.QueryEscape(token)
	httpapi.Write(ctx, rw, http.StatusOK, previewFrame(previewURL, hosting))
}

// resolvePreview reads a request for a preview of a workspace the caller owns
// and returns the preview's URL in the chosen region. When it refuses, it
// writes the response and returns false.
func (api *API) resolvePreview(rw http.ResponseWriter, r *http.Request) (*url.URL, *previewhost.Hosting, bool) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	if !api.Experiments.Enabled(codersdk.ExperimentOpenPreview) || !api.Authorize(r, policy.ActionCreate, apiKey) {
		httpapi.ResourceNotFound(rw)
		return nil, nil, false
	}
	if errors.Is(api.previewsUnavailable, previewhost.ErrHostPrefixCookieOff) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Previews require --host-prefix-cookie. An administrator can enable it.",
		})
		return nil, nil, false
	}

	var req codersdk.PreviewSignInRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return nil, nil, false
	}

	workspace, err := api.Database.GetWorkspaceByID(ctx, req.WorkspaceID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return nil, nil, false
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return nil, nil, false
	}
	// A preview runs agent-written code as whoever opens it, so only the
	// workspace owner may, even when the workspace is shared.
	if workspace.OwnerID != apiKey.UserID {
		httpapi.ResourceNotFound(rw)
		return nil, nil, false
	}

	if _, ok := appurl.ParsePreviewSlug(req.PreviewSlug); !ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Invalid preview slug %q.", req.PreviewSlug),
		})
		return nil, nil, false
	}
	agents, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return nil, nil, false
	}
	i := slices.IndexFunc(agents, func(agent database.WorkspaceAgent) bool { return agent.Name == req.AgentName })
	if i < 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("The workspace has no agent named %q.", req.AgentName),
		})
		return nil, nil, false
	}

	hosting, err := api.previewHostingFor(ctx, req.RegionID)
	if crossSite, ok := errors.AsType[crossSiteError](err); ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: string(crossSite)})
		return nil, nil, false
	}
	if errors.Is(err, errRegionWithoutPreviews) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "This region does not serve previews.",
			Detail:  err.Error(),
		})
		return nil, nil, false
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return nil, nil, false
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
		return nil, nil, false
	}

	return previewURL, hosting, true
}

// previewFrame returns how the dashboard frames the preview at previewURL.
func previewFrame(previewURL *url.URL, hosting *previewhost.Hosting) codersdk.PreviewSignInResponse {
	origin := url.URL{Scheme: previewURL.Scheme, Host: previewURL.Host}
	return codersdk.PreviewSignInResponse{
		URL:     previewURL.String(),
		Origin:  origin.String(),
		Sandbox: hosting.Sandbox(),
	}
}

// errRegionWithoutPreviews marks a region the user chose that has no preview
// hosts, as opposed to a failure looking it up. Its text follows each reason as
// the user's next step. A region on another site gets crossSiteError instead.
var errRegionWithoutPreviews = xerrors.New("choose another region")

// crossSiteError is the response title for a region whose preview hosts are on
// another site than the dashboard, where browsers do not send the preview
// host's default SameSite=Lax session cookie from the dashboard's frame, and
// Safari, or any browser set to block third-party cookies, drops it even with
// SameSite=None.
type crossSiteError string

func (e crossSiteError) Error() string { return string(e) }

// previewHostingFor returns how the region with regionID hosts previews: the
// primary for the zero ID or the deployment ID, otherwise a workspace proxy.
// Workspace proxies always host previews in strict mode.
func (api *API) previewHostingFor(ctx context.Context, regionID uuid.UUID) (*previewhost.Hosting, error) {
	if regionID == uuid.Nil || regionID.String() == api.DeploymentID {
		if api.previewHosting == nil {
			return nil, xerrors.Errorf("the deployment has no wildcard app hostname: %w", errRegionWithoutPreviews)
		}
		if registrableDomain(strings.Replace(api.AppHostname, "*", "preview", 1)) != registrableDomain(api.AccessURL.Host) {
			return nil, crossSiteError("Previews need a wildcard access URL on the same site as the dashboard.")
		}
		return api.previewHosting, nil
	}

	// The regions endpoint and app sign-in read proxies the same way. No public low-level scope grants
	// workspace_proxy:read, so reading as the caller would refuse scoped tokens that ordinary apps accept.
	//nolint:gocritic // Proxy URLs are public through the regions endpoint.
	proxy, err := api.Database.GetWorkspaceProxyByID(dbauthz.AsSystemRestricted(ctx), regionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("no workspace proxy has ID %s: %w", regionID, errRegionWithoutPreviews)
	}
	if err != nil {
		return nil, xerrors.Errorf("get workspace proxy: %w", err)
	}
	if proxy.Deleted || proxy.WildcardHostname == "" {
		return nil, xerrors.Errorf("workspace proxy %q has no wildcard app hostname: %w", proxy.Name, errRegionWithoutPreviews)
	}
	if registrableDomain(strings.Replace(proxy.WildcardHostname, "*", "preview", 1)) != registrableDomain(api.AccessURL.Host) {
		return nil, crossSiteError("Previews need a workspace proxy on the same site as the dashboard.")
	}
	proxyURL, err := url.Parse(proxy.Url)
	if err != nil {
		return nil, xerrors.Errorf("parse workspace proxy URL: %w", err)
	}
	return previewhost.Strict(proxy.WildcardHostname, proxyURL, api.AccessURL)
}

// registrableDomain returns the registrable domain of host, with or without a
// port, such as example.com for coder.example.com:8443, or the hostname
// itself when it has none, such as an IP address or localhost. Browsers treat
// two hosts as the same site when these are equal.
func registrableDomain(host string) string {
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		host = hostname
	}
	host = strings.ToLower(host)
	if domain, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		return domain
	}
	return host
}
