package coderd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	agpl "github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/workspaceapps"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/enterprise/coderd/proxyhealth"
	"github.com/coder/coder/enterprise/wsproxy/wsproxysdk"
)

// forceWorkspaceProxyHealthUpdate forces an update of the proxy health.
// This is useful when a proxy is created or deleted. Errors will be logged.
func (api *API) forceWorkspaceProxyHealthUpdate(ctx context.Context) {
	if err := api.ProxyHealth.ForceUpdate(ctx); err != nil {
		api.Logger.Error(ctx, "force proxy health update", slog.Error(err))
	}
}

// NOTE: this doesn't need a swagger definition since AGPL already has one, and
// this route overrides the AGPL one.
func (api *API) regions(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	//nolint:gocritic // this route intentionally requests resources that users
	// cannot usually access in order to give them a full list of available
	// regions.
	ctx = dbauthz.AsSystemRestricted(ctx)

	primaryRegion, err := api.AGPL.PrimaryRegion(ctx)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	regions := []codersdk.Region{primaryRegion}

	proxies, err := api.Database.GetWorkspaceProxies(ctx)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// Only add additional regions if the proxy health is enabled.
	// If it is nil, it is because the moons feature flag is not on.
	// By default, we still want to return the primary region.
	if api.ProxyHealth != nil {
		proxyHealth := api.ProxyHealth.HealthStatus()
		for _, proxy := range proxies {
			if proxy.Deleted {
				continue
			}

			health, ok := proxyHealth[proxy.ID]
			if !ok {
				health.Status = proxyhealth.Unknown
			}

			regions = append(regions, codersdk.Region{
				ID:               proxy.ID,
				Name:             proxy.Name,
				DisplayName:      proxy.DisplayName,
				IconURL:          proxy.Icon,
				Healthy:          health.Status == proxyhealth.Healthy,
				PathAppURL:       proxy.Url,
				WildcardHostname: proxy.WildcardHostname,
			})
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.RegionsResponse{
		Regions: regions,
	})
}

// @Summary Delete workspace proxy
// @ID delete-workspace-proxy
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param workspaceproxy path string true "Proxy ID or name" format(uuid)
// @Success 200 {object} codersdk.Response
// @Router /workspaceproxies/{workspaceproxy} [delete]
func (api *API) deleteWorkspaceProxy(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		proxy             = httpmw.WorkspaceProxyParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.WorkspaceProxy](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	aReq.Old = proxy
	defer commitAudit()

	err := api.Database.UpdateWorkspaceProxyDeleted(ctx, database.UpdateWorkspaceProxyDeletedParams{
		ID:      proxy.ID,
		Deleted: true,
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = database.WorkspaceProxy{}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Proxy has been deleted!",
	})

	// Update the proxy health cache to remove this proxy.
	go api.forceWorkspaceProxyHealthUpdate(api.ctx)
}

// @Summary Create workspace proxy
// @ID create-workspace-proxy
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.CreateWorkspaceProxyRequest true "Create workspace proxy request"
// @Success 201 {object} codersdk.WorkspaceProxy
// @Router /workspaceproxies [post]
func (api *API) postWorkspaceProxy(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.WorkspaceProxy](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	var req codersdk.CreateWorkspaceProxyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if strings.ToLower(req.Name) == "primary" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: `The name "primary" is reserved for the primary region.`,
			Detail:  "Cannot name a workspace proxy 'primary'.",
			Validations: []codersdk.ValidationError{
				{
					Field:  "name",
					Detail: "Reserved name",
				},
			},
		})
		return
	}

	id := uuid.New()
	secret, err := cryptorand.HexString(64)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	hashedSecret := sha256.Sum256([]byte(secret))
	fullToken := fmt.Sprintf("%s:%s", id, secret)

	proxy, err := api.Database.InsertWorkspaceProxy(ctx, database.InsertWorkspaceProxyParams{
		ID:                id,
		Name:              req.Name,
		DisplayName:       req.DisplayName,
		Icon:              req.Icon,
		TokenHashedSecret: hashedSecret[:],
		CreatedAt:         database.Now(),
		UpdatedAt:         database.Now(),
	})
	if database.IsUniqueViolation(err) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Workspace proxy with name %q already exists.", req.Name),
		})
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = proxy
	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.CreateWorkspaceProxyResponse{
		Proxy: convertProxy(proxy, proxyhealth.ProxyStatus{
			Proxy:     proxy,
			CheckedAt: time.Now(),
			Status:    proxyhealth.Unregistered,
		}),
		ProxyToken: fullToken,
	})

	// Update the proxy health cache to include this new proxy.
	go api.forceWorkspaceProxyHealthUpdate(api.ctx)
}

// nolint:revive
func validateProxyURL(u string) error {
	p, err := url.Parse(u)
	if err != nil {
		return err
	}
	if p.Scheme != "http" && p.Scheme != "https" {
		return xerrors.New("scheme must be http or https")
	}
	if !(p.Path == "/" || p.Path == "") {
		return xerrors.New("path must be empty or /")
	}
	return nil
}

// @Summary Get workspace proxies
// @ID get-workspace-proxies
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {array} codersdk.WorkspaceProxy
// @Router /workspaceproxies [get]
func (api *API) workspaceProxies(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	proxies, err := api.Database.GetWorkspaceProxies(ctx)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return
	}

	statues := api.ProxyHealth.HealthStatus()
	httpapi.Write(ctx, rw, http.StatusOK, convertProxies(proxies, statues))
}

// @Summary Issue signed workspace app token
// @ID issue-signed-workspace-app-token
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body workspaceapps.IssueTokenRequest true "Issue signed app token request"
// @Success 201 {object} wsproxysdk.IssueSignedAppTokenResponse
// @Router /workspaceproxies/me/issue-signed-app-token [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceProxyIssueSignedAppToken(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// NOTE: this endpoint will return JSON on success, but will (usually)
	// return a self-contained HTML error page on failure. The external proxy
	// should forward any non-201 response to the client.

	var req workspaceapps.IssueTokenRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// userReq is a http request from the user on the other side of the proxy.
	// Although the workspace proxy is making this call, we want to use the user's
	// authorization context to create the token.
	//
	// We can use the existing request context for all tracing/logging purposes.
	// Any workspace proxy auth uses different context keys so we don't need to
	// worry about that.
	userReq, err := http.NewRequestWithContext(ctx, "GET", req.AppRequest.BasePath, nil)
	if err != nil {
		// This should never happen
		httpapi.InternalServerError(rw, xerrors.Errorf("[DEV ERROR] new request: %w", err))
		return
	}
	userReq.Header.Set(codersdk.SessionTokenHeader, req.SessionToken)

	// Exchange the token.
	token, tokenStr, ok := api.AGPL.WorkspaceAppsProvider.Issue(ctx, rw, userReq, req)
	if !ok {
		return
	}
	if token == nil {
		httpapi.InternalServerError(rw, xerrors.New("nil token after calling token provider"))
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, wsproxysdk.IssueSignedAppTokenResponse{
		SignedTokenStr: tokenStr,
	})
}

// workspaceProxyRegister is used to register a new workspace proxy. When a proxy
// comes online, it will announce itself to this endpoint. This updates its values
// in the database and returns a signed token that can be used to authenticate
// tokens.
//
// @Summary Register workspace proxy
// @ID register-workspace-proxy
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body wsproxysdk.RegisterWorkspaceProxyRequest true "Issue signed app token request"
// @Success 201 {object} wsproxysdk.RegisterWorkspaceProxyResponse
// @Router /workspaceproxies/me/register [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceProxyRegister(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx   = r.Context()
		proxy = httpmw.WorkspaceProxy(r)
	)

	var req wsproxysdk.RegisterWorkspaceProxyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if err := validateProxyURL(req.AccessURL); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "URL is invalid.",
			Detail:  err.Error(),
		})
		return
	}

	if req.WildcardHostname != "" {
		if _, err := httpapi.CompileHostnamePattern(req.WildcardHostname); err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Wildcard URL is invalid.",
				Detail:  err.Error(),
			})
			return
		}
	}

	_, err := api.Database.RegisterWorkspaceProxy(ctx, database.RegisterWorkspaceProxyParams{
		ID:               proxy.ID,
		Url:              req.AccessURL,
		WildcardHostname: req.WildcardHostname,
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, wsproxysdk.RegisterWorkspaceProxyResponse{
		AppSecurityKey: api.AppSecurityKey.String(),
	})

	go api.forceWorkspaceProxyHealthUpdate(api.ctx)
}

// reconnectingPTYSignedToken issues a signed app token for use when connecting
// to the reconnecting PTY websocket on an external workspace proxy. This is set
// by the client as a query parameter when connecting.
//
// @Summary Issue signed app token for reconnecting PTY
// @ID issue-signed-app-token-for-reconnecting-pty
// @Security CoderSessionToken
// @Tags Applications Enterprise
// @Accept json
// @Produce json
// @Param request body codersdk.IssueReconnectingPTYSignedTokenRequest true "Issue reconnecting PTY signed token request"
// @Success 200 {object} codersdk.IssueReconnectingPTYSignedTokenResponse
// @Router /applications/reconnecting-pty-signed-token [post]
// @x-apidocgen {"skip": true}
func (api *API) reconnectingPTYSignedToken(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, rbac.ActionCreate, apiKey) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.IssueReconnectingPTYSignedTokenRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	u, err := url.Parse(req.URL)
	if err == nil && u.Scheme != "ws" && u.Scheme != "wss" {
		err = xerrors.Errorf("invalid URL scheme %q, expected 'ws' or 'wss'", u.Scheme)
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid URL.",
			Detail:  err.Error(),
		})
		return
	}

	// Assert the URL is a valid reconnecting-pty URL.
	expectedPath := fmt.Sprintf("/api/v2/workspaceagents/%s/pty", req.AgentID.String())
	if u.Path != expectedPath {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid URL path.",
			Detail:  "The provided URL is not a valid reconnecting PTY endpoint URL.",
		})
		return
	}

	scheme, err := api.AGPL.ValidWorkspaceAppHostname(ctx, u.Host, agpl.ValidWorkspaceAppHostnameOpts{
		// Only allow the proxy access URL as a hostname since we don't need a
		// ticket for the primary dashboard URL terminal.
		AllowPrimaryAccessURL: false,
		AllowPrimaryWildcard:  false,
		AllowProxyAccessURL:   true,
		AllowProxyWildcard:    false,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to verify hostname in URL.",
			Detail:  err.Error(),
		})
		return
	}
	if scheme == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid hostname in URL.",
			Detail:  "The hostname must be the primary wildcard app hostname, a workspace proxy access URL or a workspace proxy wildcard app hostname.",
		})
		return
	}

	_, tokenStr, ok := api.AGPL.WorkspaceAppsProvider.Issue(ctx, rw, r, workspaceapps.IssueTokenRequest{
		AppRequest: workspaceapps.Request{
			AccessMethod:  workspaceapps.AccessMethodTerminal,
			BasePath:      u.Path,
			AgentNameOrID: req.AgentID.String(),
		},
		SessionToken: httpmw.APITokenFromRequest(r),
		// The following fields aren't required as long as the request is authed
		// with a valid API key.
		PathAppBaseURL: "",
		AppHostname:    "",
		// The following fields are empty for terminal apps.
		AppPath:  "",
		AppQuery: "",
	})
	if !ok {
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.IssueReconnectingPTYSignedTokenResponse{
		SignedToken: tokenStr,
	})
}

func convertProxies(p []database.WorkspaceProxy, statuses map[uuid.UUID]proxyhealth.ProxyStatus) []codersdk.WorkspaceProxy {
	resp := make([]codersdk.WorkspaceProxy, 0, len(p))
	for _, proxy := range p {
		resp = append(resp, convertProxy(proxy, statuses[proxy.ID]))
	}
	return resp
}

func convertProxy(p database.WorkspaceProxy, status proxyhealth.ProxyStatus) codersdk.WorkspaceProxy {
	return codersdk.WorkspaceProxy{
		ID:               p.ID,
		Name:             p.Name,
		Icon:             p.Icon,
		URL:              p.Url,
		WildcardHostname: p.WildcardHostname,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
		Deleted:          p.Deleted,
		Status: codersdk.WorkspaceProxyStatus{
			Status:    codersdk.ProxyHealthStatus(status.Status),
			Report:    status.Report,
			CheckedAt: status.CheckedAt,
		},
	}
}
