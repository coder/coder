package coderd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
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
	// workspaceAgentFilesMaxFileBytes caps the write-file request body and
	// the read-file response body, which bounds the size of a file the
	// browser can write or read.
	workspaceAgentFilesMaxFileBytes = 32 << 20
	workspaceAgentFilesDialTimeout  = 30 * time.Second
	// find-files may walk a whole root before it answers.
	workspaceAgentFilesResponseHeaderTimeout = 2 * time.Minute
	// workspaceAgentFilesExchangeTimeout bounds the whole HTTP exchange with
	// the agent, body transfer included, and leaves room for a file of
	// workspaceAgentFilesMaxFileBytes.
	workspaceAgentFilesExchangeTimeout = 5 * time.Minute
	// workspaceAgentFilesMaxErrorBytes caps the read-file error body that is
	// buffered when the agent sends no Content-Length.
	workspaceAgentFilesMaxErrorBytes = 64 << 10
	// Transport errors embed the agent's tailnet URL, so they are logged
	// rather than returned to the client.
	workspaceAgentFilesUnreachableDetail = "The workspace agent could not be reached. Check that the workspace agent is connected and retry."
)

// The agent runs code the workspace owner controls, and the requester may be
// someone else with SSH access, such as an admin, so only the headers the
// file API needs cross the proxy. Query parameters are allowlisted per route
// as well, because coderd accepts session tokens as query parameters.
var (
	workspaceAgentFilesRequestHeaders  = []string{"Content-Type"}
	workspaceAgentFilesResponseHeaders = []string{"Content-Type", "Content-Length", "ETag"}
)

// workspaceAgentFilesRoute is an agent file API endpoint that coderd
// forwards to.
type workspaceAgentFilesRoute struct {
	path        string
	queryParams []string
	// maxResponseBytes, when set, refuses larger responses without
	// relaying any of the body.
	maxResponseBytes int64
}

// @Summary List workspace agent directory
// @ID list-workspace-agent-directory
// @Security CoderSessionToken
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Success 200
// @Router /api/experimental/workspaceagents/{workspaceagent}/files/list-directory [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentFilesListDirectory(rw http.ResponseWriter, r *http.Request) {
	api.proxyWorkspaceAgentFiles(rw, r, workspaceAgentFilesRoute{
		path:        "/api/v0/list-directory",
		queryParams: []string{"path"},
	})
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
	api.proxyWorkspaceAgentFiles(rw, r, workspaceAgentFilesRoute{
		path:             "/api/v0/read-file",
		queryParams:      []string{"path", "offset", "limit"},
		maxResponseBytes: workspaceAgentFilesMaxFileBytes,
	})
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
	api.proxyWorkspaceAgentFiles(rw, r, workspaceAgentFilesRoute{
		path:        "/api/v0/write-file",
		queryParams: []string{"path", "if_match"},
	})
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
	api.proxyWorkspaceAgentFiles(rw, r, workspaceAgentFilesRoute{
		path:        "/api/v0/find-files",
		queryParams: []string{"root", "q", "limit"},
	})
}

// proxyWorkspaceAgentFiles authorizes the request, dials the workspace agent
// and forwards the request to route on the agent's HTTP API.
func (api *API) proxyWorkspaceAgentFiles(rw http.ResponseWriter, r *http.Request, route workspaceAgentFilesRoute) {
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

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			DisableKeepAlives: true,
			// The transport detaches dials from the request context, so
			// the dial needs its own deadline.
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				ctx, cancel := context.WithTimeout(ctx, workspaceAgentFilesDialTimeout)
				defer cancel()
				return agentConn.DialContext(ctx, network, addr)
			},
			ResponseHeaderTimeout: workspaceAgentFilesResponseHeaderTimeout,
		},
	}
	// Cancels a dial that a canceled request left behind.
	defer client.CloseIdleConnections()

	agentURL := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(tailnet.TailscaleServicePrefix.AddrFromUUID(waws.WorkspaceAgent.ID).String(), strconv.Itoa(workspacesdk.AgentHTTPAPIServerPort)),
	}
	api.forwardWorkspaceAgentFilesRequest(rw, r, client, agentURL, route)
}

// forwardWorkspaceAgentFilesRequest sends r's body and allowlisted headers
// and query parameters to route on agentURL through client and copies the
// status, allowlisted headers and body of the response to rw.
func (api *API) forwardWorkspaceAgentFilesRequest(rw http.ResponseWriter, r *http.Request, client *http.Client, agentURL *url.URL, route workspaceAgentFilesRoute) {
	ctx := r.Context()

	if r.ContentLength > workspaceAgentFilesMaxFileBytes {
		writeWorkspaceAgentFilesRequestTooLarge(ctx, rw)
		return
	}

	var body io.Reader = http.NoBody
	var limitedBody *workspaceAgentFilesRequestBody
	if r.ContentLength != 0 {
		// The limit caps the bytes forwarded to the agent, which may
		// answer before the limit trips.
		limitedBody = &workspaceAgentFilesRequestBody{
			ReadCloser: http.MaxBytesReader(rw, r.Body, workspaceAgentFilesMaxFileBytes),
		}
		body = limitedBody
	}
	u := *agentURL
	u.Path = route.path
	u.RawQuery = filterWorkspaceAgentFilesQuery(r.URL.RawQuery, route.queryParams)
	exchangeCtx, cancel := context.WithTimeout(ctx, workspaceAgentFilesExchangeTimeout)
	defer cancel()
	outReq, err := http.NewRequestWithContext(exchangeCtx, r.Method, u.String(), body)
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
	if limitedBody != nil && limitedBody.exceeded.Load() {
		if err == nil {
			_ = resp.Body.Close()
		}
		writeWorkspaceAgentFilesRequestTooLarge(ctx, rw)
		return
	}
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		api.Logger.Warn(ctx, "proxy workspace agent files request", slog.F("path", route.path), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: "Failed to reach workspace agent.",
			Detail:  workspaceAgentFilesUnreachableDetail,
		})
		return
	}
	defer resp.Body.Close()

	var respBody io.Reader = resp.Body
	if route.maxResponseBytes > 0 {
		if resp.ContentLength > route.maxResponseBytes {
			writeWorkspaceAgentFilesResponseTooLarge(ctx, rw, route.maxResponseBytes)
			return
		}
		if resp.ContentLength < 0 {
			// The agent sets Content-Length on every successful read, so
			// only a small error body is buffered to learn its length.
			if resp.StatusCode/100 == 2 {
				httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
					Message: "The workspace agent sent file contents without a Content-Length.",
				})
				return
			}
			data, err := io.ReadAll(io.LimitReader(resp.Body, workspaceAgentFilesMaxErrorBytes+1))
			if err != nil || len(data) > workspaceAgentFilesMaxErrorBytes {
				httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
					Message: "The workspace agent sent an unreadable or oversized error response.",
				})
				return
			}
			rw.Header().Set("Content-Length", strconv.Itoa(len(data)))
			respBody = bytes.NewReader(data)
		}
	}

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
	if _, err := io.Copy(rw, respBody); err != nil && ctx.Err() == nil {
		api.Logger.Debug(ctx, "copy workspace agent files response", slog.F("path", route.path), slog.Error(err))
	}
}

// filterWorkspaceAgentFilesQuery returns the parameters of rawQuery named in
// allowed, in their original order and re-encoded.
func filterWorkspaceAgentFilesQuery(rawQuery string, allowed []string) string {
	var kept []string
	for part := range strings.SplitSeq(rawQuery, "&") {
		// url.ParseQuery skips these, so coderd never read them either.
		if part == "" || strings.Contains(part, ";") {
			continue
		}
		rawKey, rawValue, _ := strings.Cut(part, "=")
		key, err := url.QueryUnescape(rawKey)
		if err != nil || !slices.Contains(allowed, key) {
			continue
		}
		value, err := url.QueryUnescape(rawValue)
		if err != nil {
			continue
		}
		kept = append(kept, url.QueryEscape(key)+"="+url.QueryEscape(value))
	}
	return strings.Join(kept, "&")
}

// workspaceAgentFilesRequestBody records whether the request body hit its
// size limit. The transport reads it on another goroutine.
type workspaceAgentFilesRequestBody struct {
	io.ReadCloser
	exceeded atomic.Bool
}

func (b *workspaceAgentFilesRequestBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		b.exceeded.Store(true)
	}
	return n, err
}

func writeWorkspaceAgentFilesRequestTooLarge(ctx context.Context, rw http.ResponseWriter) {
	httpapi.RecordRequestBodyLimit(ctx, workspaceAgentFilesMaxFileBytes)
	httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{
		Message: "Request body too large.",
		Detail:  fmt.Sprintf("Maximum request body size is %d bytes.", workspaceAgentFilesMaxFileBytes),
	})
}

func writeWorkspaceAgentFilesResponseTooLarge(ctx context.Context, rw http.ResponseWriter, limit int64) {
	httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{
		Message: "File too large.",
		Detail:  fmt.Sprintf("Maximum file size is %d bytes.", limit),
	})
}
