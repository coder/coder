package coderd

import (
	"net/http"

	"github.com/google/uuid"
	"nhooyr.io/websocket"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/tailnet"
	"github.com/coder/coder/enterprise/wsproxy/wsproxysdk"
)

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

func (api *API) workspaceProxyCoordinate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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

	id := uuid.New()
	sub := (*api.AGPL.TailnetCoordinator.Load()).ServeMultiAgent(id)
	nc := websocket.NetConn(ctx, conn, websocket.MessageText)
	defer nc.Close()

	err = tailnet.ServeWorkspaceProxy(nc, sub)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, err.Error())
	}
}
