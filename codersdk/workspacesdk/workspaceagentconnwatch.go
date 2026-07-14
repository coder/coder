package workspacesdk

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/websocket"
)

type WatchErrorCode int

const (
	_ WatchErrorCode = iota // Ensure that zero value is not a valid code
	WatchErrorTooManyAgents
	WatchErrorNameNotFound
	WatchErrorNoAgents
	WatchErrorServerShutdown
	WatchErrorDatabase
	WatchErrorInternal
)

type ConnectionWatchEvent struct {
	Error       *WatchError  `json:"error"`
	BuildUpdate *BuildUpdate `json:"build_update,omitempty"`
	AgentUpdate *AgentUpdate `json:"agent_update,omitempty"`
}

type WatchError struct {
	Code      WatchErrorCode `json:"code"`
	Retryable bool           `json:"retryable"`
	Message   string         `json:"message"`
	Details   string         `json:"details,omitempty"`
}

func (e *WatchError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s", e.Message, e.Details)
	}
	return e.Message
}

type BuildUpdate struct {
	Transition codersdk.WorkspaceTransition  `json:"transition"`
	JobStatus  codersdk.ProvisionerJobStatus `json:"job_status"`
}

type AgentUpdate struct {
	Lifecycle codersdk.WorkspaceAgentLifecycle `json:"lifecycle"`
	ID        uuid.UUID                        `json:"id" format:"uuid"`
}

func (c *Client) WorkspaceAgentConnectionWatch(
	dialCtx context.Context, workspaceID uuid.UUID, agentName string,
) (
	dec *wsjson.Decoder[ConnectionWatchEvent], err error,
) {
	wsOptions := &websocket.DialOptions{
		HTTPClient: c.client.HTTPClient,
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
	}
	c.client.SessionTokenProvider.SetDialOption(wsOptions)

	watchURL, err := c.client.URL.Parse(fmt.Sprintf("/api/v2/workspaces/%s/agent-connection-watch", workspaceID))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	if agentName != "" {
		q := watchURL.Query()
		q.Set("agent_name", agentName)
		watchURL.RawQuery = q.Encode()
	}

	// nolint:bodyclose
	conn, res, err := websocket.Dial(dialCtx, watchURL.String(), wsOptions)
	if err != nil {
		bodyErr := codersdk.ReadBodyAsError(res)
		return nil, bodyErr
	}
	return wsjson.NewDecoder[ConnectionWatchEvent](conn, websocket.MessageText, c.client.Logger()), nil
}
