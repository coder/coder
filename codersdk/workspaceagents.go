package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/websocket"
)

type WorkspaceAgentStatus string

// This is also in database/modelmethods.go and should be kept in sync.
const (
	WorkspaceAgentConnecting   WorkspaceAgentStatus = "connecting"
	WorkspaceAgentConnected    WorkspaceAgentStatus = "connected"
	WorkspaceAgentDisconnected WorkspaceAgentStatus = "disconnected"
	WorkspaceAgentTimeout      WorkspaceAgentStatus = "timeout"
)

// WorkspaceAgentLifecycle represents the lifecycle state of a workspace agent.
//
// The agent lifecycle starts in the "created" state, and transitions to
// "starting" when the agent reports it has begun preparing (e.g. started
// executing the startup script).
type WorkspaceAgentLifecycle string

// WorkspaceAgentLifecycle enums.
const (
	WorkspaceAgentLifecycleCreated         WorkspaceAgentLifecycle = "created"
	WorkspaceAgentLifecycleStarting        WorkspaceAgentLifecycle = "starting"
	WorkspaceAgentLifecycleStartTimeout    WorkspaceAgentLifecycle = "start_timeout"
	WorkspaceAgentLifecycleStartError      WorkspaceAgentLifecycle = "start_error"
	WorkspaceAgentLifecycleReady           WorkspaceAgentLifecycle = "ready"
	WorkspaceAgentLifecycleShuttingDown    WorkspaceAgentLifecycle = "shutting_down"
	WorkspaceAgentLifecycleShutdownTimeout WorkspaceAgentLifecycle = "shutdown_timeout"
	WorkspaceAgentLifecycleShutdownError   WorkspaceAgentLifecycle = "shutdown_error"
	WorkspaceAgentLifecycleOff             WorkspaceAgentLifecycle = "off"
)

// Starting returns true if the agent is in the process of starting.
func (l WorkspaceAgentLifecycle) Starting() bool {
	switch l {
	case WorkspaceAgentLifecycleCreated, WorkspaceAgentLifecycleStarting:
		return true
	default:
		return false
	}
}

// ShuttingDown returns true if the agent is in the process of shutting
// down or has shut down.
func (l WorkspaceAgentLifecycle) ShuttingDown() bool {
	switch l {
	case WorkspaceAgentLifecycleShuttingDown, WorkspaceAgentLifecycleShutdownTimeout, WorkspaceAgentLifecycleShutdownError, WorkspaceAgentLifecycleOff:
		return true
	default:
		return false
	}
}

// WorkspaceAgentLifecycleOrder is the order in which workspace agent
// lifecycle states are expected to be reported during the lifetime of
// the agent process. For instance, the agent can go from starting to
// ready without reporting timeout or error, but it should not go from
// ready to starting. This is merely a hint for the agent process, and
// is not enforced by the server.
var WorkspaceAgentLifecycleOrder = []WorkspaceAgentLifecycle{
	WorkspaceAgentLifecycleCreated,
	WorkspaceAgentLifecycleStarting,
	WorkspaceAgentLifecycleStartTimeout,
	WorkspaceAgentLifecycleStartError,
	WorkspaceAgentLifecycleReady,
	WorkspaceAgentLifecycleShuttingDown,
	WorkspaceAgentLifecycleShutdownTimeout,
	WorkspaceAgentLifecycleShutdownError,
	WorkspaceAgentLifecycleOff,
}

// WorkspaceAgentStartupScriptBehavior defines whether or not the startup script
// should be considered blocking or non-blocking. The blocking behavior means
// that the agent will not be considered ready until the startup script has
// completed and, for example, SSH connections will wait for the agent to be
// ready (can be overridden).
//
// Presently, non-blocking is the default, but this may change in the future.
// Deprecated: `coder_script` allows configuration on a per-script basis.
type WorkspaceAgentStartupScriptBehavior string

const (
	WorkspaceAgentStartupScriptBehaviorBlocking    WorkspaceAgentStartupScriptBehavior = "blocking"
	WorkspaceAgentStartupScriptBehaviorNonBlocking WorkspaceAgentStartupScriptBehavior = "non-blocking"
)

type WorkspaceAgentMetadataResult struct {
	CollectedAt time.Time `json:"collected_at" format:"date-time"`
	// Age is the number of seconds since the metadata was collected.
	// It is provided in addition to CollectedAt to protect against clock skew.
	Age   int64  `json:"age"`
	Value string `json:"value"`
	Error string `json:"error"`
}

// WorkspaceAgentMetadataDescription is a description of dynamic metadata the agent should report
// back to coderd. It is provided via the `metadata` list in the `coder_agent`
// block.
type WorkspaceAgentMetadataDescription struct {
	DisplayName string `json:"display_name"`
	Key         string `json:"key"`
	Script      string `json:"script"`
	Interval    int64  `json:"interval"`
	Timeout     int64  `json:"timeout"`
}

type WorkspaceAgentMetadata struct {
	Result      WorkspaceAgentMetadataResult      `json:"result"`
	Description WorkspaceAgentMetadataDescription `json:"description"`
}

type DisplayApp string

const (
	DisplayAppVSCodeDesktop  DisplayApp = "vscode"
	DisplayAppVSCodeInsiders DisplayApp = "vscode_insiders"
	DisplayAppWebTerminal    DisplayApp = "web_terminal"
	DisplayAppPortForward    DisplayApp = "port_forwarding_helper"
	DisplayAppSSH            DisplayApp = "ssh_helper"
)

type WorkspaceAgent struct {
	ID                   uuid.UUID               `json:"id" format:"uuid"`
	CreatedAt            time.Time               `json:"created_at" format:"date-time"`
	UpdatedAt            time.Time               `json:"updated_at" format:"date-time"`
	FirstConnectedAt     *time.Time              `json:"first_connected_at,omitempty" format:"date-time"`
	LastConnectedAt      *time.Time              `json:"last_connected_at,omitempty" format:"date-time"`
	DisconnectedAt       *time.Time              `json:"disconnected_at,omitempty" format:"date-time"`
	StartedAt            *time.Time              `json:"started_at,omitempty" format:"date-time"`
	ReadyAt              *time.Time              `json:"ready_at,omitempty" format:"date-time"`
	Status               WorkspaceAgentStatus    `json:"status"`
	LifecycleState       WorkspaceAgentLifecycle `json:"lifecycle_state"`
	Name                 string                  `json:"name"`
	ResourceID           uuid.UUID               `json:"resource_id" format:"uuid"`
	InstanceID           string                  `json:"instance_id,omitempty"`
	Architecture         string                  `json:"architecture"`
	EnvironmentVariables map[string]string       `json:"environment_variables"`
	OperatingSystem      string                  `json:"operating_system"`
	LogsLength           int32                   `json:"logs_length"`
	LogsOverflowed       bool                    `json:"logs_overflowed"`
	Directory            string                  `json:"directory,omitempty"`
	ExpandedDirectory    string                  `json:"expanded_directory,omitempty"`
	Version              string                  `json:"version"`
	APIVersion           string                  `json:"api_version"`
	Apps                 []WorkspaceApp          `json:"apps"`
	// DERPLatency is mapped by region name (e.g. "New York City", "Seattle").
	DERPLatency              map[string]DERPRegion     `json:"latency,omitempty"`
	ConnectionTimeoutSeconds int32                     `json:"connection_timeout_seconds"`
	TroubleshootingURL       string                    `json:"troubleshooting_url"`
	Subsystems               []AgentSubsystem          `json:"subsystems"`
	Health                   WorkspaceAgentHealth      `json:"health"` // Health reports the health of the agent.
	DisplayApps              []DisplayApp              `json:"display_apps"`
	LogSources               []WorkspaceAgentLogSource `json:"log_sources"`
	Scripts                  []WorkspaceAgentScript    `json:"scripts"`

	TaskWaitingForUserInput bool       `json:"task_waiting_for_user_input"`
	TaskNotifications       bool       `json:"task_notifications"`
	TaskCompletedAt         *time.Time `json:"task_completed_at,omitempty"`

	Tasks []WorkspaceAgentTask `json:"tasks"`

	// StartupScriptBehavior is a legacy field that is deprecated in favor
	// of the `coder_script` resource. It's only referenced by old clients.
	// Deprecated: Remove in the future!
	StartupScriptBehavior WorkspaceAgentStartupScriptBehavior `json:"startup_script_behavior"`
}

type WorkspaceAgentTask struct {
	ID         uuid.UUID `json:"id"`
	AgentID    uuid.UUID `json:"agent_id"`
	CreatedAt  time.Time `json:"created_at"`
	Reporter   string    `json:"reporter"`
	Summary    string    `json:"summary"`
	URL        string    `json:"url"`
	Icon       string    `json:"icon"`
	Completion bool      `json:"completion"`
}

type WorkspaceAgentLogSource struct {
	WorkspaceAgentID uuid.UUID `json:"workspace_agent_id" format:"uuid"`
	ID               uuid.UUID `json:"id" format:"uuid"`
	CreatedAt        time.Time `json:"created_at" format:"date-time"`
	DisplayName      string    `json:"display_name"`
	Icon             string    `json:"icon"`
}

type WorkspaceAgentScript struct {
	ID               uuid.UUID     `json:"id" format:"uuid"`
	LogSourceID      uuid.UUID     `json:"log_source_id" format:"uuid"`
	LogPath          string        `json:"log_path"`
	Script           string        `json:"script"`
	Cron             string        `json:"cron"`
	RunOnStart       bool          `json:"run_on_start"`
	RunOnStop        bool          `json:"run_on_stop"`
	StartBlocksLogin bool          `json:"start_blocks_login"`
	Timeout          time.Duration `json:"timeout"`
	DisplayName      string        `json:"display_name"`
}

type WorkspaceAgentHealth struct {
	Healthy bool   `json:"healthy" example:"false"`                              // Healthy is true if the agent is healthy.
	Reason  string `json:"reason,omitempty" example:"agent has lost connection"` // Reason is a human-readable explanation of the agent's health. It is empty if Healthy is true.
}

type DERPRegion struct {
	Preferred           bool    `json:"preferred"`
	LatencyMilliseconds float64 `json:"latency_ms"`
}

type WorkspaceAgentLog struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at" format:"date-time"`
	Output    string    `json:"output"`
	Level     LogLevel  `json:"level"`
	SourceID  uuid.UUID `json:"source_id" format:"uuid"`
}

type AgentSubsystem string

const (
	AgentSubsystemEnvbox     AgentSubsystem = "envbox"
	AgentSubsystemEnvbuilder AgentSubsystem = "envbuilder"
	AgentSubsystemExectrace  AgentSubsystem = "exectrace"
)

func (s AgentSubsystem) Valid() bool {
	switch s {
	case AgentSubsystemEnvbox, AgentSubsystemEnvbuilder, AgentSubsystemExectrace:
		return true
	default:
		return false
	}
}

// WatchWorkspaceAgentMetadata watches the metadata of a workspace agent.
// The returned channel will be closed when the context is canceled. Exactly
// one error will be sent on the error channel. The metadata channel is never closed.
func (c *Client) WatchWorkspaceAgentMetadata(ctx context.Context, id uuid.UUID) (<-chan []WorkspaceAgentMetadata, <-chan error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	metadataChan := make(chan []WorkspaceAgentMetadata, 256)

	ready := make(chan struct{})
	watch := func() error {
		res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/watch-metadata", id), nil)
		if err != nil {
			return err
		}
		if res.StatusCode != http.StatusOK {
			return ReadBodyAsError(res)
		}

		nextEvent := ServerSentEventReader(ctx, res.Body)
		defer res.Body.Close()

		firstEvent := true
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			sse, err := nextEvent()
			if err != nil {
				return err
			}

			if firstEvent {
				close(ready) // Only close ready after the first event is received.
				firstEvent = false
			}

			// Ignore pings.
			if sse.Type == ServerSentEventTypePing {
				continue
			}

			b, ok := sse.Data.([]byte)
			if !ok {
				return xerrors.Errorf("unexpected data type: %T", sse.Data)
			}

			switch sse.Type {
			case ServerSentEventTypeData:
				var met []WorkspaceAgentMetadata
				err = json.Unmarshal(b, &met)
				if err != nil {
					return xerrors.Errorf("unmarshal metadata: %w", err)
				}
				metadataChan <- met
			case ServerSentEventTypeError:
				var r Response
				err = json.Unmarshal(b, &r)
				if err != nil {
					return xerrors.Errorf("unmarshal error: %w", err)
				}
				return xerrors.Errorf("%+v", r)
			default:
				return xerrors.Errorf("unexpected event type: %s", sse.Type)
			}
		}
	}

	errorChan := make(chan error, 1)
	go func() {
		defer close(errorChan)
		err := watch()
		select {
		case <-ready:
		default:
			close(ready) // Error before first event.
		}
		errorChan <- err
	}()

	// Wait until first event is received and the subscription is registered.
	<-ready

	return metadataChan, errorChan
}

// WorkspaceAgent returns an agent by ID.
func (c *Client) WorkspaceAgent(ctx context.Context, id uuid.UUID) (WorkspaceAgent, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s", id), nil)
	if err != nil {
		return WorkspaceAgent{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgent{}, ReadBodyAsError(res)
	}
	var workspaceAgent WorkspaceAgent
	err = json.NewDecoder(res.Body).Decode(&workspaceAgent)
	if err != nil {
		return WorkspaceAgent{}, err
	}
	return workspaceAgent, nil
}

type IssueReconnectingPTYSignedTokenRequest struct {
	// URL is the URL of the reconnecting-pty endpoint you are connecting to.
	URL     string    `json:"url" validate:"required"`
	AgentID uuid.UUID `json:"agentID" format:"uuid" validate:"required"`
}

type IssueReconnectingPTYSignedTokenResponse struct {
	SignedToken string `json:"signed_token"`
}

func (c *Client) IssueReconnectingPTYSignedToken(ctx context.Context, req IssueReconnectingPTYSignedTokenRequest) (IssueReconnectingPTYSignedTokenResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/applications/reconnecting-pty-signed-token", req)
	if err != nil {
		return IssueReconnectingPTYSignedTokenResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return IssueReconnectingPTYSignedTokenResponse{}, ReadBodyAsError(res)
	}
	var resp IssueReconnectingPTYSignedTokenResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

type WorkspaceAgentListeningPortsResponse struct {
	// If there are no ports in the list, nothing should be displayed in the UI.
	// There must not be a "no ports available" message or anything similar, as
	// there will always be no ports displayed on platforms where our port
	// detection logic is unsupported.
	Ports []WorkspaceAgentListeningPort `json:"ports"`
}

type WorkspaceAgentListeningPort struct {
	ProcessName string `json:"process_name"` // may be empty
	Network     string `json:"network"`      // only "tcp" at the moment
	Port        uint16 `json:"port"`
}

// WorkspaceAgentListeningPorts returns a list of ports that are currently being
// listened on inside the workspace agent's network namespace.
func (c *Client) WorkspaceAgentListeningPorts(ctx context.Context, agentID uuid.UUID) (WorkspaceAgentListeningPortsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/listening-ports", agentID), nil)
	if err != nil {
		return WorkspaceAgentListeningPortsResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentListeningPortsResponse{}, ReadBodyAsError(res)
	}
	var listeningPorts WorkspaceAgentListeningPortsResponse
	return listeningPorts, json.NewDecoder(res.Body).Decode(&listeningPorts)
}

// WorkspaceAgentDevcontainer describes a devcontainer of some sort
// that is visible to the workspace agent. This struct is an abstraction
// of potentially multiple implementations, and the fields will be
// somewhat implementation-dependent.
type WorkspaceAgentDevcontainer struct {
	// CreatedAt is the time the container was created.
	CreatedAt time.Time `json:"created_at" format:"date-time"`
	// ID is the unique identifier of the container.
	ID string `json:"id"`
	// FriendlyName is the human-readable name of the container.
	FriendlyName string `json:"name"`
	// Image is the name of the container image.
	Image string `json:"image"`
	// Labels is a map of key-value pairs of container labels.
	Labels map[string]string `json:"labels"`
	// Running is true if the container is currently running.
	Running bool `json:"running"`
	// Ports includes ports exposed by the container.
	Ports []WorkspaceAgentListeningPort `json:"ports"`
	// Status is the current status of the container. This is somewhat
	// implementation-dependent, but should generally be a human-readable
	// string.
	Status string `json:"status"`
	// Volumes is a map of "things" mounted into the container. Again, this
	// is somewhat implementation-dependent.
	Volumes map[string]string `json:"volumes"`
}

// WorkspaceAgentListContainersResponse is the response to the list containers
// request.
type WorkspaceAgentListContainersResponse struct {
	// Containers is a list of containers visible to the workspace agent.
	Containers []WorkspaceAgentDevcontainer `json:"containers"`
	// Warnings is a list of warnings that may have occurred during the
	// process of listing containers. This should not include fatal errors.
	Warnings []string `json:"warnings,omitempty"`
}

func workspaceAgentContainersLabelFilter(kvs map[string]string) RequestOption {
	return func(r *http.Request) {
		q := r.URL.Query()
		for k, v := range kvs {
			kv := fmt.Sprintf("%s=%s", k, v)
			q.Add("label", kv)
		}
		r.URL.RawQuery = q.Encode()
	}
}

// WorkspaceAgentListContainers returns a list of containers that are currently
// running on a Docker daemon accessible to the workspace agent.
func (c *Client) WorkspaceAgentListContainers(ctx context.Context, agentID uuid.UUID, labels map[string]string) (WorkspaceAgentListContainersResponse, error) {
	lf := workspaceAgentContainersLabelFilter(labels)
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/containers", agentID), nil, lf)
	if err != nil {
		return WorkspaceAgentListContainersResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentListContainersResponse{}, ReadBodyAsError(res)
	}
	var cr WorkspaceAgentListContainersResponse

	return cr, json.NewDecoder(res.Body).Decode(&cr)
}

//nolint:revive // Follow is a control flag on the server as well.
func (c *Client) WorkspaceAgentLogsAfter(ctx context.Context, agentID uuid.UUID, after int64, follow bool) (<-chan []WorkspaceAgentLog, io.Closer, error) {
	var queryParams []string
	if after != 0 {
		queryParams = append(queryParams, fmt.Sprintf("after=%d", after))
	}
	if follow {
		queryParams = append(queryParams, "follow")
	}
	var query string
	if len(queryParams) > 0 {
		query = "?" + strings.Join(queryParams, "&")
	}
	reqURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/logs%s", agentID, query))
	if err != nil {
		return nil, nil, err
	}

	if !follow {
		resp, err := c.Request(ctx, http.MethodGet, reqURL.String(), nil)
		if err != nil {
			return nil, nil, xerrors.Errorf("execute request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, nil, ReadBodyAsError(resp)
		}

		var logs []WorkspaceAgentLog
		err = json.NewDecoder(resp.Body).Decode(&logs)
		if err != nil {
			return nil, nil, xerrors.Errorf("decode startup logs: %w", err)
		}

		ch := make(chan []WorkspaceAgentLog, 1)
		ch <- logs
		close(ch)
		return ch, closeFunc(func() error { return nil }), nil
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(reqURL, []*http.Cookie{{
		Name:  SessionTokenCookie,
		Value: c.SessionToken(),
	}})
	httpClient := &http.Client{
		Jar:       jar,
		Transport: c.HTTPClient.Transport,
	}
	conn, res, err := websocket.Dial(ctx, reqURL.String(), &websocket.DialOptions{
		HTTPClient:      httpClient,
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if res == nil {
			return nil, nil, err
		}
		return nil, nil, ReadBodyAsError(res)
	}
	d := wsjson.NewDecoder[[]WorkspaceAgentLog](conn, websocket.MessageText, c.logger)
	return d.Chan(), d, nil
}
