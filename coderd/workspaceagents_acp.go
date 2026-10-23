package coderd

import (
	"context"
	"io"
	"net/http"
	"time"

	"golang.org/x/crypto/ssh"

	"cdr.dev/slog/v3"
	"github.com/coder/websocket"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// workspaceAgentACP relays ACP bytes without owning the native conversation.
// Closing the connection stops only the workspace adapter's stdio process.
//
// @Summary Connect to a workspace ACP adapter (experimental)
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

	// Accept first so a foreign Origin cannot cause workspace command execution.
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

	// Start reading before dialing, with bounded backpressure until SSH is ready.
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

	fail := func(err error) {
		api.Logger.Debug(ctx, "workspace acp adapter failed", slog.F("agent_id", waws.WorkspaceAgent.ID), slog.Error(err))
		_ = ws.Close(websocket.StatusInternalError, "Workspace ACP adapter exited. Check coder-acp setup in the workspace.")
	}
	dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dialCancel()
	agentConn, release, err := api.agentProvider.AgentConn(dialCtx, waws.WorkspaceAgent.ID)
	if err != nil {
		fail(err)
		return
	}
	defer release()
	rawConn, err := agentConn.SSHTCPConn(dialCtx)
	if err != nil {
		fail(err)
		return
	}
	defer rawConn.Close()
	// Bound the handshake itself, before an SSH client exists to close.
	stopDial := context.AfterFunc(dialCtx, func() { _ = rawConn.Close() })
	defer stopDial()
	sshConn, channels, requests, err := ssh.NewClientConn(rawConn, "localhost:22", &ssh.ClientConfig{
		// The authenticated tailnet connection identifies the workspace agent.
		// #nosec G106
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		fail(err)
		return
	}
	sshClient := ssh.NewClient(sshConn, channels, requests)
	defer sshClient.Close()
	stopSSH := context.AfterFunc(ctx, func() { _ = sshClient.Close() })
	defer stopSSH()
	session, err := sshClient.NewSession()
	if err != nil {
		fail(err)
		return
	}
	defer session.Close()
	stdin, err := session.StdinPipe()
	if err != nil {
		fail(err)
		return
	}
	session.Stdout = transport
	// Diagnostics must never enter the protocol or expose workspace secrets in
	// control-plane logs. The adapter keeps its own workspace-local service log.
	session.Stderr = io.Discard
	// This fixed executable is configured by the workspace owner/template. No
	// browser-supplied command, shell argument, or environment is evaluated here.
	if err := session.Start("coder-acp acp"); err != nil {
		fail(err)
		return
	}
	if !stopDial() {
		fail(dialCtx.Err())
		return
	}
	dialCancel()
	if auditor := api.tunnelAuditor(r); auditor != nil {
		auditor.Audit(waws.WorkspaceAgent.ID, nil)
	}

	copyDone := make(chan struct{})
	go func() {
		defer close(copyDone)
		_, _ = io.Copy(stdin, input)
		_ = stdin.Close()
	}()
	err = session.Wait()
	if err != nil && ctx.Err() == nil {
		fail(err)
	} else {
		_ = ws.Close(websocket.StatusNormalClosure, "")
	}
	_ = input.Close()
	_ = sshClient.Close()
	<-copyDone
}
