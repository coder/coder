package workspacesdk

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/websocket"
)

// ACPConfigValue is a selectable value advertised by a harness.
type ACPConfigValue struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Group       string `json:"group,omitempty"`
	GroupName   string `json:"group_name,omitempty"`
}

// ACPConfigOption describes an ACP session select option.
type ACPConfigOption struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description,omitempty"`
	CurrentValue string           `json:"current_value"`
	Category     string           `json:"category,omitempty"`
	Values       []ACPConfigValue `json:"values"`
}

// ACPHarness is the public discovery result. Commands stay on the agent.
type ACPHarness struct {
	Slug          string            `json:"slug"`
	DisplayName   string            `json:"display_name"`
	LoadSession   bool              `json:"load_session"`
	ResumeSession bool              `json:"resume_session"`
	ConfigOptions []ACPConfigOption `json:"config_options"`
	Error         string            `json:"error,omitempty"`
}

// ACPCursor identifies a position in an in-memory transcript.
type ACPCursor struct {
	GenerationID uuid.UUID `json:"generation_id"`
	Seq          uint64    `json:"seq"`
}

// ACPSessionID identifies a native session and supplies its recovery context.
// The agent uses it to load or resume sessions without local metadata storage.
type ACPSessionID struct {
	HarnessSlug      string `json:"harness_slug"`
	WorkingDirectory string `json:"working_directory"`
	SessionID        string `json:"session_id"`
}

// ACPCreateSessionRequest creates an idle session in an explicit directory.
type ACPCreateSessionRequest struct {
	HarnessSlug      string            `json:"harness_slug"`
	WorkingDirectory string            `json:"working_directory"`
	Config           map[string]string `json:"config,omitempty"`
}

// ACPMessageRequest supplies a prompt to an idle session.
type ACPMessageRequest struct {
	Text string `json:"text"`
}

// ACPMessageOutcome describes how a prompt was admitted.
type ACPMessageOutcome string

// ACPMessageOutcomePrompt means a prompt request was written to the adapter.
const ACPMessageOutcomePrompt ACPMessageOutcome = "prompt"

// ACPMessageResponse reports message admission, not turn completion.
type ACPMessageResponse struct {
	Outcome ACPMessageOutcome `json:"outcome"`
}

// ACPSessionStatus describes the lifecycle state of an ACP runtime session.
type ACPSessionStatus string

const (
	ACPSessionStatusInitializing ACPSessionStatus = "initializing"
	ACPSessionStatusIdle         ACPSessionStatus = "idle"
	ACPSessionStatusRunning      ACPSessionStatus = "running"
	ACPSessionStatusInterrupting ACPSessionStatus = "interrupting"
	ACPSessionStatusError        ACPSessionStatus = "error"
)

// ACPSession describes a runtime session without its transcript.
type ACPSession struct {
	Harness            ACPHarness        `json:"harness"`
	ID                 ACPSessionID      `json:"id"`
	HarnessDisplayName string            `json:"harness_display_name"`
	Config             map[string]string `json:"config,omitempty"`
	Status             ACPSessionStatus  `json:"status"`
	StopReason         string            `json:"stop_reason,omitempty"`
	Error              string            `json:"error,omitempty"`
	Cursor             ACPCursor         `json:"cursor"`
}

// ACPEventKind identifies the content of an ACP transcript or stream event.
type ACPEventKind string

const (
	ACPEventKindUpdate   ACPEventKind = "update"
	ACPEventKindStatus   ACPEventKind = "status"
	ACPEventKindError    ACPEventKind = "error"
	ACPEventKindReset    ACPEventKind = "reset"
	ACPEventKindSnapshot ACPEventKind = "snapshot"
)

// ACPEvent carries an ACP update, runtime status, or error.
type ACPEvent struct {
	Cursor  ACPCursor       `json:"cursor"`
	Kind    ACPEventKind    `json:"kind"`
	Text    string          `json:"text,omitempty"`
	Update  json.RawMessage `json:"update,omitempty"`
	Session *ACPSession     `json:"session,omitempty"`
}

// ACPSessionResponse carries rich history and its assistant-text projection.
type ACPSessionResponse struct {
	Session           ACPSession `json:"session"`
	Events            []ACPEvent `json:"events"`
	AssistantResponse string     `json:"assistant_response"`
}

// ACPReadOptions optionally selects the portion after a transcript cursor.
type ACPReadOptions struct {
	After *ACPCursor
}

func acpQuery(id ACPSessionID, after *ACPCursor) url.Values {
	v := url.Values{
		"harness_slug":      {id.HarnessSlug},
		"working_directory": {id.WorkingDirectory},
		"session_id":        {id.SessionID},
	}
	if after != nil {
		v.Set("generation_id", after.GenerationID.String())
		v.Set("after", strconv.FormatUint(after.Seq, 10))
	}
	return v
}

func acpRequest[T any](ctx context.Context, c *agentConn, method, path string, body any) (T, error) {
	var result T
	request := c.apiRequest
	if method == http.MethodPost {
		request = c.toolCallRequest
	}
	res, err := request(ctx, method, "/api/v0/acp"+path, body)
	if err != nil {
		return result, xerrors.Errorf("request ACP: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusAccepted {
		return result, codersdk.ReadBodyAsError(res)
	}
	return result, decodeAgentJSON(res, &result)
}

// ListACPHarnesses returns the cached agent-local harness catalog.
func (c *agentConn) ListACPHarnesses(ctx context.Context) ([]ACPHarness, error) {
	var result []ACPHarness
	res, err := c.apiRequest(ctx, http.MethodGet, "/api/v0/acp/harnesses", nil)
	if err != nil {
		return nil, xerrors.Errorf("list ACP harnesses: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, codersdk.ReadBodyAsError(res)
	}
	return result, decodeAgentJSON(res, &result)
}

// CreateACPSession creates an idle ACP session before its first prompt.
func (c *agentConn) CreateACPSession(ctx context.Context, req ACPCreateSessionRequest) (ACPSession, error) {
	return acpRequest[ACPSession](ctx, c, http.MethodPost, "/sessions", req)
}

// ReadACPSession returns the available transcript and runtime status.
func (c *agentConn) ReadACPSession(ctx context.Context, id ACPSessionID, opts ACPReadOptions) (ACPSessionResponse, error) {
	return acpRequest[ACPSessionResponse](ctx, c, http.MethodGet, "/sessions/session?"+acpQuery(id, opts.After).Encode(), nil)
}

// SendACPMessage writes a prompt to an idle session's adapter.
func (c *agentConn) SendACPMessage(ctx context.Context, id ACPSessionID, req ACPMessageRequest) (ACPMessageResponse, error) {
	return acpRequest[ACPMessageResponse](ctx, c, http.MethodPost, "/sessions/session/messages?"+acpQuery(id, nil).Encode(), req)
}

// InterruptACPSession requests cancellation while retaining the subprocess.
func (c *agentConn) InterruptACPSession(ctx context.Context, id ACPSessionID) (ACPSession, error) {
	return acpRequest[ACPSession](ctx, c, http.MethodPost, "/sessions/session/interrupt?"+acpQuery(id, nil).Encode(), nil)
}

// WatchACPSession replays history and streams subsequent ordered events through
// the first idle or error status. An already idle or failed baseline completes
// immediately after its snapshot. A stream ending without a terminal snapshot
// or status can be resumed using the last received cursor.
func (c *agentConn) WatchACPSession(ctx context.Context, logger slog.Logger, id ACPSessionID, opts ACPReadOptions) (<-chan ACPEvent, io.Closer, error) {
	host := net.JoinHostPort(c.agentAddress().String(), strconv.Itoa(AgentHTTPAPIServerPort))
	c.headersMu.RLock()
	headers := c.extraHeaders.Clone()
	c.headersMu.RUnlock()
	u := "http://" + host + "/api/v0/acp/sessions/session/stream?" + acpQuery(id, opts.After).Encode()
	conn, res, err := websocket.Dial(ctx, u, &websocket.DialOptions{HTTPClient: c.apiClient(ctx), HTTPHeader: headers})
	if res != nil && res.Body != nil {
		defer res.Body.Close()
	}
	if err != nil {
		if res != nil {
			return nil, nil, codersdk.ReadBodyAsError(res)
		}
		return nil, nil, err
	}
	conn.SetReadLimit(16 << 20)
	d := wsjson.NewDecoder[ACPEvent](conn, websocket.MessageText, logger)
	return d.Chan(), d, nil
}
