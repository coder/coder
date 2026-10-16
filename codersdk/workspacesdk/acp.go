package workspacesdk

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
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
	Epoch uuid.UUID `json:"epoch"`
	Seq   uint64    `json:"seq"`
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
