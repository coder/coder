package coderd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
)

// @Summary Connect to the workspace agent desktop
// @ID connect-to-the-workspace-agent-desktop
// @Security CoderSessionToken
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Success 101
// @Router /api/v2/workspaceagents/{workspaceagent}/desktop [get]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentDesktop(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		waws   = httpmw.WorkspaceAgentAndWorkspaceParam(r)
		logger = api.Logger.Named("workspace_desktop").With(slog.F("agent_id", waws.WorkspaceAgent.ID))
	)

	// Driving the desktop is equivalent to a shell on the workspace, so
	// require the same permission as SSH and the web terminal.
	if !api.Authorize(r, policy.ActionSSH, waws) {
		httpapi.ResourceNotFound(rw)
		return
	}

	apiAgent, err := db2sdk.WorkspaceAgent(
		api.DERPMap(), *api.TailnetCoordinator.Load(), waws.WorkspaceAgent, nil, nil, nil, api.AgentInactiveDisconnectTimeout,
		api.DeploymentValues.AgentFallbackTroubleshootingURL.String(),
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	if apiAgent.Status != codersdk.WorkspaceAgentConnected {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Agent state is %q, it must be in the %q state.", apiAgent.Status, codersdk.WorkspaceAgentConnected),
		})
		return
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dialCancel()
	agentConn, release, err := api.agentProvider.AgentConn(dialCtx, waws.WorkspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error dialing workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	defer release()

	// Starting the desktop can take a while on first use because the
	// agent may have to download and unpack the runtime, so use the
	// request context rather than the dial timeout.
	desktopConn, err := agentConn.ConnectDesktopVNC(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to connect to the agent desktop.",
			Detail:  err.Error(),
		})
		return
	}
	defer desktopConn.Close()

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		logger.Error(ctx, "failed to accept websocket", slog.Error(err))
		return
	}
	// No read limit because RFB framebuffer updates can be large.
	conn.SetReadLimit(-1)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx, wsNetConn := workspaceapps.WebsocketNetConn(ctx, conn, websocket.MessageBinary)
	defer wsNetConn.Close()
	ctx = api.wsWatcher.Watch(ctx, logger, conn)

	agentssh.Bicopy(ctx, wsNetConn, desktopConn)
}
