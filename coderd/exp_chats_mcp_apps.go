package coderd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const maxMCPAppHTMLBytes = 4 << 20

// @Summary Read chat MCP App resource
// @ID read-chat-mcp-app-resource
// @Security CoderSessionToken
// @Tags Chats
// @Produce text/html
// @Param chat path string true "Chat ID" format(uuid)
// @Param server query string true "Workspace MCP server name"
// @Param uri query string true "Declared ui:// resource URI"
// @Success 200 {string} string "Sandboxed MCP App HTML"
// @Failure 400 {object} codersdk.Response
// @Failure 404 {object} codersdk.Response
// @Failure 413 {object} codersdk.Response
// @Failure 415 {object} codersdk.Response
// @Failure 500 {object} codersdk.Response
// @Failure 502 {object} codersdk.Response
// @Router /api/v2/chats/{chat}/mcp-apps/resource [get]
// @x-apidocgen {"skip": true}
func (api *API) readChatMCPAppResource(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Experiments.Enabled(codersdk.ExperimentChatMCPApps) {
		httpapi.ResourceNotFound(rw)
		return
	}
	serverName, resourceURI := r.URL.Query().Get("server"), r.URL.Query().Get("uri")
	parsed, err := url.Parse(resourceURI)
	if err != nil || parsed.Scheme != "ui" || serverName == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "A server and ui:// resource URI are required."})
		return
	}
	chat := httpmw.ChatParam(r)
	if !chat.WorkspaceID.Valid || !chat.AgentID.Valid || api.chatDaemon == nil {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err := api.chatDaemon.ResolveMCPAppResource(ctx, chat.ID, serverName, resourceURI); err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
		} else {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to resolve MCP App resource.", Detail: err.Error()})
		}
		return
	}
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// The catalog is pinned to this agent, not the latest workspace build.
	conn, release, err := api.agentProvider.AgentConn(dialCtx, chat.AgentID.UUID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{Message: "Failed to dial workspace agent.", Detail: err.Error()})
		return
	}
	defer release()
	resource, err := conn.ReadMCPResource(ctx, workspacesdk.ReadMCPResourceRequest{ServerName: serverName, URI: resourceURI})
	if errors.Is(err, workspacesdk.ErrMCPResourceTooLarge) {
		httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{Message: "MCP App resource exceeds the size limit."})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{Message: "Failed to read MCP App resource.", Detail: err.Error()})
		return
	}
	mediaType, params, err := mime.ParseMediaType(resource.MimeType)
	if err != nil || (mediaType != "text/html+skybridge" && (mediaType != "text/html" || !strings.EqualFold(params["profile"], "mcp-app"))) {
		httpapi.Write(ctx, rw, http.StatusUnsupportedMediaType, codersdk.Response{Message: "Resource is not MCP App HTML."})
		return
	}
	html := []byte(resource.Text)
	if len(html) == 0 && resource.Blob != "" {
		if len(resource.Blob) > base64.StdEncoding.EncodedLen(maxMCPAppHTMLBytes) {
			httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{Message: "MCP App HTML exceeds 4 MiB."})
			return
		}
		html, err = base64.StdEncoding.DecodeString(resource.Blob)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{Message: "Invalid MCP App resource encoding."})
			return
		}
	}
	if len(html) > maxMCPAppHTMLBytes {
		httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{Message: "MCP App HTML exceeds 4 MiB."})
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Cache-Control", "no-store")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("Referrer-Policy", "no-referrer")
	rw.Header().Set("Content-Security-Policy", mcpAppCSP(resource.Meta))
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(html)
}

func mcpAppCSP(meta json.RawMessage) string {
	var policy struct {
		UI struct {
			CSP struct {
				ConnectDomains  []string `json:"connectDomains"`
				ResourceDomains []string `json:"resourceDomains"`
			} `json:"csp"`
		} `json:"ui"`
	}
	_ = json.Unmarshal(meta, &policy)
	resources := strings.Join(mcpAppOrigins(policy.UI.CSP.ResourceDomains), " ")
	connect := strings.Join(mcpAppOrigins(policy.UI.CSP.ConnectDomains), " ")
	if connect == "" {
		connect = "'none'"
	}
	// CSP sandbox also isolates the resource when opened outside the host iframe.
	return "sandbox allow-scripts; default-src 'none'; " +
		"script-src 'unsafe-inline' " + resources + "; " +
		"style-src 'unsafe-inline' " + resources + "; " +
		"img-src data: blob: " + resources + "; " +
		"font-src data: " + resources + "; " +
		"media-src data: blob: " + resources + "; " +
		"connect-src " + connect + "; " +
		"frame-ancestors 'self'; base-uri 'none'; form-action 'none'; frame-src 'none'; object-src 'none'"
}

func mcpAppOrigins(domains []string) []string {
	var origins []string
	for _, domain := range domains {
		if len(origins) == 16 {
			break
		}
		if len(domain) > 253 {
			continue
		}
		parsed, err := url.Parse(domain)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
			parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" ||
			(parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" ||
			strings.ContainsAny(domain, " \t\r\n;'\"*#\\") {
			continue
		}
		origins = append(origins, "https://"+parsed.Host)
	}
	return origins
}
