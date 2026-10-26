package agentnative

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/agent/agentexec"
)

//go:embed all:claudemod/hooks all:claudemod/.claude-plugin
var claudeAssets embed.FS

const (
	claudeStaleAfter          = 10 * time.Second
	claudeCommandTimeout      = 10 * time.Second
	claudeRegistrationTimeout = 30 * time.Second
	claudeEventLimit          = 128*1024 - 512
	claudeUnknown             = "native Claude command outcome is unknown; inspect the native terminal before retrying"
)

// Claude observes and controls a native Claude background session through a mod.
// Closing the observer never terminates native work.
type Claude struct {
	mu         sync.Mutex
	options    BackendOptions
	clock      quartz.Clock
	execer     agentexec.Execer
	plugin     string
	descriptor string
	root       string
	installed  bool
	closed     bool
	done       chan struct{}
	sessions   map[string]*claudeRuntime
	launches   map[string]*claudeLaunch
}

type claudeState struct {
	LaunchID        string                   `json:"launchId"`
	RuntimeID       string                   `json:"runtimeId"`
	Seq             uint64                   `json:"seq"`
	Retired         []string                 `json:"retired,omitempty"`
	Mode            string                   `json:"mode"`
	ActiveTurnID    string                   `json:"activeTurnId,omitempty"`
	OwnedRequestID  string                   `json:"ownedRequestId,omitempty"`
	OwnedTurnID     string                   `json:"ownedTurnId,omitempty"`
	Ended           bool                     `json:"ended,omitempty"`
	Fault           string                   `json:"fault,omitempty"`
	ResumeAttempted bool                     `json:"resumeAttempted,omitempty"`
	Receipts        map[string]claudeReceipt `json:"receipts,omitempty"`
}

type claudeReceipt struct {
	Kind   string `json:"kind"`
	Digest string `json:"digest,omitempty"`
	Result string `json:"result"`
}

type claudeRuntime struct {
	session    Session
	state      claudeState
	ready      bool
	fresh      bool
	lastSeen   time.Time
	commands   map[string]*claudePending
	registered chan struct{}
}

type claudeLaunch struct {
	cwd        string
	runtime    *claudeRuntime
	registered chan struct{}
}

type claudeCommand struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Mode      string `json:"mode,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

type claudePending struct {
	command claudeCommand
	done    chan struct{}
	result  error
}

type claudeHeartbeat struct {
	SessionID    string  `json:"sessionId"`
	RuntimeID    string  `json:"runtimeId"`
	LaunchID     string  `json:"launchId"`
	Cwd          string  `json:"cwd"`
	Mode         string  `json:"mode"`
	ActiveTurnID string  `json:"activeTurnId"`
	Ended        bool    `json:"ended"`
	Events       []Event `json:"events"`
}

var errClaudeLiveWorker = xerrors.New("matching native worker is still running")

var _ Backend = (*Claude)(nil)
var _ http.Handler = (*Claude)(nil)

// NewClaude creates an observer without starting Claude or contacting its daemon.
func NewClaude(options BackendOptions) (*Claude, error) {
	if options.Host == nil || !nativeValidPath(options.StateDir) {
		return nil, xerrors.New("Claude requires a host and an absolute private state directory")
	}
	u, err := url.Parse(options.CallbackURL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || options.CallbackToken == "" {
		return nil, xerrors.New("Claude requires an authenticated loopback callback URL")
	}
	if options.Clock == nil {
		options.Clock = quartz.NewReal()
	}
	options.Environment = slices.Clone(options.Environment)
	root, err := claudeConfigRoot(options.Environment)
	if err != nil {
		return nil, err
	}
	var execer agentexec.Execer
	if options.Command == nil {
		var err error
		execer, err = agentexec.NewExecer()
		if err != nil {
			return nil, xerrors.Errorf("create Claude command executor: %w", err)
		}
	}
	return &Claude{options: options, clock: options.Clock, execer: execer, plugin: filepath.Join(options.StateDir, "claude-plugin"), descriptor: filepath.Join(options.StateDir, "claude-bridge.json"), root: root, done: make(chan struct{}), sessions: make(map[string]*claudeRuntime), launches: make(map[string]*claudeLaunch)}, nil
}

func claudeConfigRoot(environment []string) (string, error) {
	home := claudeEnv(environment, "HOME")
	if !nativeValidPath(home) {
		return "", xerrors.New("Claude requires the workspace user's HOME")
	}
	root := filepath.Join(home, ".claude")
	if configured := claudeEnv(environment, "CLAUDE_CONFIG_DIR"); configured != "" && configured != root {
		return "", xerrors.Errorf("custom Claude config roots: %w", ErrUnsupported)
	}
	return root, nil
}

// installLocked refreshes the descriptor atomically. A live mod rereads this file
// on every poll, so agent restarts may rotate both the port and bearer token.
func (c *Claude) installLocked() error {
	if c.closed {
		return xerrors.New("Claude observer is closed")
	}
	if c.installed {
		return nil
	}
	if err := claudeMakePrivate(c.options.StateDir); err != nil {
		return err
	}
	if err := fs.WalkDir(claudeAssets, "claudemod", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel("claudemod", path)
		if err != nil {
			return err
		}
		target := filepath.Join(c.plugin, rel)
		if entry.IsDir() {
			return claudeMakePrivate(target)
		}
		data, err := claudeAssets.ReadFile(path)
		if err != nil {
			return err
		}
		return claudeWritePrivate(target, data)
	}); err != nil {
		return xerrors.Errorf("install Claude mod: %w", err)
	}
	data, err := json.Marshal(map[string]string{"url": strings.TrimRight(c.options.CallbackURL, "/"), "token": c.options.CallbackToken})
	if err != nil {
		return err
	}
	if err := claudeWritePrivate(c.descriptor, append(data, '\n')); err != nil {
		return xerrors.Errorf("write Claude descriptor: %w", err)
	}
	c.installed = true
	return nil
}

// Create starts a native background session without submitting a task.
func (c *Claude) Create(ctx context.Context, cwd string) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	if !nativeValidPath(cwd) {
		return Session{}, xerrors.New("invalid Claude working directory")
	}
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		return Session{}, xerrors.New("Claude working directory does not exist")
	}
	launchID := uuid.NewString()
	launch := &claudeLaunch{cwd: cwd, registered: make(chan struct{})}
	c.mu.Lock()
	if err := c.installLocked(); err != nil {
		c.mu.Unlock()
		return Session{}, err
	}
	if len(c.sessions)+len(c.launches) >= 100 {
		c.mu.Unlock()
		return Session{}, xerrors.New("Claude session budget exhausted")
	}
	c.launches[launchID] = launch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.launches, launchID); c.mu.Unlock() }()
	settings, _ := json.Marshal(map[string]any{"env": map[string]string{"WORKSPACE_BRIDGE_FILE": c.descriptor, "WORKSPACE_BRIDGE_LAUNCH_ID": launchID, "WORKSPACE_BRIDGE_MODE": "automation", "DISABLE_AUTOUPDATER": "1"}})
	if _, err := c.run(ctx, cwd, 20*time.Second, "--bg", "--name", "workspace-bridge-"+launchID[:8], "--plugin-dir", c.plugin, "--settings", string(settings), "/bridge-status"); err != nil {
		return Session{}, xerrors.New("native Claude launch outcome is unknown; inspect claude agents before retrying")
	}
	if err := c.wait(ctx, launch.registered, claudeRegistrationTimeout); err != nil {
		return Session{}, xerrors.Errorf("Claude registration pending; inspect claude agents before retrying: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return launch.runtime.session, nil
}

// Resume restores observation or resumes the exact native UUID, never a task.
func (c *Claude) Resume(ctx context.Context, session Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	if err := c.installLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	r, err := c.loadLocked(session)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	if r.ready && !r.state.Ended && !r.lastSeen.IsZero() && c.clock.Since(r.lastSeen) <= claudeStaleAfter {
		c.mu.Unlock()
		return nil
	}
	if r.fresh || (!r.lastSeen.IsZero() && c.clock.Since(r.lastSeen) <= claudeStaleAfter && !r.state.Ended) {
		state := claudeClone(r.state)
		if err := c.publishLocked(r, state, []Event{{Type: "runtime.resumed", Data: map[string]any{"sessionId": session.ID, "runtimeId": state.RuntimeID, "mode": state.Mode}}}); err != nil {
			c.mu.Unlock()
			return err
		}
		r.ready, r.fresh = true, false
		c.mu.Unlock()
		return nil
	}
	if r.state.ResumeAttempted || r.state.Fault != "" {
		c.mu.Unlock()
		return xerrors.New("Claude resume outcome is unknown; inspect the native terminal")
	}
	state := claudeClone(r.state)
	select {
	case <-r.registered:
		r.registered = make(chan struct{})
	default:
	}
	reconnecting := r.registered
	c.mu.Unlock()
	if err := c.verifyResume(ctx, session, state.LaunchID); err != nil {
		c.mu.Lock()
		reconnected := !r.lastSeen.IsZero() && c.clock.Since(r.lastSeen) <= claudeStaleAfter && !r.state.Ended
		c.mu.Unlock()
		if reconnected {
			return c.Resume(ctx, session)
		}
		if !errors.Is(err, errClaudeLiveWorker) {
			return err
		}
		// A restarted agent publishes a new descriptor before the live mod's next
		// poll. Wait for that exact runtime rather than cloning a live worker.
		if err := c.wait(ctx, reconnecting, claudeRegistrationTimeout); err != nil {
			return xerrors.Errorf("matching Claude runtime did not reconnect: %w", err)
		}
		c.mu.Lock()
		live := !r.lastSeen.IsZero() && c.clock.Since(r.lastSeen) <= claudeStaleAfter && !r.state.Ended
		c.mu.Unlock()
		if !live {
			return xerrors.New("matching Claude runtime is not live")
		}
		return c.Resume(ctx, session)
	}
	c.mu.Lock()
	// A matching live mod can arrive while native files are being checked.
	if !r.lastSeen.IsZero() && c.clock.Since(r.lastSeen) <= claudeStaleAfter && !r.state.Ended {
		c.mu.Unlock()
		return c.Resume(ctx, session)
	}
	if c.closed || r.state.ResumeAttempted || r.state.RuntimeID != state.RuntimeID || r.state.Seq != state.Seq {
		c.mu.Unlock()
		return xerrors.New("Claude runtime changed during resume validation")
	}
	if len(state.Retired) >= 64 {
		c.mu.Unlock()
		return xerrors.New("Claude runtime replacement budget exhausted")
	}
	if state.RuntimeID != "" {
		state.Retired = append(state.Retired, state.RuntimeID)
	}
	state.RuntimeID, state.Seq, state.Mode = "", 0, "human"
	state.ActiveTurnID, state.OwnedRequestID, state.OwnedTurnID = "", "", ""
	state.Ended, state.ResumeAttempted = false, true
	for id, receipt := range state.Receipts {
		if receipt.Result == "pending" {
			receipt.Result = "unknown"
			state.Receipts[id] = receipt
		}
	}
	if err := c.publishLocked(r, state, []Event{{Type: "session.resuming", Data: map[string]any{"mode": "human"}}}); err != nil {
		c.mu.Unlock()
		return err
	}
	// Commands belong to one runtime. No old prompt may be delivered to the
	// resumed process, even when its native dispatch outcome was unknown.
	for id := range r.commands {
		c.finishLocked(r, id, xerrors.New(claudeUnknown))
	}
	r.ready = true
	r.registered = make(chan struct{})
	registered := r.registered
	c.mu.Unlock()
	// Additional settings or plugin flags clone a native job. Only these three
	// arguments are permitted when resuming an existing conversation.
	if _, err := c.run(ctx, session.Cwd, 20*time.Second, "--bg", "--resume", session.NativeID); err != nil {
		return xerrors.New("native Claude resume outcome is unknown; inspect claude agents before retrying")
	}
	if err := c.wait(ctx, registered, claudeRegistrationTimeout); err != nil {
		return xerrors.Errorf("Claude resumed registration is pending: %w", err)
	}
	return c.SetMode(ctx, session, "human")
}

func (c *Claude) loadLocked(session Session) (*claudeRuntime, error) {
	if session.Harness != "claude" || !claudeUUID(session.ID) || session.ID != session.NativeID || !nativeValidPath(session.Cwd) {
		return nil, xerrors.New("invalid Claude session identity")
	}
	if r := c.sessions[session.ID]; r != nil {
		if r.session.Cwd != session.Cwd {
			return nil, xerrors.New("Claude working directory changed")
		}
		return r, nil
	}
	var state claudeState
	if err := json.Unmarshal(session.State, &state); err != nil || !claudeUUID(state.LaunchID) {
		return nil, xerrors.New("missing Claude ownership receipt")
	}
	if state.Receipts == nil {
		state.Receipts = make(map[string]claudeReceipt)
	}
	r := &claudeRuntime{session: session, state: state, commands: make(map[string]*claudePending), registered: make(chan struct{})}
	c.sessions[session.ID] = r
	return r, nil
}

// ServeHTTP handles the manager's authenticated /v1/runtime route. It acknowledges
// a native sequence only after the events and receipt were atomically published.
func (c *Claude) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/v1/runtime" || request.Method != http.MethodPost {
		http.NotFound(w, request)
		return
	}
	var body claudeHeartbeat
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 2*1024*1024))
	if err := decoder.Decode(&body); err != nil {
		http.Error(w, "invalid runtime heartbeat", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		http.Error(w, "invalid runtime heartbeat", http.StatusBadRequest)
		return
	}
	if !claudeUUID(body.SessionID) || !claudeUUID(body.LaunchID) || len(body.RuntimeID) == 0 || len(body.RuntimeID) > 200 || !nativeValidPath(body.Cwd) || !claudeMode(body.Mode) || len(body.ActiveTurnID) > 200 || len(body.Events) > 128 {
		http.Error(w, "invalid runtime identity", http.StatusBadRequest)
		return
	}
	for _, event := range body.Events {
		if event.Seq == 0 || event.Seq > 9007199254740991 || event.Type == "" || len(event.Type) > 80 || event.Data == nil {
			http.Error(w, "invalid runtime event", http.StatusBadRequest)
			return
		}
		if event.Type == "turn.start" || event.Type == "turn.complete" {
			turnID, _ := event.Data["turnId"].(string)
			if turnID == "" || len(turnID) > 200 {
				http.Error(w, "invalid runtime turn", http.StatusBadRequest)
				return
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		http.Error(w, "observer closed", http.StatusServiceUnavailable)
		return
	}
	r := c.sessions[body.SessionID]
	if r == nil {
		if launch := c.launches[body.LaunchID]; launch != nil && launch.cwd == body.Cwd && launch.runtime == nil {
			state := claudeState{LaunchID: body.LaunchID, RuntimeID: body.RuntimeID, Mode: body.Mode, Receipts: make(map[string]claudeReceipt)}
			saved, _ := json.Marshal(state)
			r = &claudeRuntime{session: Session{ID: body.SessionID, NativeID: body.SessionID, Harness: "claude", Cwd: body.Cwd, Mode: body.Mode, StartedAt: c.clock.Now().UnixMilli(), State: saved}, state: state, fresh: true, commands: make(map[string]*claudePending), registered: make(chan struct{})}
			c.sessions[body.SessionID] = r
			launch.runtime = r
			close(launch.registered)
		} else if session, err := c.options.Host.Session(body.SessionID); err == nil {
			r, _ = c.loadLocked(session)
		}
	}
	if r == nil || r.session.Cwd != body.Cwd || r.state.LaunchID != body.LaunchID || slices.Contains(r.state.Retired, body.RuntimeID) || (r.state.RuntimeID != "" && r.state.RuntimeID != body.RuntimeID) {
		http.Error(w, "unrecognized native runtime", http.StatusConflict)
		return
	}
	if !r.ready {
		// Keep all unacknowledged events in the mod until manager registration.
		r.lastSeen = c.clock.Now()
		select {
		case <-r.registered:
		default:
			close(r.registered)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ack": r.state.Seq, "commands": []claudeCommand{}})
		return
	}
	state := claudeClone(r.state)
	seq := state.Seq
	for _, event := range body.Events {
		if event.Seq <= seq {
			continue
		}
		if event.Seq != seq+1 && event.Type != "overflow" {
			c.faultLocked(r, "native Claude event gap; inspect the native terminal")
			http.Error(w, "runtime event gap", http.StatusConflict)
			return
		}
		seq = event.Seq
	}
	events := make([]Event, 0, len(body.Events)+2)
	if state.RuntimeID == "" {
		state.RuntimeID, state.ResumeAttempted = body.RuntimeID, false
		events = append(events, Event{Type: "runtime.resumed", Data: map[string]any{"runtimeId": body.RuntimeID, "mode": "human"}})
	}
	state.Mode, state.ActiveTurnID = body.Mode, body.ActiveTurnID
	results := make(map[string]error)
	for _, event := range body.Events {
		if event.Seq <= state.Seq {
			continue
		}
		state.Seq = event.Seq
		for _, normalized := range claudeJournalEvents(event) {
			claudeIngest(&state, normalized, r.commands, results)
			normalized.Seq = 0
			events = append(events, normalized)
		}
	}
	if body.Ended && !state.Ended {
		state.Ended = true
		events = append(events, Event{Type: "session.end", Data: map[string]any{"reason": "native runtime ended"}})
	}
	if state.Ended || state.Fault != "" {
		state.Mode = "human"
	}
	if err := c.publishLocked(r, state, events); err != nil {
		http.Error(w, "persist runtime events", http.StatusInternalServerError)
		return
	}
	r.lastSeen = c.clock.Now()
	select {
	case <-r.registered:
	default:
		close(r.registered)
	}
	for id, err := range results {
		c.finishLocked(r, id, err)
	}
	if state.Ended || state.Fault != "" {
		for id := range r.commands {
			c.finishLocked(r, id, xerrors.New(claudeUnknown))
		}
	}
	commands := make([]claudeCommand, 0, len(r.commands))
	if !state.Ended && state.Fault == "" {
		for _, command := range r.commands {
			commands = append(commands, command.command)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ack": state.Seq, "commands": commands})
}

// claudeJournalEvents preserves the mod's unescaped JSON budget while adapting
// streaming text to the journal's HTML-escaped JSON budget. A non-streaming event
// that cannot fit is a durable fault, not an endlessly retried HTTP rejection.
func claudeJournalEvents(event Event) []Event {
	overflow := []Event{{Type: "overflow", Data: map[string]any{"reason": "native Claude event exceeds journal budget; inspect the native terminal"}}}
	var wire bytes.Buffer
	encoder := json.NewEncoder(&wire)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(event); err != nil || wire.Len()-1 > claudeEventLimit {
		return overflow
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return overflow
	}
	if len(encoded) <= claudeEventLimit {
		return []Event{event}
	}
	text, ok := event.Data["text"].(string)
	if !ok || (event.Type != "text" && event.Type != "thought") {
		return overflow
	}
	// Check fixed metadata before splitting, and never split a UTF-8 code point.
	base := event
	base.Data = maps.Clone(event.Data)
	base.Data["text"] = ""
	encoded, err = json.Marshal(base)
	if err != nil || len(encoded) >= claudeEventLimit {
		return overflow
	}
	// JSON uses at most six bytes for each byte of input (ASCII control/HTML).
	chunkBytes := (claudeEventLimit - len(encoded)) / 6
	if chunkBytes < utf8.UTFMax {
		return overflow
	}
	var events []Event
	for len(text) > 0 {
		if len(events) >= 16 {
			return overflow
		}
		n := min(chunkBytes, len(text))
		for n < len(text) && !utf8.RuneStart(text[n]) {
			n--
		}
		chunk := base
		chunk.Data = maps.Clone(base.Data)
		chunk.Data["text"] = text[:n]
		events = append(events, chunk)
		text = text[n:]
	}
	return events
}

func claudeIngest(state *claudeState, event Event, commands map[string]*claudePending, results map[string]error) {
	requestID, _ := event.Data["requestId"].(string)
	turnID, _ := event.Data["turnId"].(string)
	receipt, known := state.Receipts[requestID]
	if known && receipt.Kind == "prompt" {
		switch event.Type {
		case "turn.start":
			if state.OwnedRequestID == requestID && turnID != "" {
				state.OwnedTurnID = turnID
			}
			fallthrough
		case "prompt.accepted":
			if receipt.Result == "pending" {
				receipt.Result = "accepted"
				results[requestID] = nil
			}
		case "prompt.rejected":
			if receipt.Result == "pending" {
				receipt.Result = "rejected"
				results[requestID] = xerrors.New("native Claude rejected the prompt; inspect the native terminal")
			}
			if state.OwnedRequestID == requestID && state.OwnedTurnID == "" {
				state.OwnedRequestID = ""
			}
		case "turn.complete":
			if turnID != "" && state.OwnedRequestID == requestID && state.OwnedTurnID == turnID {
				state.OwnedRequestID, state.OwnedTurnID = "", ""
				if receipt.Result == "pending" {
					results[requestID] = nil
				}
				receipt.Result = "completed"
			}
		}
		state.Receipts[requestID] = receipt
	}
	switch event.Type {
	case "command.done", "command.error":
		id, _ := event.Data["commandId"].(string)
		if pending := commands[id]; pending != nil {
			var err error
			if event.Type == "command.error" || pending.command.Type == "prompt" || (pending.command.Type == "mode" && pending.command.Mode != state.Mode) {
				err = xerrors.New("native Claude command was not confirmed")
			}
			results[id] = err
			receipt := state.Receipts[id]
			receipt.Result = "accepted"
			if err != nil {
				receipt.Result = "rejected"
			}
			state.Receipts[id] = receipt
		}
	case "session.end":
		state.Ended = true
	case "overflow":
		state.Fault = "native Claude event stream overflow; inspect the native terminal"
	}
}

// Prompt waits for native acceptance, not completion. Retries never redispatch a
// request whose receipt says it may already have reached the native runtime.
func (c *Claude) Prompt(ctx context.Context, session Session, prompt Prompt) error {
	if prompt.ID == "" || len(prompt.ID) > 200 || prompt.Text == "" || len(prompt.Text) > 1024*1024 {
		return xerrors.New("invalid Claude prompt")
	}
	return c.command(ctx, session, claudeCommand{ID: prompt.ID, Type: "prompt", Text: prompt.Text})
}

// Cancel targets only the mod's outstanding request and its exact owned root turn.
func (c *Claude) Cancel(ctx context.Context, session Session, requestID string) error {
	if requestID == "" || len(requestID) > 200 {
		return xerrors.New("invalid Claude request identity")
	}
	return c.command(ctx, session, claudeCommand{ID: "cancel:" + requestID, Type: "cancel", RequestID: requestID})
}

// SetMode waits for a native acknowledgement with the requested mode.
func (c *Claude) SetMode(ctx context.Context, session Session, mode string) error {
	if !claudeMode(mode) {
		return xerrors.New("invalid Claude mode")
	}
	return c.command(ctx, session, claudeCommand{ID: uuid.NewString(), Type: "mode", Mode: mode})
}

func (c *Claude) command(ctx context.Context, session Session, command claudeCommand) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	r, err := c.loadLocked(session)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	if c.closed || !r.ready || r.state.Ended || r.state.Fault != "" || r.lastSeen.IsZero() || c.clock.Since(r.lastSeen) > claudeStaleAfter {
		c.mu.Unlock()
		return xerrors.New("native Claude runtime is unavailable or stale")
	}
	encoded, _ := json.Marshal(command)
	digest := fmt.Sprintf("%x", sha256.Sum256(encoded))
	pending := r.commands[command.ID]
	if receipt, exists := r.state.Receipts[command.ID]; exists {
		if receipt.Digest != digest {
			c.mu.Unlock()
			return xerrors.New("Claude command identity was reused with different input")
		}
		if receipt.Result == "accepted" || receipt.Result == "completed" {
			c.mu.Unlock()
			return nil
		}
		if pending == nil || receipt.Result != "pending" {
			c.mu.Unlock()
			return xerrors.New(claudeUnknown)
		}
	} else {
		if len(r.commands) >= 32 || len(r.state.Receipts) >= 1000 {
			c.mu.Unlock()
			return xerrors.New("Claude command receipt budget exhausted")
		}
		if command.Type == "prompt" && (r.state.Mode != "automation" || r.state.ActiveTurnID != "" || r.state.OwnedRequestID != "") {
			c.mu.Unlock()
			return xerrors.New("native Claude is not idle under automation control")
		}
		if command.Type == "cancel" && r.state.OwnedRequestID != command.RequestID {
			c.mu.Unlock()
			return xerrors.New("Claude cancellation does not own the outstanding request")
		}
		state := claudeClone(r.state)
		state.Receipts[command.ID] = claudeReceipt{Kind: command.Type, Digest: digest, Result: "pending"}
		if command.Type == "prompt" {
			state.OwnedRequestID = command.ID
		}
		if err := c.publishLocked(r, state, nil); err != nil {
			c.mu.Unlock()
			return err
		}
		pending = &claudePending{command: command, done: make(chan struct{})}
		r.commands[command.ID] = pending
	}
	c.mu.Unlock()
	err = c.wait(ctx, pending.done, claudeCommandTimeout)
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-pending.done:
		return pending.result
	default:
	}
	if err != nil {
		c.faultLocked(r, claudeUnknown)
		return xerrors.Errorf("%s: %w", claudeUnknown, err)
	}
	return pending.result
}

func (*Claude) finishLocked(r *claudeRuntime, id string, err error) {
	if pending := r.commands[id]; pending != nil {
		pending.result = err
		delete(r.commands, id)
		close(pending.done)
	}
}

func (c *Claude) publishLocked(r *claudeRuntime, state claudeState, events []Event) error {
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(encoded) > 256*1024 {
		return xerrors.New("Claude receipt state budget exhausted")
	}
	if err := c.options.Host.Publish(r.session.ID, events, encoded); err != nil {
		return xerrors.Errorf("persist Claude receipt: %w", err)
	}
	r.state = state
	return nil
}

func (c *Claude) faultLocked(r *claudeRuntime, reason string) {
	state := claudeClone(r.state)
	state.Fault, state.Mode = reason, "human"
	for id, receipt := range state.Receipts {
		if receipt.Result == "pending" {
			receipt.Result = "unknown"
			state.Receipts[id] = receipt
		}
	}
	if err := c.publishLocked(r, state, []Event{{Type: "overflow", Data: map[string]any{"reason": reason}}}); err != nil {
		// Persistence failure must still stop delivery in this process.
		r.state.Fault = reason
	}
	for id := range r.commands {
		c.finishLocked(r, id, xerrors.New(reason))
	}
}

// Attach returns a native terminal command without changing session control.
func (c *Claude) Attach(_ context.Context, session Session) (Command, error) {
	if session.Harness != "claude" || !claudeUUID(session.NativeID) || session.ID != session.NativeID || !nativeValidPath(session.Cwd) {
		return Command{}, xerrors.New("invalid Claude session identity")
	}
	return Command{Path: "claude", Args: []string{"attach", session.NativeID[:8]}, Dir: session.Cwd, Env: slices.Clone(c.options.Environment)}, nil
}

// Close releases only observation. The descriptor remains for a future observer.
func (c *Claude) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.done)
	}
	return nil
}

func (c *Claude) wait(ctx context.Context, done <-chan struct{}, duration time.Duration) error {
	timer := c.clock.NewTimer(duration, "claude", "wait")
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return xerrors.New("Claude observer is closed")
	case <-timer.C:
		return context.DeadlineExceeded
	}
}

// run bounds both output and execution. Native diagnostics never become errors
// because they may contain provider credentials or conversation text.
func (c *Claude) run(ctx context.Context, cwd string, timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var command *exec.Cmd
	if c.options.Command != nil {
		command = c.options.Command(ctx, "claude", args...)
	} else {
		path, err := claudeExecutable(c.options.Environment)
		if err != nil {
			return nil, err
		}
		command = c.execer.CommandContext(ctx, path, args...)
	}
	command.Env = slices.Clone(c.options.Environment)
	command.Dir = cwd
	command.WaitDelay = time.Second
	output := &claudeOutput{limit: 1024 * 1024}
	command.Stdout, command.Stderr = output, io.Discard
	if err := command.Run(); err != nil {
		return nil, xerrors.New("native Claude command failed")
	}
	return output.data, nil
}

type claudeOutput struct {
	data  []byte
	limit int
}

func (b *claudeOutput) Write(data []byte) (int, error) {
	if len(data) > b.limit-len(b.data) {
		return 0, xerrors.New("native output exceeds limit")
	}
	b.data = append(b.data, data...)
	return len(data), nil
}

func claudeExecutable(env []string) (string, error) {
	for _, dir := range filepath.SplitList(claudeEnv(env, "PATH")) {
		if !filepath.IsAbs(dir) {
			continue
		}
		path := filepath.Join(dir, "claude")
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return path, nil
		}
	}
	return "", xerrors.New("Claude executable not found in workspace PATH")
}
func claudeEnv(env []string, name string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if value, ok := strings.CutPrefix(env[i], name+"="); ok {
			return value
		}
	}
	return ""
}
func claudeUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}
func claudeMode(mode string) bool { return mode == "human" || mode == "automation" }
func claudeClone(state claudeState) claudeState {
	state.Retired = slices.Clone(state.Retired)
	receipts := make(map[string]claudeReceipt, len(state.Receipts))
	for id, receipt := range state.Receipts {
		receipts[id] = receipt
	}
	state.Receipts = receipts
	return state
}
