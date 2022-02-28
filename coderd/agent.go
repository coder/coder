package coderd

import (
	"fmt"
	"io"
	"net/http"

	"github.com/hashicorp/yamux"
	"nhooyr.io/websocket"

	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
)

type AgentResourceMetadata struct {
	MemoryTotal uint64  `json:"memory_total"`
	DiskTotal   uint64  `json:"disk_total"`
	CPUCores    uint64  `json:"cpu_cores"`
	CPUModel    string  `json:"cpu_model"`
	CPUMhz      float64 `json:"cpu_mhz"`
}

type AgentInstanceMetadata struct {
	JailOrchestrator   string `json:"jail_orchestrator"`
	OperatingSystem    string `json:"operating_system"`
	Platform           string `json:"platform"`
	PlatformFamily     string `json:"platform_family"`
	KernelVersion      string `json:"kernel_version"`
	KernelArchitecture string `json:"kernel_architecture"`
	Cloud              string `json:"cloud"`
	Jail               string `json:"jail"`
	VNC                bool   `json:"vnc"`
}

func (api *api) agentUpdate() {

}

func (api *api) agentConnectByResource(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitGroup.Add(1)
	defer api.websocketWaitGroup.Done()

	agent := httpmw.Agent(r)
	if !agent.UpdatedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionRequired, httpapi.Response{
			Message: "Agent hasn't connected yet!",
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

func (api *api) agentServe(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitGroup.Add(1)
	defer api.websocketWaitGroup.Done()

	agent := httpmw.Agent(r)
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
	<-r.Context().Done()
}
