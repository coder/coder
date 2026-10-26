// Package agentnative owns experimental native coding-agent sessions in a
// workspace. Native harnesses own inference, tools, and conversation history.
package agentnative

import (
	"context"
	"encoding/json"
	"os/exec"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

// ErrUnsupported means a native operation cannot be performed safely.
var ErrUnsupported = xerrors.New("native operation is not supported")

// Session is a durable binding to a native conversation, not a model transcript.
// Adapters must not mutate a Session supplied by the manager.
type Session struct {
	ID                 string          `json:"sessionId"`
	NativeID           string          `json:"nativeSessionId"`
	Harness            string          `json:"harness"`
	Cwd                string          `json:"cwd"`
	Mode               string          `json:"mode"`
	StartedAt          int64           `json:"startedAt"`
	Archived           bool            `json:"archived"`
	ResumePending      bool            `json:"resumePending"`
	Fault              string          `json:"fault,omitempty"`
	State              json.RawMessage `json:"state,omitempty"`
	Cursor             uint64          `json:"cursor"`
	HistoryStartCursor uint64          `json:"historyStartCursor"`
	ActiveRequestID    string          `json:"activeRequestId,omitempty"`
	ActiveTurnID       string          `json:"activeTurnId,omitempty"`
	PendingInput       bool            `json:"pendingInput"`
}

// Event is a normalized native observation. Seq is assigned by the manager.
// The event types and data match the ACP projection, not the native wire format.
type Event struct {
	Seq  uint64         `json:"seq"`
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// Prompt identifies one dispatch attempt independently of a transport request.
type Prompt struct {
	ID   string `json:"requestId"`
	Text string `json:"text"`
}

// Command describes a native terminal attachment. Env may contain credentials
// and must only be returned to authenticated local clients, never the browser.
type Command struct {
	Path string   `json:"path"`
	Args []string `json:"args"`
	Env  []string `json:"env,omitempty"`
	Dir  string   `json:"dir"`
}

// Host gives adapters atomic access to durable observations and adapter receipts.
// An empty state leaves existing receipts unchanged. Updates must not call back
// into the adapter. Session returns an independent snapshot.
type Host interface {
	Session(id string) (Session, error)
	Publish(id string, events []Event, state json.RawMessage) error
}

// Backend is the native boundary. Resume restores access to an existing
// conversation without submitting any prompt. Close releases observation only;
// Shutdown is an explicit workspace-stop operation. Implementations serialize
// their native streams and allow cancellation of request contexts.
type Backend interface {
	Create(context.Context, string) (Session, error)
	Resume(context.Context, Session) error
	Prompt(context.Context, Session, Prompt) error
	Cancel(context.Context, Session, string) error
	SetMode(context.Context, Session, string) error
	Attach(context.Context, Session) (Command, error)
	Close() error
	Shutdown(context.Context) error
}

// BackendOptions configure a native adapter. Environment must include the
// workspace user's HOME and PATH. Command is injectable for process tests.
type BackendOptions struct {
	StateDir      string
	Environment   []string
	Host          Host
	Logger        slog.Logger
	Clock         quartz.Clock
	CallbackURL   string
	CallbackToken string
	Command       func(context.Context, string, ...string) *exec.Cmd
}
