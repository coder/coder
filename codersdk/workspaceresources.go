package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type WorkspaceAgentStatus string

const (
	WorkspaceAgentConnecting   WorkspaceAgentStatus = "connecting"
	WorkspaceAgentConnected    WorkspaceAgentStatus = "connected"
	WorkspaceAgentDisconnected WorkspaceAgentStatus = "disconnected"
)

type WorkspaceResource struct {
	ID         uuid.UUID           `json:"id"`
	CreatedAt  time.Time           `json:"created_at"`
	JobID      uuid.UUID           `json:"job_id"`
	Transition WorkspaceTransition `json:"workspace_transition"`
	Type       string              `json:"type"`
	Name       string              `json:"name"`
	Agents     []WorkspaceAgent    `json:"agents,omitempty"`
}

type WorkspaceAgent struct {
	ID                   uuid.UUID            `json:"id"`
	CreatedAt            time.Time            `json:"created_at"`
	UpdatedAt            time.Time            `json:"updated_at"`
	FirstConnectedAt     *time.Time           `json:"first_connected_at,omitempty"`
	LastConnectedAt      *time.Time           `json:"last_connected_at,omitempty"`
	DisconnectedAt       *time.Time           `json:"disconnected_at,omitempty"`
	Status               WorkspaceAgentStatus `json:"status"`
	Name                 string               `json:"name"`
	ResourceID           uuid.UUID            `json:"resource_id"`
	InstanceID           string               `json:"instance_id,omitempty"`
	Architecture         string               `json:"architecture"`
	EnvironmentVariables map[string]string    `json:"environment_variables"`
	OperatingSystem      string               `json:"operating_system"`
	StartupScript        string               `json:"startup_script,omitempty"`
	Directory            string               `json:"directory,omitempty"`
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

func (c *Client) WorkspaceResource(ctx context.Context, id uuid.UUID) (WorkspaceResource, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceresources/%s", id), nil)
	if err != nil {
		return WorkspaceResource{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceResource{}, readBodyAsError(res)
	}
	var resource WorkspaceResource
	return resource, json.NewDecoder(res.Body).Decode(&resource)
}
