package workspaceapps

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/coderd/workspaceapps/previewhost"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/site"
)

// servePreview serves a request to a preview host. Every response, coderd's
// own pages included, goes through the preview policy writer.
func (s *Server) servePreview(rw http.ResponseWriter, r *http.Request, host string, middlewares []func(http.Handler) http.Handler) {
	rw = s.Previews.Wrap(rw)

	// A service worker outlives the page that registered it, so it could
	// keep answering on this host after the port or files change. Browsers
	// send this header only for service worker scripts.
	if r.Header.Get("Service-Worker") == "script" {
		writePreviewError(rw, r, s.DashboardURL, http.StatusForbidden, "Preview pages cannot register service workers.")
		return
	}

	if isFromOtherPreview(r, host) {
		writeFromOtherPreview(rw, r, s.DashboardURL)
		return
	}

	app, _, err := s.Previews.Parse(host)
	if err != nil {
		writePreviewError(rw, r, s.DashboardURL, http.StatusBadRequest, "This is not a valid preview address.")
		return
	}

	// A preview loaded as the top-level page has no wrapper to keep it on
	// its own host, so it reopens inside the dashboard. Fetch metadata is
	// missing from non-browser clients and from plain-HTTP origins other
	// than localhost, so a request without it is treated as a page load.
	// Chromium sends no fetch metadata with a WebSocket handshake, which
	// never loads a page.
	dest := r.Header.Get("Sec-Fetch-Dest")
	if dest == "document" || (dest == "" && !httpapi.IsWebsocketUpgrade(r)) {
		http.Redirect(rw, r, s.previewOpenURL(app, r.URL).String(), http.StatusSeeOther)
		return
	}

	// The smuggled API key sets a cookie on the whole app wildcard, which a
	// preview must never do.
	if r.URL.Query().Has(SubdomainProxyAPIKeyParam) {
		writePreviewError(rw, r, s.DashboardURL, http.StatusBadRequest, "Previews sign in only from the dashboard.")
		return
	}

	// The direct-open redirect above already answered page loads, so a
	// sign-in never runs as the top-level document.
	if r.URL.Query().Has(previewhost.SignInParam) {
		s.signInPreview(rw, r, host)
		return
	}

	token, ok := ResolveRequest(rw, r, ResolveRequestOptions{
		Logger:              s.Logger,
		Cookies:             s.cookies,
		CookieCfg:           s.CookiesConfig,
		SignedTokenProvider: s.SignedTokenProvider,
		DashboardURL:        s.DashboardURL,
		PathAppBaseURL:      s.AccessURL,
		AppHostname:         s.Hostname,
		AppRequest: Request{
			AccessMethod:      AccessMethodSubdomain,
			BasePath:          "/",
			UsernameOrID:      app.Username,
			WorkspaceNameOrID: app.WorkspaceName,
			AgentNameOrID:     app.AgentName,
			AppSlugOrPort:     app.AppSlugOrPort,
		},
		AppPath:  r.URL.Path,
		AppQuery: r.URL.RawQuery,
	})
	if !ok {
		return
	}

	// Previews never get the app's CORS handling, so ModifyResponse strips
	// any CORS headers the preview server sends.
	chi.Chain(middlewares...).HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		s.proxyWorkspaceApp(rw, r, *token, r.URL.Path, app)
	}).ServeHTTP(rw, r)
}

// signInPreview trades the dashboard's sign-in payload for a session cookie
// on this preview host only, then reloads the page without the payload.
func (s *Server) signInPreview(rw http.ResponseWriter, r *http.Request, host string) {
	ctx := r.Context()
	var payload PreviewSignInPayload
	err := jwtutils.Decrypt(ctx, s.APIKeyEncryptionKeycache, r.URL.Query().Get(previewhost.SignInParam), &payload)
	if err == nil && !appurl.HostnamesMatch(payload.Host, host) {
		err = xerrors.Errorf("payload is for host %q", payload.Host)
	}
	if err != nil {
		s.Logger.Debug(ctx, "invalid preview sign-in", slog.Error(err))
		writePreviewError(rw, r, s.DashboardURL, http.StatusBadRequest, "This preview sign-in is invalid or expired. Reopen the preview from Coder.")
		return
	}

	// No Domain, so the cookie stays on this preview host.
	http.SetCookie(rw, s.CookiesConfig.Apply(&http.Cookie{
		Name:     codersdk.PreviewSessionTokenCookie,
		Value:    payload.APIKey,
		Path:     "/",
		HttpOnly: true,
	}))

	redirectURL := originLocalURL(r.URL.Path)
	redirectURL.RawQuery, _ = stripQueryParam(r.URL.RawQuery, previewhost.SignInParam)
	rw.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(rw, r, redirectURL.String(), http.StatusSeeOther)
}

// previewOpenURL returns the dashboard page that opens app's preview at u's
// path and query inside its wrapper.
func (s *Server) previewOpenURL(app appurl.ApplicationURL, u *url.URL) *url.URL {
	appPath := originLocalURL(u.Path)
	appPath.RawQuery, _ = stripQueryParam(u.RawQuery, previewhost.SignInParam)
	appPath.RawQuery, _ = stripQueryParam(appPath.RawQuery, SubdomainProxyAPIKeyParam)

	open := *s.DashboardURL
	open.Path = "/previews/open"
	open.RawQuery = url.Values{
		"owner":     {app.Username},
		"workspace": {app.WorkspaceName},
		"agent":     {app.AgentName},
		"slug":      {app.AppSlugOrPort},
		"path":      {appPath.String()},
	}.Encode()
	return &open
}

func writePreviewError(rw http.ResponseWriter, r *http.Request, dashboardURL *url.URL, status int, description string) {
	site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
		Status:      status,
		Title:       http.StatusText(status),
		Description: description,
		Actions:     []site.Action{{URL: dashboardURL.String(), Text: "Back to site"}},
	})
}

func writeFromOtherPreview(rw http.ResponseWriter, r *http.Request, dashboardURL *url.URL) {
	writePreviewError(rw, r, dashboardURL, http.StatusForbidden, "Preview pages cannot send requests to other apps.")
}

// isFromOtherPreview reports whether a page on a preview host other than host
// sent r. The browser sets Origin, so a preview page cannot forge it, and it
// adds the user's cookies for the target, so the request would act as the
// user. This also covers requests that CSP does not govern, such as the POST
// of a link's ping attribute.
func isFromOtherPreview(r *http.Request, host string) bool {
	origin, err := url.Parse(r.Header.Get(httpmw.OriginHeader))
	if err != nil || origin.Host == "" || appurl.HostnamesMatch(origin.Host, host) {
		return false
	}
	return appurl.IsPreviewHostname(origin.Hostname())
}
