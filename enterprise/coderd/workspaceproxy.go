package coderd

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/enterprise/proxysdk"
)

// @Summary Create workspace proxy
// @ID create-workspace-proxy
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Templates
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

	if err := validateProxyURL(req.URL); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "URL is invalid.",
			Detail:  err.Error(),
		})
		return
	}

	if _, err := httpapi.CompileHostnamePattern(req.WildcardHostname); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Wildcard URL is invalid.",
			Detail:  err.Error(),
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
		ID:                uuid.New(),
		Name:              req.Name,
		DisplayName:       req.DisplayName,
		Icon:              req.Icon,
		Url:               req.URL,
		WildcardHostname:  req.WildcardHostname,
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
		Proxy:      convertProxy(proxy),
		ProxyToken: fullToken,
	})
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

	httpapi.Write(ctx, rw, http.StatusOK, convertProxies(proxies))
}

func convertProxies(p []database.WorkspaceProxy) []codersdk.WorkspaceProxy {
	resp := make([]codersdk.WorkspaceProxy, 0, len(p))
	for _, proxy := range p {
		resp = append(resp, convertProxy(proxy))
	}
	return resp
}

func convertProxy(p database.WorkspaceProxy) codersdk.WorkspaceProxy {
	return codersdk.WorkspaceProxy{
		ID:               p.ID,
		Name:             p.Name,
		Icon:             p.Icon,
		URL:              p.Url,
		WildcardHostname: p.WildcardHostname,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
		Deleted:          p.Deleted,
	}
}

// TODO(@dean): move this somewhere
func requireExternalProxyAuth(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			token := r.Header.Get(proxysdk.ExternalProxyTokenHeader)
			if token == "" {
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "Missing external proxy token",
				})
				return
			}

			// Split the token and lookup the corresponding workspace proxy.
			parts := strings.Split(token, ":")
			if len(parts) != 2 {
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid external proxy token",
				})
				return
			}
			proxyID, err := uuid.Parse(parts[0])
			if err != nil {
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid external proxy token",
				})
				return
			}
			secret := parts[1]
			if len(secret) != 64 {
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid external proxy token",
				})
				return
			}

			// Get the proxy.
			proxy, err := db.GetWorkspaceProxyByID(ctx, proxyID)
			if xerrors.Is(err, sql.ErrNoRows) {
				// Proxy IDs are public so we don't care about leaking them via
				// timing attacks.
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid external proxy token",
					Detail:  "Proxy not found.",
				})
				return
			}
			if err != nil {
				httpapi.InternalServerError(w, err)
				return
			}
			if proxy.Deleted {
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid external proxy token",
					Detail:  "Proxy has been deleted.",
				})
				return
			}

			// Do a subtle constant time comparison of the hash of the secret.
			hashedSecret := sha256.Sum256([]byte(secret))
			if subtle.ConstantTimeCompare(proxy.TokenHashedSecret, hashedSecret[:]) != 1 {
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid external proxy token",
					Detail:  "Invalid proxy token secret.",
				})
				return
			}

			// TODO: set on context.

			next.ServeHTTP(w, r)
		})
	}
}

// @Summary Issue signed workspace app token
// @ID issue-signed-workspace-app-token
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body proxysdk.IssueSignedAppTokenRequest true "Issue signed app token request"
// @Success 201 {object} proxysdk.IssueSignedAppTokenResponse
// @Router /proxy-internal/issue-signed-app-token [post]
// @x-apidocgen {"skip": true}
func (api *API) issueSignedAppToken(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// NOTE: this endpoint will return JSON on success, but will (usually)
	// return a self-contained HTML error page on failure. The external proxy
	// should forward any non-201 response to the client.

	var req proxysdk.IssueSignedAppTokenRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// HACK: the CreateToken code reads the session token from the request, but
	// since the session token is in a header, we need to make a fake request.
	//
	// TODO(@dean): fix this hack. This could be fixed by providing the token as
	// a param to CreateToken instead of the whole request.
	fakeReq := httptest.NewRequest("GET", req.AppRequest.BasePath, nil)
	fakeReq.Header.Set(codersdk.SessionTokenHeader, req.SessionToken)

	// Exchange the token.
	token, tokenStr, ok := api.AGPL.WorkspaceAppsProvider.CreateToken(ctx, rw, fakeReq, req.AppRequest)
	if !ok {
		return
	}
	if token == nil {
		httpapi.InternalServerError(rw, xerrors.New("nil token after calling token provider"))
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, proxysdk.IssueSignedAppTokenResponse{
		SignedToken:    *token,
		SignedTokenStr: tokenStr,
	})
}
