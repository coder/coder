package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/pion/webrtc/v3"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpmw"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
)

type WorkspaceAgentStatus string

const (
	WorkspaceAgentWaiting      WorkspaceAgentStatus = "waiting"
	WorkspaceAgentConnected    WorkspaceAgentStatus = "connected"
	WorkspaceAgentDisconnected WorkspaceAgentStatus = "disconnected"
)

type WorkspaceResource struct {
	ID         uuid.UUID                    `json:"id"`
	CreatedAt  time.Time                    `json:"created_at"`
	JobID      uuid.UUID                    `json:"job_id"`
	Transition database.WorkspaceTransition `json:"workspace_transition"`
	Address    string                       `json:"address"`
	Type       string                       `json:"type"`
	Name       string                       `json:"name"`
	Agent      *WorkspaceAgent              `json:"agent,omitempty"`
}

type WorkspaceAgent struct {
	ID                   uuid.UUID            `json:"id"`
	CreatedAt            time.Time            `json:"created_at"`
	UpdatedAt            time.Time            `json:"updated_at"`
	FirstConnectedAt     *time.Time           `json:"first_connected_at,omitempty"`
	LastConnectedAt      *time.Time           `json:"last_connected_at,omitempty"`
	DisconnectedAt       *time.Time           `json:"disconnected_at,omitempty"`
	Status               WorkspaceAgentStatus `json:"status"`
	ResourceID           uuid.UUID            `json:"resource_id"`
	InstanceID           string               `json:"instance_id,omitempty"`
	EnvironmentVariables map[string]string    `json:"environment_variables"`
	StartupScript        string               `json:"startup_script,omitempty"`
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
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceresources/%s", id), nil)
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

// DialWorkspaceAgent creates a connection to the specified resource.
func (c *Client) DialWorkspaceAgent(ctx context.Context, resource uuid.UUID) (proto.DRPCPeerBrokerClient, error) {
	serverURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceresources/%s/dial", resource.String()))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{{
		Name:  httpmw.AuthCookie,
		Value: c.SessionToken,
	}})
	httpClient := &http.Client{
		Jar: jar,
	}
	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, readBodyAsError(res)
	}
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Client(websocket.NetConn(ctx, conn, websocket.MessageBinary), config)
	if err != nil {
		return nil, xerrors.Errorf("multiplex client: %w", err)
	}
	return proto.NewDRPCPeerBrokerClient(provisionersdk.Conn(session)), nil
}

// ListenWorkspaceAgent connects as a workspace agent.
// It obtains the agent ID based off the session token.
func (c *Client) ListenWorkspaceAgent(ctx context.Context, opts *peer.ConnOptions) (*peerbroker.Listener, error) {
	serverURL, err := c.URL.Parse("/api/v2/workspaceresources/agent")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{{
		Name:  httpmw.AuthCookie,
		Value: c.SessionToken,
	}})
	httpClient := &http.Client{
		Jar: jar,
	}
	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, readBodyAsError(res)
	}
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Client(websocket.NetConn(ctx, conn, websocket.MessageBinary), config)
	if err != nil {
		return nil, xerrors.Errorf("multiplex client: %w", err)
	}
	return peerbroker.Listen(session, func(ctx context.Context) ([]webrtc.ICEServer, error) {
		return []webrtc.ICEServer{{
			URLs: []string{"stun:stun.l.google.com:19302"},
		}}, nil
	}, opts)
}
