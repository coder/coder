package coderd

import (
	"net/http"

	"github.com/google/uuid"
	"nhooyr.io/websocket"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/util/apiversion"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
	agpl "github.com/coder/coder/v2/tailnet"
)

// @Summary Agent is legacy
// @ID agent-is-legacy
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param workspaceagent path string true "Workspace Agent ID" format(uuid)
// @Success 200 {object} wsproxysdk.AgentIsLegacyResponse
// @Router /workspaceagents/{workspaceagent}/legacy [get]
// @x-apidocgen {"skip": true}
func (api *API) agentIsLegacy(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	agentID, ok := httpmw.ParseUUIDParam(rw, r, "workspaceagent")
	if !ok {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing UUID in URL.",
		})
		return
	}

	node := (*api.AGPL.TailnetCoordinator.Load()).Node(agentID)
	httpapi.Write(ctx, rw, http.StatusOK, wsproxysdk.AgentIsLegacyResponse{
		Found: node != nil,
		Legacy: node != nil &&
			len(node.Addresses) > 0 &&
			node.Addresses[0].Addr() == codersdk.WorkspaceAgentIP,
	})
}

// @Summary Workspace Proxy Coordinate
// @ID workspace-proxy-coordinate
// @Security CoderSessionToken
// @Tags Enterprise
// @Success 101
// @Router /workspaceproxies/me/coordinate [get]
// @x-apidocgen {"skip": true}
func (api *API) workspaceProxyCoordinate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	version := "1.0"
	msgType := websocket.MessageText
	qv := r.URL.Query().Get("version")
	if qv != "" {
		version = qv
	}
	if err := agpl.CurrentVersion.Validate(version); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Unknown or unsupported API version",
			Validations: []codersdk.ValidationError{
				{Field: "version", Detail: err.Error()},
			},
		})
		return
	}
	maj, _, _ := apiversion.Parse(version)
	if maj >= 2 {
		// Versions 2+ use dRPC over a binary connection
		msgType = websocket.MessageBinary
	}

	api.AGPL.WebsocketWaitMutex.Lock()
	api.AGPL.WebsocketWaitGroup.Add(1)
	api.AGPL.WebsocketWaitMutex.Unlock()
	defer api.AGPL.WebsocketWaitGroup.Done()

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}

	ctx, nc := websocketNetConn(ctx, conn, msgType)
	defer nc.Close()

	id := uuid.New()
	err = api.tailnetService.ServeMultiAgentClient(ctx, version, nc, id)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, err.Error())
	} else {
		_ = conn.Close(websocket.StatusGoingAway, "")
	}
}
