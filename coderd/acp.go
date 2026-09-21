package coderd

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// proxyACP serves the ephemeral ACP session API, including WebSocket snapshots.
//
// @Summary Access ephemeral ACP sessions
// @ID access-acp-sessions
// @Security CoderSessionToken
// @Tags Chats
// @Param chat path string true "Parent chat ID" format(uuid)
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Param acppath path string true "Session operation"
// @Success 200 {object} codersdk.ACPSession
// @Router /api/v2/chats/{chat}/acp/agents/{workspaceagent}/sessions/{acppath} [get]
// @Router /api/v2/chats/{chat}/acp/agents/{workspaceagent}/sessions/{acppath} [post]
// @x-apidocgen {"skip": true}
func (api *API) proxyACP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		if origin := r.Header.Get("Origin"); origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || !strings.EqualFold(parsed.Host, r.Host) {
				httpapi.Forbidden(w)
				return
			}
		}
	}

	if r.Method != http.MethodGet && !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.Forbidden(w)
		return
	}
	workspace, ok := api.authorizeChatWorkspaceExec(w, r, chat, "ACP agents require a workspace.")
	if !ok {
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "workspaceagent"))
	if err != nil {
		httpapi.ResourceNotFound(w)
		return
	}
	agentWorkspace, err := api.Database.GetWorkspaceByAgentID(ctx, agentID)
	if err != nil || agentWorkspace.ID != workspace.ID || agentWorkspace.OrganizationID != chat.OrganizationID {
		httpapi.ResourceNotFound(w)
		return
	}
	suffix := strings.Trim(chi.URLParam(r, "*"), "/")
	// Only expose the PoC's session operations, not arbitrary agent endpoints.
	parts := strings.Split(suffix, "/")
	if suffix != "" {
		if _, err = uuid.Parse(parts[0]); err != nil || len(parts) > 2 {
			httpapi.ResourceNotFound(w)
			return
		}
		if len(parts) == 2 && parts[1] != "stream" && parts[1] != "wait" && parts[1] != "messages" && parts[1] != "interrupt" {
			httpapi.ResourceNotFound(w)
			return
		}
	}
	conn, release, err := api.agentProvider.AgentConn(ctx, agentID)
	if err != nil {
		httpapi.Write(ctx, w, 502, codersdk.Response{Message: "Workspace agent unavailable.", Detail: err.Error()})
		return
	}
	defer release()
	target := &url.URL{Scheme: "http", Host: fmt.Sprintf("agent:%d", workspacesdk.AgentHTTPAPIServerPort)}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{DialContext: conn.DialContext, DisableKeepAlives: true}
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = workspacesdk.ACPPath(chat.OrganizationID, chat.ID) + suffix
		req.URL.RawPath = ""
		query := req.URL.Query()
		query.Del(codersdk.SessionTokenCookie)
		req.URL.RawQuery = query.Encode()
		req.Host = target.Host
		req.Header.Del("Cookie")
		req.Header.Del("Authorization")
		req.Header.Del(codersdk.SessionTokenHeader)
		// Origin was checked against coderd; the internal host differs.
		req.Header.Del("Origin")
	}
	proxy.ServeHTTP(w, r)
}
