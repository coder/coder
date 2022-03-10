package coderd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"cdr.dev/slog"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
)

type WorkspaceResource struct {
	ID         uuid.UUID                    `json:"id"`
	CreatedAt  time.Time                    `json:"created_at"`
	JobID      uuid.UUID                    `json:"job_id"`
	Transition database.WorkspaceTransition `json:"workspace_transition"`
	Type       string                       `json:"type"`
	Name       string                       `json:"name"`
	Agent      *WorkspaceAgent              `json:"agent,omitempty"`
}

type WorkspaceAgent struct {
	ID                   uuid.UUID         `json:"id"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
	ResourceID           uuid.UUID         `json:"resource_id"`
	InstanceID           string            `json:"instance_id,omitempty"`
	EnvironmentVariables map[string]string `json:"environment_variables"`
	StartupScript        string            `json:"startup_script,omitempty"`
}

type WorkspaceAgentResourceMetadata struct {
	MemoryTotal uint64  `json:"memory_total"`
	DiskTotal   uint64  `json:"disk_total"`
	CPUCores    uint64  `json:"cpu_cores"`
	CPUModel    string  `json:"cpu_model"`
	CPUMhz      float64 `json:"cpu_mhz"`
}

type WorkspaceAgentInstanceMetadata struct {
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
	var apiAgent *WorkspaceAgent
	if workspaceResource.AgentID.Valid {
		agent, err := api.Database.GetWorkspaceAgentByResourceID(r.Context(), workspaceResource.ID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get provisioner job agent: %s", err),
			})
			return
		}
		convertedAgent, err := convertWorkspaceAgent(agent)
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
	err = api.Database.UpdateWorkspaceAgentByID(r.Context(), database.UpdateWorkspaceAgentByIDParams{
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
			err = api.Database.UpdateWorkspaceAgentByID(r.Context(), database.UpdateWorkspaceAgentByIDParams{
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

func convertWorkspaceAgent(agent database.WorkspaceAgent) (WorkspaceAgent, error) {
	var envs map[string]string
	if agent.EnvironmentVariables.Valid {
		err := json.Unmarshal(agent.EnvironmentVariables.RawMessage, &envs)
		if err != nil {
			return WorkspaceAgent{}, xerrors.Errorf("unmarshal: %w", err)
		}
	}
	return WorkspaceAgent{
		ID:                   agent.ID,
		CreatedAt:            agent.CreatedAt,
		UpdatedAt:            agent.UpdatedAt.Time,
		ResourceID:           agent.ResourceID,
		InstanceID:           agent.AuthInstanceID.String,
		StartupScript:        agent.StartupScript.String,
		EnvironmentVariables: envs,
	}, nil
}
