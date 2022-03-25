package coderd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
)

func (api *api) workspaceResource(rw http.ResponseWriter, r *http.Request) {
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	workspaceResource := httpmw.WorkspaceResourceParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), workspaceBuild.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "Job hasn't completed!",
		})
		return
	}
	var apiAgent *codersdk.WorkspaceAgent
	if workspaceResource.AgentID.Valid {
		agent, err := api.Database.GetWorkspaceAgentByResourceID(r.Context(), workspaceResource.ID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get provisioner job agent: %s", err),
			})
			return
		}
		convertedAgent, err := convertWorkspaceAgent(agent, api.AgentConnectionUpdateFrequency)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("convert provisioner job agent: %s", err),
			})
			return
		}
		apiAgent = &convertedAgent
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertWorkspaceResource(workspaceResource, apiAgent))
}

func (api *api) workspaceResourceDial(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitGroup.Add(1)
	defer api.websocketWaitGroup.Done()

	resource := httpmw.WorkspaceResourceParam(r)
	if !resource.AgentID.Valid {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "resource doesn't have an agent",
		})
		return
	}
	agent, err := api.Database.GetWorkspaceAgentByResourceID(r.Context(), resource.ID)
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

func (api *api) workspaceAgentListen(rw http.ResponseWriter, r *http.Request) {
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
	resource, err := api.Database.GetWorkspaceResourceByID(r.Context(), agent.ResourceID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("accept websocket: %s", err),
		})
		return
	}

	api.Logger.Info(r.Context(), "accepting agent", slog.F("resource", resource), slog.F("agent", agent))

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
	firstConnectedAt := agent.FirstConnectedAt
	if !firstConnectedAt.Valid {
		firstConnectedAt = sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		}
	}
	lastConnectedAt := sql.NullTime{
		Time:  database.Now(),
		Valid: true,
	}
	disconnectedAt := agent.DisconnectedAt
	updateConnectionTimes := func() error {
		err = api.Database.UpdateWorkspaceAgentConnectionByID(r.Context(), database.UpdateWorkspaceAgentConnectionByIDParams{
			ID:               agent.ID,
			FirstConnectedAt: firstConnectedAt,
			LastConnectedAt:  lastConnectedAt,
			DisconnectedAt:   disconnectedAt,
		})
		if err != nil {
			return err
		}
		return nil
	}

	defer func() {
		disconnectedAt = sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		}
		_ = updateConnectionTimes()
	}()

	err = updateConnectionTimes()
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}

	ticker := time.NewTicker(api.AgentConnectionUpdateFrequency)
	defer ticker.Stop()
	for {
		select {
		case <-session.CloseChan():
			return
		case <-ticker.C:
			lastConnectedAt = sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			}
			err = updateConnectionTimes()
			if err != nil {
				_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
				return
			}
		}
	}
}

func convertWorkspaceAgent(dbAgent database.WorkspaceAgent, agentUpdateFrequency time.Duration) (codersdk.WorkspaceAgent, error) {
	var envs map[string]string
	if dbAgent.EnvironmentVariables.Valid {
		err := json.Unmarshal(dbAgent.EnvironmentVariables.RawMessage, &envs)
		if err != nil {
			return codersdk.WorkspaceAgent{}, xerrors.Errorf("unmarshal: %w", err)
		}
	}
	agent := codersdk.WorkspaceAgent{
		ID:                   dbAgent.ID,
		CreatedAt:            dbAgent.CreatedAt,
		UpdatedAt:            dbAgent.UpdatedAt,
		ResourceID:           dbAgent.ResourceID,
		InstanceID:           dbAgent.AuthInstanceID.String,
		StartupScript:        dbAgent.StartupScript.String,
		EnvironmentVariables: envs,
	}
	if dbAgent.FirstConnectedAt.Valid {
		agent.FirstConnectedAt = &dbAgent.FirstConnectedAt.Time
	}
	if dbAgent.LastConnectedAt.Valid {
		agent.LastConnectedAt = &dbAgent.LastConnectedAt.Time
	}
	if dbAgent.DisconnectedAt.Valid {
		agent.DisconnectedAt = &dbAgent.DisconnectedAt.Time
	}
	switch {
	case !dbAgent.FirstConnectedAt.Valid:
		// If the agent never connected, it's waiting for the compute
		// to start up.
		agent.Status = codersdk.WorkspaceAgentWaiting
	case dbAgent.DisconnectedAt.Time.After(dbAgent.LastConnectedAt.Time):
		// If we've disconnected after our last connection, we know the
		// agent is no longer connected.
		agent.Status = codersdk.WorkspaceAgentDisconnected
	case agentUpdateFrequency*2 >= database.Now().Sub(dbAgent.LastConnectedAt.Time):
		// The connection updated it's timestamp within the update frequency.
		// We multiply by two to allow for some lag.
		agent.Status = codersdk.WorkspaceAgentConnected
	case database.Now().Sub(dbAgent.LastConnectedAt.Time) > agentUpdateFrequency*2:
		// The connection died without updating the last connected.
		agent.Status = codersdk.WorkspaceAgentDisconnected
	}

	return agent, nil
}
