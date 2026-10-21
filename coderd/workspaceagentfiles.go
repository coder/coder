package coderd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
)

const (
	// workspaceAgentFilesMaxRequestBytes caps the request body coderd streams
	// to the agent, which bounds the size of a single file write.
	workspaceAgentFilesMaxRequestBytes = 32 << 20
	workspaceAgentFilesDialTimeout     = 30 * time.Second
	// Transport errors embed the agent's tailnet URL, so they are logged
	// rather than returned to the client.
	workspaceAgentFilesUnreachableDetail = "The workspace agent could not be reached. Check that the workspace agent is connected and retry."
)

// The agent runs code the workspace owner controls, and the requester may be
// someone else with SSH access, such as an admin, so only the headers the
// file API needs cross the proxy in either direction.
var (
	workspaceAgentFilesRequestHeaders  = []string{"Content-Type"}
	workspaceAgentFilesResponseHeaders = []string{"Content-Type", "Content-Length", "ETag"}
)

// @Summary List workspace agent directory
// @ID list-workspace-agent-directory
// @Security CoderSessionToken
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Success 200
// @Router /api/experimental/workspaceagents/{workspaceagent}/files/list-directory [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentFilesListDirectory(rw http.ResponseWriter, r *http.Request) {
	api.proxyWorkspaceAgentFiles(rw, r, "/api/v0/list-directory")
}

// @Summary Read workspace agent file
// @ID read-workspace-agent-file
// @Security CoderSessionToken
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Success 200
// @Router /api/experimental/workspaceagents/{workspaceagent}/files/read-file [get]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentFilesReadFile(rw http.ResponseWriter, r *http.Request) {
	api.proxyWorkspaceAgentFiles(rw, r, "/api/v0/read-file")
}

// @Summary Write workspace agent file
// @ID write-workspace-agent-file
// @Security CoderSessionToken
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Success 200
// @Router /api/experimental/workspaceagents/{workspaceagent}/files/write-file [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentFilesWriteFile(rw http.ResponseWriter, r *http.Request) {
	api.proxyWorkspaceAgentFiles(rw, r, "/api/v0/write-file")
}

// @Summary Find workspace agent files
// @ID find-workspace-agent-files
// @Security CoderSessionToken
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Success 200
// @Router /api/experimental/workspaceagents/{workspaceagent}/files/find-files [get]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentFilesFindFiles(rw http.ResponseWriter, r *http.Request) {
	api.proxyWorkspaceAgentFiles(rw, r, "/api/v0/find-files")
}

// proxyWorkspaceAgentFiles authorizes the request, dials the workspace agent
// and forwards the request to agentPath on the agent's HTTP API.
func (api *API) proxyWorkspaceAgentFiles(rw http.ResponseWriter, r *http.Request, agentPath string) {
	ctx := r.Context()
	waws := httpmw.WorkspaceAgentAndWorkspaceParam(r)

	// Reading and writing arbitrary files is equivalent to shell access.
	if !api.Authorize(r, policy.ActionSSH, waws) {
		httpapi.ResourceNotFound(rw)
		return
	}

	agentStatus := waws.WorkspaceAgent.Status(dbtime.Now(), api.AgentInactiveDisconnectTimeout)
	if agentStatus.Status != database.WorkspaceAgentStatusConnected {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Agent state is %q, it must be in the %q state.", agentStatus.Status, database.WorkspaceAgentStatusConnected),
		})
		return
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, workspaceAgentFilesDialTimeout)
	defer dialCancel()
	agentConn, release, err := api.agentProvider.AgentConn(dialCtx, waws.WorkspaceAgent.ID)
	if err != nil {
		api.Logger.Warn(ctx, "dial workspace agent for files request", slog.F("agent_id", waws.WorkspaceAgent.ID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: "Failed to dial workspace agent.",
			Detail:  workspaceAgentFilesUnreachableDetail,
		})
		return
	}
	defer release()

	client := agentConn.AppHTTPClient()
	// The transport detaches dials from the request context. Closing idle
	// connections also cancels a dial that a canceled request left behind.
	defer client.CloseIdleConnections()

	target := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(tailnet.TailscaleServicePrefix.AddrFromUUID(waws.WorkspaceAgent.ID).String(), strconv.Itoa(workspacesdk.AgentHTTPAPIServerPort)),
		Path:   agentPath,
	}
	api.forwardWorkspaceAgentFilesRequest(rw, r, client, target)
}

// forwardWorkspaceAgentFilesRequest sends r's query string, body and
// allowlisted headers to target through client and copies the status,
// allowlisted headers and body of the response to rw.
func (api *API) forwardWorkspaceAgentFilesRequest(rw http.ResponseWriter, r *http.Request, client *http.Client, target *url.URL) {
	ctx := r.Context()

	if r.ContentLength > workspaceAgentFilesMaxRequestBytes {
		writeWorkspaceAgentFilesRequestTooLarge(ctx, rw)
		return
	}

	var body io.Reader = http.NoBody
	if r.ContentLength != 0 {
		body = http.MaxBytesReader(rw, r.Body, workspaceAgentFilesMaxRequestBytes)
	}
	u := *target
	u.RawQuery = r.URL.RawQuery
	outReq, err := http.NewRequestWithContext(ctx, r.Method, u.String(), body)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating workspace agent request.",
			Detail:  err.Error(),
		})
		return
	}
	outReq.ContentLength = r.ContentLength
	for _, name := range workspaceAgentFilesRequestHeaders {
		if v := r.Header.Values(name); len(v) > 0 {
			outReq.Header[http.CanonicalHeaderKey(name)] = v
		}
	}

	resp, err := client.Do(outReq)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			writeWorkspaceAgentFilesRequestTooLarge(ctx, rw)
			return
		}
		if ctx.Err() != nil {
			return
		}
		api.Logger.Warn(ctx, "proxy workspace agent files request", slog.F("path", target.Path), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: "Failed to reach workspace agent.",
			Detail:  workspaceAgentFilesUnreachableDetail,
		})
		return
	}
	defer resp.Body.Close()

	for _, name := range workspaceAgentFilesResponseHeaders {
		if v := resp.Header.Values(name); len(v) > 0 {
			rw.Header()[http.CanonicalHeaderKey(name)] = v
		}
	}
	// The agent picks Content-Type from the file name, so a file named
	// x.html would otherwise render as a page on the Coder origin when
	// opened directly.
	rw.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	rw.Header().Set("Content-Disposition", "attachment")
	rw.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(rw, resp.Body); err != nil && ctx.Err() == nil {
		api.Logger.Debug(ctx, "copy workspace agent files response", slog.F("path", target.Path), slog.Error(err))
	}
}

func writeWorkspaceAgentFilesRequestTooLarge(ctx context.Context, rw http.ResponseWriter) {
	httpapi.RecordRequestBodyLimit(ctx, workspaceAgentFilesMaxRequestBytes)
	httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{
		Message: "Request body too large.",
		Detail:  fmt.Sprintf("Maximum request body size is %d bytes.", workspaceAgentFilesMaxRequestBytes),
	})
}
