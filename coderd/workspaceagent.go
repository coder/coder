package coderd

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hashicorp/yamux"
	"nhooyr.io/websocket"

	"cdr.dev/slog"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
)

func (api *api) workspaceAgentConnectByResource(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitGroup.Add(1)
	defer api.websocketWaitGroup.Done()

	resource := httpmw.WorkspaceResource(r)
	if !resource.WorkspaceAgentID.Valid {
		httpapi.Write(rw, http.StatusPreconditionRequired, httpapi.Response{
			Message: "resource doesn't have an agent",
		})
		return
	}
	agent, err := api.Database.GetWorkspaceAgentByID(r.Context(), resource.WorkspaceAgentID.UUID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace agent: %s", err),
		})
		return
	}
	if !agent.UpdatedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionRequired, httpapi.Response{
			Message: "Agent hasn't connected yet!",
		})
		return
	}

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("accept websocket: %s", err),
		})
		return
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Server(websocket.NetConn(r.Context(), conn, websocket.MessageBinary), config)
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}
	err = peerbroker.ProxyListen(r.Context(), session, peerbroker.ProxyOptions{
		ChannelID: resource.WorkspaceAgentID.UUID.String(),
		Logger:    api.Logger.Named("peerbroker-proxy-dial"),
		Pubsub:    api.Pubsub,
	})
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, fmt.Sprintf("serve: %s", err))
		return
	}
}

func (api *api) workspaceAgentServe(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitGroup.Add(1)
	defer api.websocketWaitGroup.Done()

	workspaceAgent := httpmw.WorkspaceAgent(r)
	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("accept websocket: %s", err),
		})
		return
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Server(websocket.NetConn(r.Context(), conn, websocket.MessageBinary), config)
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}
	closer, err := peerbroker.ProxyDial(proto.NewDRPCPeerBrokerClient(provisionersdk.Conn(session)), peerbroker.ProxyOptions{
		ChannelID: workspaceAgent.ID.String(),
		Pubsub:    api.Pubsub,
		Logger:    api.Logger.Named("peerbroker-proxy-listen"),
	})
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}

	err = api.Database.UpdateWorkspaceAgentByID(r.Context(), database.UpdateWorkspaceAgentByIDParams{
		ID: workspaceAgent.ID,
		UpdatedAt: sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		},
	})
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}
	defer closer.Close()
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-ticker.C:
			err = api.Database.UpdateWorkspaceAgentByID(r.Context(), database.UpdateWorkspaceAgentByIDParams{
				ID: workspaceAgent.ID,
				UpdatedAt: sql.NullTime{
					Time:  database.Now(),
					Valid: true,
				},
			})
			if err != nil {
				api.Logger.Error(r.Context(), "update workspace agent by id", slog.Error(err), slog.F("id", workspaceAgent.ID.String()))
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}
