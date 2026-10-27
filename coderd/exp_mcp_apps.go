package coderd

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
)

// MCP Apps (SEP-1865) host routes, behind the mcp-apps experiment. The
// browser shows a view in a sandboxed iframe of the agent chat right
// panel. coderd talks to the MCP server for the view, as the chat owner.
//
// EXPERIMENTAL: these routes are a prototype and can change.

// mcpAppEntrypointsTimeout bounds the listing of all entry points, so one
// slow server cannot block the Add panel menu.
const mcpAppEntrypointsTimeout = 10 * time.Second

// maxMCPAppEntrypointServers caps the servers of one listing request.
const maxMCPAppEntrypointServers = 32

// requireMCPAppOwner writes 403 unless the caller owns the chat. The
// MCP session uses the owner's MCP credentials, so a shared-chat viewer
// must not drive it.
func requireMCPAppOwner(rw http.ResponseWriter, r *http.Request, chat database.Chat) bool {
	if chat.OwnerID != httpmw.APIKey(r).UserID {
		httpapi.Write(r.Context(), rw, http.StatusForbidden, codersdk.Response{
			Message: "Only the chat owner can use MCP Apps views.",
		})
		return false
	}
	return true
}

// mcpAppSession checks the chat and the {mcpServer} URL parameter and
// connects. It writes the error response and returns ok=false on failure.
func (api *API) mcpAppSession(rw http.ResponseWriter, r *http.Request) (*mcpclient.AppSession, bool) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	if !api.requireChatDaemon(ctx, rw) || !requireMCPAppOwner(rw, r, chat) {
		return nil, false
	}
	configID, err := uuid.Parse(chi.URLParam(r, "mcpServer"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid MCP server ID.",
			Detail:  err.Error(),
		})
		return nil, false
	}
	session, _, err := api.chatDaemon.MCPAppSession(ctx, chat, configID)
	if err != nil {
		writeMCPAppError(ctx, api.Logger, rw, err)
		return nil, false
	}
	return session, true
}

func writeMCPAppError(ctx context.Context, logger slog.Logger, rw http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, chatd.ErrMCPServerNotAvailable), errors.Is(err, mcpclient.ErrAppToolNotFound):
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{Message: "Not found.", Detail: err.Error()})
	case errors.Is(err, mcpclient.ErrAppToolNotAllowed):
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "The tool does not allow calls from MCP Apps views.",
			Detail:  err.Error(),
		})
	case errors.Is(err, mcpclient.ErrAppViewInvalid), errors.Is(err, mcpclient.ErrAppResultTooLarge):
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: "The MCP server returned an unusable response.",
			Detail:  err.Error(),
		})
	default:
		logger.Warn(ctx, "mcp apps request failed", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: "The MCP server request failed.",
			Detail:  err.Error(),
		})
	}
}

// getChatMCPAppView returns the ui:// resource of an MCP server as an
// HTML page with a CSP sandbox header. The header gives the page an
// opaque origin, also when a user opens the URL as a top-level page.
// EXPERIMENTAL
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatMCPAppView(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, ok := api.mcpAppSession(rw, r)
	if !ok {
		return
	}
	defer session.Close()

	view, err := session.ReadView(ctx, r.URL.Query().Get("uri"))
	if err != nil {
		writeMCPAppError(ctx, api.Logger, rw, err)
		return
	}
	rw.Header().Set("Content-Security-Policy", mcpAppViewCSP(view.CSP, api.AccessURL))
	rw.Header().Set("Cache-Control", "no-store")
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("Referrer-Policy", "no-referrer")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(view.HTML)
}

// postChatMCPAppToolCall runs a tools/call request of a view. The tool
// visibility must include "app". EXPERIMENTAL
func (api *API) postChatMCPAppToolCall(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req codersdk.ChatMCPAppToolCallRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.Name == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "The tool name is required."})
		return
	}
	session, ok := api.mcpAppSession(rw, r)
	if !ok {
		return
	}
	defer session.Close()

	result, err := session.CallAppTool(ctx, req.Name, req.Arguments)
	if err != nil {
		writeMCPAppError(ctx, api.Logger, rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, result)
}

// postChatMCPAppOpen opens a thread entry point: coderd calls the tool
// with {} and returns the view data. No chat message changes.
// EXPERIMENTAL
func (api *API) postChatMCPAppOpen(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req codersdk.ChatMCPAppOpenRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.ToolName == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "The tool name is required."})
		return
	}
	session, ok := api.mcpAppSession(rw, r)
	if !ok {
		return
	}
	defer session.Close()

	app, err := session.OpenEntrypoint(ctx, req.ToolName)
	if err != nil {
		writeMCPAppError(ctx, api.Logger, rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, app)
}

// getChatMCPAppEntrypoints lists the thread entry points of the MCP
// servers in the mcp_server_id query parameters. The browser sends the
// composer selection. A server that fails is left out of the list.
// EXPERIMENTAL
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatMCPAppEntrypoints(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	if !api.requireChatDaemon(ctx, rw) || !requireMCPAppOwner(rw, r, chat) {
		return
	}
	var ids []uuid.UUID
	for _, raw := range r.URL.Query()["mcp_server_id"] {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid mcp_server_id.",
				Detail:  err.Error(),
			})
			return
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) > maxMCPAppEntrypointServers {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Too many MCP servers."})
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, mcpAppEntrypointsTimeout)
	defer cancel()
	// Each goroutine writes only its own index.
	perServer := make([][]codersdk.ChatMCPAppEntrypoint, len(ids))
	var eg errgroup.Group
	for i, id := range ids {
		eg.Go(func() error {
			session, cfg, err := api.chatDaemon.MCPAppSession(listCtx, chat, id)
			if err != nil {
				if !errors.Is(err, chatd.ErrMCPServerNotAvailable) {
					api.Logger.Debug(ctx, "skip MCP server for MCP Apps entry points",
						slog.F("mcp_server_config_id", id), slog.Error(err))
				}
				return nil
			}
			defer session.Close()
			entrypoints, err := session.Entrypoints(listCtx)
			if err != nil {
				api.Logger.Debug(ctx, "list MCP Apps entry points",
					slog.F("mcp_server_config_id", id), slog.Error(err))
				return nil
			}
			out := make([]codersdk.ChatMCPAppEntrypoint, 0, len(entrypoints))
			for _, e := range entrypoints {
				out = append(out, codersdk.ChatMCPAppEntrypoint{
					MCPServerConfigID: cfg.ID,
					ServerDisplayName: cfg.DisplayName,
					ToolName:          e.ToolName,
					Title:             e.Title,
					ResourceURI:       e.ResourceURI,
				})
			}
			perServer[i] = out
			return nil
		})
	}
	_ = eg.Wait()
	result := []codersdk.ChatMCPAppEntrypoint{}
	for _, list := range perServer {
		result = append(result, list...)
	}
	httpapi.Write(ctx, rw, http.StatusOK, result)
}

// mcpAppCSPOrigin matches the origins a view can declare: https with an
// optional leading wildcard label, or a loopback http origin for local
// development. Anything else (quotes, ';', paths, other schemes) is
// dropped, so a server cannot inject a CSP directive.
var mcpAppCSPOrigin = regexp.MustCompile(`^(https://(\*\.)?[a-z0-9-]+(\.[a-z0-9-]+)*(:[0-9]{1,5})?|http://(localhost|127\.0\.0\.1)(:[0-9]{1,5})?)$`)

// cleanMCPAppOrigins keeps the valid declared origins and drops the
// Coder origin, which a view must not reach.
func cleanMCPAppOrigins(origins []string, accessURL *url.URL) string {
	var coder string
	if accessURL != nil {
		coder = strings.ToLower(accessURL.Scheme + "://" + accessURL.Host)
	}
	var out []string
	for _, o := range origins {
		o = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(o)), "/")
		if o == coder || !mcpAppCSPOrigin.MatchString(o) || slices.Contains(out, o) {
			continue
		}
		out = append(out, o)
	}
	return strings.Join(out, " ")
}

// mcpAppViewCSP builds the CSP of a view page from its declared domains
// (_meta.ui.csp). The sandbox directive gives the page an opaque origin:
// no cookies, no storage, and no same-origin access to Coder.
func mcpAppViewCSP(csp mcpclient.AppViewCSP, accessURL *url.URL) string {
	res := cleanMCPAppOrigins(csp.ResourceDomains, accessURL)
	withRes := func(base string) string { return strings.TrimSpace(base + " " + res) }
	orNone := func(v string) string {
		if v == "" {
			return "'none'"
		}
		return v
	}
	return strings.Join([]string{
		"sandbox allow-scripts allow-forms",
		"default-src 'none'",
		withRes("script-src 'unsafe-inline'"),
		withRes("style-src 'unsafe-inline'"),
		withRes("img-src data: blob:"),
		withRes("font-src data:"),
		withRes("media-src data: blob:"),
		"connect-src " + orNone(cleanMCPAppOrigins(csp.ConnectDomains, accessURL)),
		"frame-src " + orNone(cleanMCPAppOrigins(csp.FrameDomains, accessURL)),
		"base-uri 'none'",
		"form-action 'none'",
		"object-src 'none'",
		"frame-ancestors 'self'",
	}, "; ")
}
