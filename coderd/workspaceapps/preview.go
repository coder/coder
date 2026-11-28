package workspaceapps

import (
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/site"
)

// servePreview serves a request to a preview host. Every response coderd
// writes goes through the preview policy writer, and proxyWorkspaceApp
// applies the same policy to proxied responses, WebSocket upgrades included.
func (s *Server) servePreview(rw http.ResponseWriter, r *http.Request, host string, middlewares []func(http.Handler) http.Handler) {
	rw = s.Previews.Wrap(rw)

	// A service worker outlives the page that registered it, so it could
	// keep answering on this host after the port or files change. Browsers
	// send this header only for service worker scripts.
	if r.Header.Get("Service-Worker") == "script" {
		writePreviewError(rw, r, s.DashboardURL, http.StatusForbidden, "Preview pages cannot register service workers.")
		return
	}

	if s.isFromOtherPreview(r, host) {
		writeFromOtherPreview(rw, r, s.DashboardURL)
		return
	}

	app, _, err := s.Previews.Parse(host)
	if err != nil {
		writePreviewError(rw, r, s.DashboardURL, http.StatusBadRequest, "This is not a valid preview address.")
		return
	}

	// A preview loaded as the top-level page has no wrapper to keep it on
	// its own host, so it is refused. Fetch metadata is missing from
	// non-browser clients and from plain-HTTP origins other than localhost,
	// so a request without it is treated as a page load. Chromium sends no
	// fetch metadata with a WebSocket handshake, which never loads a page.
	dest := r.Header.Get("Sec-Fetch-Dest")
	if dest == "document" || (dest == "" && !httpapi.IsWebsocketUpgrade(r)) {
		description := "Previews open only inside a Coder chat. Open this preview from its chat."
		if dest == "" && s.AccessURL.Scheme == "http" && !isLocalhostName(host) {
			description = "Previews need an HTTPS access URL."
		}
		writePreviewError(rw, r, s.DashboardURL, http.StatusForbidden, description)
		return
	}

	// The smuggled API key sets a cookie on the whole app wildcard, which a
	// preview must never do.
	if r.URL.Query().Has(SubdomainProxyAPIKeyParam) {
		writePreviewError(rw, r, s.DashboardURL, http.StatusBadRequest, "Previews sign in only from the dashboard.")
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

// isLocalhostName reports whether host, with or without a port, is localhost
// or a subdomain of it, the only plain-HTTP hosts that browsers send fetch
// metadata to. A preview host is never an IP address.
func isLocalhostName(host string) bool {
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		host = hostname
	}
	host = strings.ToLower(host)
	return host == "localhost" || strings.HasSuffix(host, ".localhost")
}

func writePreviewError(rw http.ResponseWriter, r *http.Request, dashboardURL *url.URL, status int, description string) {
	site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
		Status:      status,
		Title:       http.StatusText(status),
		Description: description,
		Actions:     []site.Action{{URL: dashboardURL.String(), Text: "Back to site"}},
	})
}

// previewSignInNeeded is what a preview host's signed-out page posts to the
// dashboard that frames it. The dashboard accepts it only from the preview
// frame's origin and window, and then signs the preview in.
const previewSignInNeeded = "coder-preview-sign-in-needed"

// writePreviewSignedOut answers a preview request that has no session.
func writePreviewSignedOut(rw http.ResponseWriter, r *http.Request, dashboardURL *url.URL) {
	// JSEscapeString escapes quotes, <, > and &, so its strings are safe in a
	// quoted script string.
	message := `"` + template.JSEscapeString(previewSignInNeeded) + `"`
	dashboardOrigin := `"` + template.JSEscapeString(dashboardURL.Scheme+"://"+dashboardURL.Host) + `"`
	site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
		Status: http.StatusUnauthorized,
		Title:  http.StatusText(http.StatusUnauthorized),
		Description: "<p>Reopen this preview from Coder to sign in again.</p>" +
			"<script>window.top.postMessage(" + message + ", " + dashboardOrigin + ")</script>",
		RenderDescriptionMarkdown: true,
		Actions:                   []site.Action{{URL: dashboardURL.String(), Text: "Back to site"}},
	})
}

func writeFromOtherPreview(rw http.ResponseWriter, r *http.Request, dashboardURL *url.URL) {
	writePreviewError(rw, r, dashboardURL, http.StatusForbidden, "Preview pages cannot send requests to other apps.")
}

// isFromOtherPreview reports whether a page on a preview host other than host
// sent r. The browser sets Origin, so a preview page cannot forge it, and it
// adds the user's cookies for the target, so the request would act as the user.
// The preview CSP already blocks such requests; refusing them here also blocks
// those whose Origin names another preview host where a browser does not enforce
// it. Only s's own wildcard pattern tells a preview host from an app host, so
// previews on other servers, such as workspace proxies, are left to their CSP.
func (s *Server) isFromOtherPreview(r *http.Request, host string) bool {
	origin, err := url.Parse(r.Header.Get(httpmw.OriginHeader))
	if err != nil || origin.Host == "" || s.HostnameRegex == nil || appurl.HostnamesMatch(origin.Host, host) {
		return false
	}
	_, _, ok := appurl.ParsePreviewHostname(s.HostnameRegex, origin.Host)
	return ok
}
