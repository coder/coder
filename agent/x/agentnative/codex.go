package agentnative

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/agent/agentexec"
)

const codexVersion = "0.164.0"
const codexBufferLimit = 1 << 20

// Codex observes a dedicated native app-server. Closing the observer does not
// stop the server or its work. Only Shutdown may stop a server started here.
type Codex struct {
	options        BackendOptions
	execer         agentexec.Execer
	clock          quartz.Clock
	lifecycle      sync.Mutex
	mu             sync.Mutex
	conn           codexRPC
	generation     uint64
	closed         bool
	ctx            context.Context
	cancel         context.CancelFunc
	tasks          sync.WaitGroup
	sessions       map[string]*codexSession
	creating       bool
	early          []codexMessage
	earlyBytes     int
	earlyOverflow  bool
	owned          *exec.Cmd
	ownedDone      <-chan struct{}
	ownedRecord    *codexServerRecord
	ownedFinalized bool
	seenServer     bool
	dial           func(context.Context, func(codexMessage), func()) (codexRPC, error)
}

type codexReceipt struct {
	ClientID        string `json:"clientId"`
	TextHash        string `json:"textHash"`
	Status          string `json:"status"`
	TurnID          string `json:"turnId,omitempty"`
	CancelSent      bool   `json:"cancelSent,omitempty"`
	CancelRequested bool   `json:"cancelRequested,omitempty"`
}

type codexState struct {
	Version  string                   `json:"version"`
	Requests map[string]*codexReceipt `json:"requests"`
}

type codexSession struct {
	identity     Session
	state        codexState
	registered   bool
	ready        bool
	fault        bool
	mode         string
	status       string
	active       string
	revision     uint64
	workRevision uint64
	turns        map[string]*codexTurn
	approvals    map[string]codexApproval
	buffer       []codexMessage
	bufferBytes  int
	raceBytes    int
	itemCount    int
}

var _ Backend = (*Codex)(nil)

// NewCodex constructs an observer without running a CLI or starting a service.
func NewCodex(options BackendOptions) (*Codex, error) {
	if options.Host == nil || !filepath.IsAbs(options.StateDir) {
		return nil, xerrors.New("codex requires a host and an absolute state directory")
	}
	if options.Clock == nil {
		options.Clock = quartz.NewReal()
	}
	options.Environment = append([]string(nil), options.Environment...)
	var execer agentexec.Execer
	if options.Command == nil {
		var err error
		execer, err = agentexec.NewExecer()
		if err != nil {
			return nil, xerrors.Errorf("create Codex process executor: %w", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Codex{options: options, execer: execer, clock: options.Clock, ctx: ctx, cancel: cancel, sessions: make(map[string]*codexSession)}
	c.dial = c.dialRPC
	return c, nil
}

// command resolves the executable in the workspace user's PATH, not the
// bootstrap agent's environment. Construction never probes or runs the CLI.
func (c *Codex) command(ctx context.Context, args ...string) (*exec.Cmd, error) {
	if c.options.Command != nil {
		return c.options.Command(ctx, "codex", args...), nil
	}
	path := ""
	for _, entry := range c.options.Environment {
		if value, ok := strings.CutPrefix(entry, "PATH="); ok {
			path = value
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if !filepath.IsAbs(dir) {
			continue
		}
		candidate := filepath.Join(dir, "codex")
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return c.execer.CommandContext(ctx, candidate, args...), nil
		}
	}
	return nil, xerrors.New("Codex executable not found in workspace PATH")
}

func codexValidID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && len(id) == 36 && strings.EqualFold(parsed.String(), id)
}

func codexValidateSession(s Session) error {
	if s.Harness != "codex" || !codexValidID(s.NativeID) || s.ID != "codex:"+s.NativeID || !filepath.IsAbs(s.Cwd) || len(s.Cwd) > 4096 {
		return xerrors.New("invalid codex session identity")
	}
	return nil
}

func newCodexSession(s Session) *codexSession {
	return &codexSession{identity: s, mode: "human", state: codexState{Version: codexVersion, Requests: make(map[string]*codexReceipt)}, turns: make(map[string]*codexTurn), approvals: make(map[string]codexApproval)}
}

func (c *Codex) publish(s *codexSession, events ...Event) error {
	state, err := json.Marshal(s.state)
	if err == nil {
		err = c.options.Host.Publish(s.identity.ID, events, state)
	}
	if err != nil {
		s.fault = true
		s.ready = false
		s.mode = "human"
	}
	return err
}

func (c *Codex) fault(s *codexSession, reason string) {
	if s.fault {
		return
	}
	s.fault = true
	s.ready = false
	s.mode = "human"
	for _, t := range s.turns {
		if t.timer != nil {
			t.timer.Stop()
			t.timer = nil
		}
	}
	for _, r := range s.state.Requests {
		if r.Status == "queued" || r.Status == "running" {
			r.Status = "unknown"
		}
	}
	if s.registered {
		_ = c.publish(s, Event{Type: "mode", Data: map[string]any{"mode": "human"}}, Event{Type: "overflow", Data: map[string]any{"reason": reason}})
	}
}

func (c *Codex) disconnected(generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != generation {
		return
	}
	c.generation++
	c.conn = nil
	if c.closed {
		return
	}
	for _, s := range c.sessions {
		c.fault(s, "Codex disconnected; native work state is unknown")
	}
}

// Create returns the native identity first. Live events are held until the host
// registers that identity and calls Resume; nothing is published prematurely.
func (c *Codex) Create(ctx context.Context, cwd string) (Session, error) {
	if !filepath.IsAbs(cwd) || len(cwd) > 4096 {
		return Session{}, xerrors.New("codex cwd must be absolute")
	}
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	connection, err := c.ensure(ctx, true)
	if err != nil {
		return Session{}, &creationSetupError{err: err}
	}
	c.mu.Lock()
	c.creating = true
	c.early = nil
	c.earlyBytes = 0
	c.earlyOverflow = false
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.creating = false; c.early = nil; c.earlyBytes = 0; c.mu.Unlock() }()
	var result struct {
		Thread codexThread `json:"thread"`
	}
	if err := connection.call(ctx, "thread/start", map[string]any{"cwd": cwd}, &result); err != nil {
		return Session{}, err
	}
	if err := result.Thread.validate(Session{Cwd: cwd}); err != nil {
		return Session{}, err
	}
	s := Session{ID: "codex:" + result.Thread.ID, NativeID: result.Thread.ID, Harness: "codex", Cwd: cwd, Mode: "human", StartedAt: c.clock.Now().UnixMilli()}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.conn != connection || c.earlyOverflow {
		return Session{}, xerrors.New("codex creation observation was lost; native thread may exist")
	}
	if _, exists := c.sessions[s.NativeID]; exists {
		return Session{}, xerrors.New("codex returned an existing thread")
	}
	runtime := newCodexSession(s)
	runtime.status = result.Thread.Status.Type
	for _, event := range c.early {
		if event.threadID() == s.NativeID {
			runtime.buffer = append(runtime.buffer, event)
			runtime.bufferBytes += event.size()
		}
	}
	c.sessions[s.NativeID] = runtime
	s.State, err = json.Marshal(runtime.state)
	return s, err
}

// Resume joins native history without importing or replaying it. Cold threads
// are inspected for queued work and unfinished goals before loading them.
func (c *Codex) Resume(ctx context.Context, saved Session) (err error) {
	if err := codexValidateSession(saved); err != nil {
		return err
	}
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	snapshot, err := c.options.Host.Session(saved.ID)
	if err != nil {
		return err
	}
	if snapshot.NativeID != saved.NativeID || snapshot.Cwd != saved.Cwd || snapshot.Harness != "codex" {
		return xerrors.New("codex saved identity changed")
	}
	connection, err := c.ensure(ctx, true)
	if err != nil {
		return err
	}
	c.mu.Lock()
	s := c.sessions[saved.NativeID]
	fresh := s != nil && !s.registered
	if s != nil && s.registered && s.ready {
		c.mu.Unlock()
		return nil
	}
	if s == nil || s.registered {
		s = newCodexSession(snapshot)
		if len(snapshot.State) == 0 || json.Unmarshal(snapshot.State, &s.state) != nil || s.state.Version != codexVersion || s.state.Requests == nil || len(s.state.Requests) > 4096 {
			c.mu.Unlock()
			return xerrors.New("invalid or missing codex receipts")
		}
		clients := make(map[string]bool)
		for requestID, r := range s.state.Requests {
			if !codexValidID(requestID) || r == nil || !codexValidID(r.ClientID) || clients[r.ClientID] || len(r.TextHash) != 64 {
				c.mu.Unlock()
				return xerrors.New("invalid codex prompt receipt")
			}
			clients[r.ClientID] = true
			if r.Status != "completed" && r.Status != "rejected" {
				r.Status = "unknown"
			}
		}
		c.sessions[saved.NativeID] = s
	}
	s.registered = true
	defer func() {
		if err != nil {
			c.mu.Lock()
			c.fault(s, "Codex resume was not confirmed; inspect the native terminal")
			c.mu.Unlock()
		}
	}()
	if s.fault {
		c.mu.Unlock()
		return xerrors.New("codex startup event buffer exhausted")
	}
	if err = c.publish(s, Event{Type: "mode", Data: map[string]any{"mode": "human"}}); err != nil {
		c.mu.Unlock()
		return err
	}
	for _, event := range s.buffer {
		if e := c.ingest(s, event); e != nil {
			c.fault(s, "Invalid Codex startup stream")
			break
		}
	}
	s.buffer = nil
	s.bufferBytes = 0
	beforeReadRevision := s.revision
	c.mu.Unlock()
	var before struct {
		Thread codexThread `json:"thread"`
	}
	if err = connection.call(ctx, "thread/read", map[string]any{"threadId": saved.NativeID, "includeTurns": false}, &before); err != nil {
		return err
	}
	if err = before.Thread.validate(saved); err != nil {
		return err
	}
	if before.Thread.Status.Type == "systemError" {
		return xerrors.New("native codex thread is faulted")
	}
	if fresh {
		// thread/start already loaded this identity on this connection. Native
		// resume rejects a brand-new thread without a persisted rollout; only
		// complete the host registration handshake, without replay or loading.
		c.mu.Lock()
		defer c.mu.Unlock()
		if s.fault || c.closed || c.conn != connection || before.Thread.Status.Type == "notLoaded" {
			return xerrors.New("new Codex thread is no longer observed")
		}
		if s.revision == beforeReadRevision {
			s.status = before.Thread.Status.Type
		}
		s.ready = true
		return c.publish(s)
	}
	c.mu.Lock()
	workRevision := s.workRevision
	c.mu.Unlock()
	cold := before.Thread.Status.Type == "notLoaded"
	if cold {
		// 0.164.0 exposes no observer-only resume, queue revision precondition,
		// or exclusive client lease. Its queue extension dispatches on idle after
		// cold resume. These checks and the notification fence reject observed
		// concurrent edits, but cannot exclude a native client's queue/add or
		// goal update between the final check and resume. Cold restoration needs
		// the operational assumption that native clients do not edit this thread
		// during restoration. Never cancel work started by that residual race.
		var queue struct {
			Data json.RawMessage `json:"data"`
			Next json.RawMessage `json:"nextCursor"`
		}
		if err = connection.call(ctx, "thread/queue/list", map[string]any{"threadId": saved.NativeID, "limit": 1}, &queue); err != nil {
			return err
		}
		var data []json.RawMessage
		if len(queue.Data) == 0 || queue.Data[0] != '[' || json.Unmarshal(queue.Data, &data) != nil || len(data) != 0 || string(queue.Next) != "null" {
			return xerrors.New("saved codex thread has queued or unknown work")
		}
		var goal struct {
			Goal json.RawMessage `json:"goal"`
		}
		err = connection.call(ctx, "thread/goal/get", map[string]any{"threadId": saved.NativeID}, &goal)
		var rejected *codexRPCError
		if errors.As(err, &rejected) && rejected.goalsDisabled {
			goal.Goal = json.RawMessage("null")
			err = nil
		}
		if err != nil {
			return err
		}
		if string(goal.Goal) != "null" {
			var state struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(goal.Goal, &state) != nil || state.Status != "complete" {
				return xerrors.New("saved codex thread has an unfinished or unknown goal")
			}
		}
	}
	c.mu.Lock()
	revision := s.revision
	unsafeResume := s.fault || (cold && s.workRevision != workRevision)
	c.mu.Unlock()
	if unsafeResume {
		return xerrors.New("native Codex state changed during resume preflight")
	}
	var result struct {
		Thread codexThread `json:"thread"`
		Page   struct {
			Data []codexNativeTurn `json:"data"`
		} `json:"initialTurnsPage"`
	}
	if err = connection.call(ctx, "thread/resume", map[string]any{"threadId": saved.NativeID, "excludeTurns": true, "initialTurnsPage": map[string]any{"limit": 1, "itemsView": "summary", "sortDirection": "desc"}}, &result); err != nil {
		return err
	}
	if err = result.Thread.validate(saved); err != nil {
		return err
	}
	if result.Page.Data == nil || len(result.Page.Data) > 1 {
		return xerrors.New("invalid codex resume page")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if s.fault || c.closed || c.conn != connection {
		return xerrors.New("codex disconnected while resuming")
	}
	if cold && s.workRevision != workRevision {
		return xerrors.New("native Codex work changed during cold resume; inspect the native terminal")
	}
	current := revision == s.revision
	if current {
		s.status = result.Thread.Status.Type
	}
	for _, native := range result.Page.Data {
		if !codexSafeID(native.ID) || native.Items == nil || !codexTurnStatus(native.Status) {
			return xerrors.New("invalid codex resume turn")
		}
		t := s.turns[native.ID]
		if native.Status == "inProgress" && (current || (s.status == "active" && (s.active == "" || s.active == native.ID))) {
			t, err = c.turn(s, native.ID)
			if err != nil {
				return err
			}
			if !t.nativeCompleted && !t.completed {
				s.active = t.id
			}
		}
		if t != nil && !t.identified && !t.completed {
			for _, item := range native.Items {
				if item.Type == "userMessage" {
					if err = c.item(s, t, item, true); err != nil {
						return err
					}
				}
			}
		}
	}
	for _, t := range s.turns {
		if !t.identified && !t.completed {
			return xerrors.New("codex live turn identity is missing from resume page")
		}
	}
	if s.status == "systemError" {
		return xerrors.New("native codex thread is faulted")
	}
	s.ready = true
	return c.publish(s)
}

func (c *Codex) live(identity Session) (*codexSession, error) {
	if err := codexValidateSession(identity); err != nil {
		return nil, err
	}
	s := c.sessions[identity.NativeID]
	if c.closed || c.conn == nil || s == nil || !s.ready || s.fault || s.identity.Cwd != identity.Cwd {
		return nil, xerrors.New("codex session is not live")
	}
	return s, nil
}

// Prompt durably records correlation before dispatch. A response alone never
// grants turn ownership: only the native userMessage.clientId can do that.
func (c *Codex) Prompt(ctx context.Context, identity Session, prompt Prompt) error {
	if !codexValidID(prompt.ID) || strings.TrimSpace(prompt.Text) == "" || len(prompt.Text) > 400000 {
		return xerrors.New("invalid codex prompt")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	s, err := c.live(identity)
	connection := c.conn
	c.mu.Unlock()
	if err != nil {
		return err
	}
	var before struct {
		Thread codexThread `json:"thread"`
	}
	if err := connection.call(ctx, "thread/read", map[string]any{"threadId": identity.NativeID, "includeTurns": false}, &before); err != nil {
		return err
	}
	if err := before.Thread.validate(identity); err != nil {
		c.mu.Lock()
		c.fault(s, "Native Codex working directory no longer matches this session")
		c.mu.Unlock()
		return err
	}
	c.mu.Lock()
	s, err = c.live(identity)
	if err != nil || c.conn != connection {
		c.mu.Unlock()
		if err != nil {
			return err
		}
		return xerrors.New("Codex connection changed during prompt preflight")
	}
	hash := sha256.Sum256([]byte(prompt.Text))
	textHash := hex.EncodeToString(hash[:])
	if old := s.state.Requests[prompt.ID]; old != nil {
		c.mu.Unlock()
		if old.TextHash != textHash {
			return xerrors.New("codex request ID already used for different text")
		}
		return xerrors.New("codex request already dispatched; inspect its durable receipt")
	}
	if s.mode != "automation" || s.active != "" || s.status != "idle" || before.Thread.Status.Type != "idle" || len(s.approvals) != 0 {
		c.mu.Unlock()
		return xerrors.New("codex is busy or under human control")
	}
	for _, r := range s.state.Requests {
		if r.Status == "queued" || r.Status == "running" || r.Status == "unknown" {
			c.mu.Unlock()
			return xerrors.New("codex has outstanding or unknown work")
		}
	}
	if len(s.state.Requests) >= 4096 {
		c.mu.Unlock()
		return xerrors.New("codex prompt receipt limit reached")
	}
	r := &codexReceipt{ClientID: uuid.NewString(), TextHash: textHash, Status: "queued"}
	s.state.Requests[prompt.ID] = r
	if err := c.publish(s, Event{Type: "prompt.accepted", Data: map[string]any{"requestId": prompt.ID, "clientUserMessageId": r.ClientID, "textHash": textHash}}); err != nil {
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()
	var result struct {
		Turn codexNativeTurn `json:"turn"`
	}
	// Pin this turn's native cwd as well as checking the saved binding. A
	// settings change after the read must not silently relocate our tools.
	err = connection.call(ctx, "turn/start", map[string]any{"threadId": identity.NativeID, "cwd": identity.Cwd, "clientUserMessageId": r.ClientID, "input": []any{map[string]any{"type": "text", "text": prompt.Text, "text_elements": []any{}}}}, &result)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		var rejection *codexRPCError
		if errors.As(err, &rejection) && rejection.rejected && r.TurnID == "" && !s.fault {
			r.Status = "rejected"
			if e := c.publish(s, Event{Type: "prompt.rejected", Data: map[string]any{"requestId": prompt.ID, "reason": "Native Codex rejected the prompt; inspect the native terminal"}}); e != nil {
				return e
			}
		} else if r.Status != "completed" {
			c.fault(s, "Codex prompt outcome is unknown; inspect the native terminal before retrying")
		}
		return err
	}
	if s.fault || c.closed {
		return xerrors.New("codex prompt observation lost")
	}
	t, err := c.turn(s, result.Turn.ID)
	if err != nil || (r.TurnID != "" && r.TurnID != result.Turn.ID) {
		c.fault(s, "Codex prompt correlation changed")
		return xerrors.New("invalid codex prompt correlation")
	}
	r.TurnID = t.id
	// A terminal response arriving after completion must not revive liveness.
	if !t.completed {
		if !t.nativeCompleted && s.active == "" {
			s.active = t.id
		}
		for _, item := range result.Turn.Items {
			if item.Type == "userMessage" {
				if err = c.item(s, t, item, false); err != nil {
					break
				}
			}
		}
	}
	if err != nil || (t.identified && t.requestID != prompt.ID) {
		c.fault(s, "Codex native turn is not owned by this prompt")
		return xerrors.New("codex turn ownership was not confirmed")
	}
	if !t.identified {
		t.timer = c.clock.AfterFunc(10*time.Second, func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			if !t.identified && !c.closed {
				c.fault(s, "Codex prompt ownership was not confirmed")
			}
		}, "codex-ownership")
	}
	if err := c.publish(s); err != nil {
		return err
	}
	return c.maybeCancel(s, prompt.ID)
}

// Cancel records an explicit request even before turn/start has identified its
// root. It never broadens cancellation to another turn or uses a response ID as
// ownership proof. Native completion, not an interrupt response, is the outcome.
func (c *Codex) Cancel(ctx context.Context, identity Session, requestID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s, err := c.live(identity)
	if err != nil {
		return err
	}
	r := s.state.Requests[requestID]
	if r == nil {
		return xerrors.New("unknown codex prompt")
	}
	if r.Status == "completed" || r.Status == "rejected" || r.CancelSent {
		return nil
	}
	t := s.turns[r.TurnID]
	if r.Status == "unknown" || (t != nil && t.human) {
		return xerrors.Errorf("codex turn is not cancellable by this request: %w", ErrUnsupported)
	}
	r.CancelRequested = true
	if err := c.publish(s); err != nil {
		return err
	}
	return c.maybeCancel(s, requestID)
}

// maybeCancel runs with mu held, after identity and buffered completions have
// been applied. The durable sent flag prevents duplicate interrupts even when
// identification, the start response, and a repeated cancel race each other.
func (c *Codex) maybeCancel(s *codexSession, requestID string) error {
	r := s.state.Requests[requestID]
	if r == nil || !r.CancelRequested || r.CancelSent || r.Status != "running" || s.fault || c.closed || c.conn == nil {
		return nil
	}
	t := s.turns[r.TurnID]
	if t == nil || !t.identified || t.requestID != requestID || t.human || t.nativeCompleted || t.completed || s.active != t.id {
		return nil
	}
	r.CancelSent = true
	if err := c.publish(s); err != nil {
		return err
	}
	connection, turnID := c.conn, t.id
	c.tasks.Add(1)
	go func() {
		defer c.tasks.Done()
		c.mu.Lock()
		canSend := !c.closed && !s.fault && c.conn == connection && !t.human && !t.nativeCompleted && !t.completed && s.active == turnID
		c.mu.Unlock()
		if !canSend {
			return
		}
		var response map[string]any
		err := connection.call(c.ctx, "turn/interrupt", map[string]any{"threadId": s.identity.NativeID, "turnId": turnID}, &response)
		if err != nil {
			c.mu.Lock()
			defer c.mu.Unlock()
			var rejection *codexRPCError
			if !errors.As(err, &rejection) || !rejection.rejected {
				c.fault(s, "Codex cancellation outcome is unknown")
			} else {
				// A rejected interrupt leaves work running and requires human
				// attention; it is not a completed or rejected prompt.
				_ = c.human(s)
			}
		}
	}()
	return nil
}

// SetMode gates future prompts, without interrupting work or answering approvals.
func (c *Codex) SetMode(ctx context.Context, identity Session, mode string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if mode != "human" && mode != "automation" {
		return xerrors.New("invalid codex mode")
	}
	if mode == "human" {
		// Human attachment is a recovery path, not an automation admission.
		// Verify the existing native target without loading it or clearing fault.
		if _, err := c.Attach(ctx, identity); err != nil {
			return err
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if s := c.sessions[identity.NativeID]; s != nil {
			s.mode = "human"
			return c.publish(s, Event{Type: "mode", Data: map[string]any{"mode": "human"}})
		}
		return c.options.Host.Publish(identity.ID, []Event{{Type: "mode", Data: map[string]any{"mode": "human"}}}, nil)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s, err := c.live(identity)
	if err != nil {
		return err
	}
	s.mode = mode
	return c.publish(s, Event{Type: "mode", Data: map[string]any{"mode": mode}})
}

// Attach verifies an existing service and identity, but never starts or resumes it.
func (c *Codex) Attach(ctx context.Context, identity Session) (Command, error) {
	if err := codexValidateSession(identity); err != nil {
		return Command{}, err
	}
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	connection, err := c.ensure(ctx, false)
	if err != nil {
		return Command{}, err
	}
	var result struct {
		Thread codexThread `json:"thread"`
	}
	if err := connection.call(ctx, "thread/read", map[string]any{"threadId": identity.NativeID, "includeTurns": false}, &result); err != nil {
		return Command{}, err
	}
	if err := result.Thread.validate(identity); err != nil {
		return Command{}, err
	}
	return Command{Path: "codex", Args: []string{"resume", "--remote", "unix://" + filepath.Join(c.options.StateDir, "codex.sock"), identity.NativeID}, Env: append([]string(nil), c.options.Environment...), Dir: identity.Cwd}, nil
}

// settled requires both native idleness and resolved local observations. An
// idle notification alone does not settle a dispatch awaiting attribution.
func (s *codexSession) settled() bool {
	if !s.ready || s.fault || s.status != "idle" || s.active != "" || len(s.approvals) != 0 {
		return false
	}
	for _, r := range s.state.Requests {
		if r.Status != "completed" && r.Status != "rejected" {
			return false
		}
	}
	for _, t := range s.turns {
		if !t.completed {
			return false
		}
	}
	return true
}

// Close releases observation only. Native work and the native service survive.
func (c *Codex) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	connection := c.conn
	c.conn = nil
	c.generation++
	for _, s := range c.sessions {
		if s.settled() {
			// Deliberately detaching an idle observer is not lost work. A fault
			// would persist through restoration and prevent future automation.
			s.ready = false
			_ = c.human(s)
		} else {
			c.fault(s, "Codex observer closed; native session continues")
		}
	}
	c.mu.Unlock()
	c.cancel()
	var err error
	if connection != nil {
		err = connection.close()
	}
	c.tasks.Wait()
	return err
}

// Shutdown stops only the native process session proved by our durable owner
// record, including after an observer restart. Unverifiable servers are refused.
func (c *Codex) Shutdown(ctx context.Context) error {
	_ = c.Close()
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	return c.stopOwned(ctx)
}
