package coderd

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hashicorp/yamux"
	"nhooyr.io/websocket"

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

	resource := httpmw.WorkspaceResourceParam(r)
	if !resource.AgentID.Valid {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "resource doesn't have an agent",
		})
		return
	}
	agent, err := api.Database.GetProvisionerJobAgentByResourceID(r.Context(), resource.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job agent: %s", err),
		})
		return
	}
	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
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
		ChannelID: agent.ID.String(),
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

	agent := httpmw.WorkspaceAgent(r)
	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
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
		ChannelID: agent.ID.String(),
		Pubsub:    api.Pubsub,
		Logger:    api.Logger.Named("peerbroker-proxy-listen"),
	})
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}
	defer closer.Close()
	err = api.Database.UpdateProvisionerJobAgentByID(r.Context(), database.UpdateProvisionerJobAgentByIDParams{
		ID: agent.ID,
		UpdatedAt: sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		},
	})
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-session.CloseChan():
			return
		case <-ticker.C:
			err = api.Database.UpdateProvisionerJobAgentByID(r.Context(), database.UpdateProvisionerJobAgentByIDParams{
				ID: agent.ID,
				UpdatedAt: sql.NullTime{
					Time:  database.Now(),
					Valid: true,
				},
			})
			if err != nil {
				_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
				return
			}
		}
	}
}
