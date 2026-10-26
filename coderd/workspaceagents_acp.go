package coderd

import (
	"context"
	"io"
	"net/http"
	"time"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
)

// workspaceAgentACP relays ACP bytes without owning the native conversation.
// Closing the connection detaches observation without stopping native services.
//
// @Summary Connect to workspace native agents over ACP (experimental)
// @ID connect-workspace-agent-acp
// @Security CoderSessionToken
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Success 101
// @Failure 400 {object} codersdk.Response
// @Failure 401 {object} codersdk.Response
// @Failure 403 {object} codersdk.Response
// @Failure 404 {object} codersdk.Response
// @Router /api/v2/workspaceagents/{workspaceagent}/experimental/acp [get]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentACP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	waws := httpmw.WorkspaceAgentAndWorkspaceParam(r)
	if !api.Experiments.Enabled(codersdk.ExperimentNativeAgents) || !api.Authorize(r, policy.ActionSSH, waws) {
		httpapi.ResourceNotFound(rw)
		return
	}
	// Match the SSH transport restriction rather than creating an alternative
	// execution path around a deployment's browser-only policy.
	if override := api.WorkspaceClientCoordinateOverride.Load(); override != nil && *override != nil && (*override)(rw) {
		return
	}
	if waws.WorkspaceAgent.OperatingSystem != "linux" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Native workspace agents currently require Linux."})
		return
	}

	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()

	// Accept first so a foreign Origin cannot cause any workspace-side work.
	ws, err := websocket.Accept(rw, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = ws.CloseNow() }()
	ctx, transport := codersdk.WebsocketNetConn(ctx, ws, websocket.MessageBinary)
	defer transport.Close()
	ws.SetReadLimit(4 << 20)
	ctx = api.wsWatcher.Watch(ctx, api.Logger, ws)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start reading before dialing, with bounded backpressure until the agent is ready.
	input, output := io.Pipe()
	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		_, _ = io.Copy(output, transport)
		_ = output.Close()
		cancel()
	}()
	defer func() {
		_ = input.Close()
		_ = transport.Close()
		<-inputDone
	}()

	fail := func() {
		// Agent errors may contain local credentials. Do not log their details.
		api.Logger.Debug(ctx, "workspace native agents transport failed", slog.F("agent_id", waws.WorkspaceAgent.ID))
		_ = ws.Close(websocket.StatusInternalError, "Workspace native agents are unavailable.")
	}
	dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dialCancel()
	agentConn, release, err := api.agentProvider.AgentConn(dialCtx, waws.WorkspaceAgent.ID)
	if err != nil {
		fail()
		return
	}
	defer release()
	// The returned websocket retains its context for the whole connection.
	// Bound its handshake without attaching the dial deadline to its lifetime.
	stopDial := context.AfterFunc(dialCtx, cancel)
	defer stopDial()
	nativeConn, err := agentConn.ConnectNativeAgentACP(ctx)
	if err != nil {
		fail()
		return
	}
	defer nativeConn.Close()
	if !stopDial() {
		fail()
		return
	}
	dialCancel()
	stopNative := context.AfterFunc(ctx, func() { _ = nativeConn.Close() })
	defer stopNative()
	if auditor := api.tunnelAuditor(r); auditor != nil {
		auditor.Audit(waws.WorkspaceAgent.ID, nil)
	}

	copyDone := make(chan struct{})
	go func() {
		defer close(copyDone)
		_, _ = io.Copy(nativeConn, input)
		_ = nativeConn.Close()
	}()
	_, err = io.Copy(transport, nativeConn)
	if err != nil && ctx.Err() == nil {
		fail()
	} else {
		_ = ws.Close(websocket.StatusNormalClosure, "")
	}
	_ = input.Close()
	_ = nativeConn.Close()
	<-copyDone
}
