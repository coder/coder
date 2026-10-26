package agentnative

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

// Options configures the workspace's native session manager. Environment must
// come from the workspace user, not the agent's bootstrap environment.
type Options struct {
	StateDir    string
	Environment []string
	Logger      slog.Logger
	Clock       quartz.Clock
	Command     func(context.Context, string, ...string) *exec.Cmd
	// Factories replaces the default backends when non-nil. Custom factories
	// own their prerequisite directories; New prepares only StateDir for them.
	Factories map[string]func(BackendOptions) (Backend, error)
}

// Manager journals native observations and guards at-most-once dispatch. It does
// not own native conversation contents or replay prompts during restoration.
type Manager struct {
	mu           sync.Mutex
	state        snapshot
	store        *sessionStore
	backends     map[string]Backend
	busy         map[string]bool
	creating     int
	attempt      string
	clock        quartz.Clock
	ctx          context.Context
	cancel       context.CancelFunc
	changed      chan struct{}
	wg           sync.WaitGroup
	closed       bool
	stopping     bool
	persistErr   error
	local        *localServer
	closeOnce    sync.Once
	closeErr     error
	shutdownOnce sync.Once
	shutdownDone chan struct{}
	shutdownErr  error
}

// New prepares prerequisite directories and opens the private journal and local
// listener. Native processes are initialized lazily by Create or Restore.
func New(options Options) (_ *Manager, err error) {
	if options.Clock == nil {
		options.Clock = quartz.NewReal()
	}
	if options.StateDir == "" {
		for _, value := range options.Environment {
			if home, ok := strings.CutPrefix(value, "HOME="); ok {
				options.StateDir = DefaultStateDir(home)
			}
		}
	}
	dir, err := openPrivateDirectory(options.StateDir, privateDirectoryCreate)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = dir.close()
		}
	}()
	if err := dir.acquire(); err != nil {
		return nil, err
	}
	if options.Factories == nil {
		if err := PrepareDirectories(options.StateDir, options.Environment); err != nil {
			return nil, err
		}
	}
	store, saved, err := openStore(dir)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{state: saved, store: store, backends: map[string]Backend{}, busy: map[string]bool{}, attempt: uuid.NewString(), clock: options.Clock,
		ctx: ctx, cancel: cancel, changed: make(chan struct{}), shutdownDone: make(chan struct{})}
	defer func() {
		if err != nil {
			cancel()
			if m.local != nil {
				_ = m.local.close()
			}
			for _, backend := range m.backends {
				_ = backend.Close()
			}
		}
	}()
	// An interrupted dispatch is never inferred successful or requeued. Keep
	// identity and receipts available for read-only native reconciliation.
	next := saved.clone()
	for _, r := range next.Sessions {
		r.Session.Archived, r.Session.ResumePending, r.Session.Mode = true, false, "human"
		if invalidateRequests(r) || r.ControlPending {
			r.Session.Fault = "Native work or control outcome is unknown after observer restart; inspect the native terminal"
		}
		r.ControlPending = false
	}
	if j := next.Job; j != nil && (j.Status == "starting" || j.Status == "created" || j.Status == "prompting") {
		j.Status = "unknown"
	}
	if err := m.commitLocked(next); err != nil {
		return nil, err
	}
	m.local, err = newLocalServer(m)
	if err != nil {
		return nil, err
	}
	factories := options.Factories
	if factories == nil {
		factories = map[string]func(BackendOptions) (Backend, error){
			"claude":   func(o BackendOptions) (Backend, error) { return NewClaude(o) },
			"codex":    func(o BackendOptions) (Backend, error) { return NewCodex(o) },
			"opencode": func(o BackendOptions) (Backend, error) { return NewOpenCode(o) },
		}
	}
	for _, harness := range []string{"claude", "codex", "opencode"} {
		factory := factories[harness]
		if factory == nil {
			continue
		}
		backend, err := factory(BackendOptions{StateDir: options.StateDir, Environment: slices.Clone(options.Environment), Host: m, Logger: options.Logger,
			Clock: options.Clock, Command: options.Command, CallbackURL: m.local.descriptor.URL, CallbackToken: m.local.descriptor.Token})
		if err != nil {
			return nil, xerrors.Errorf("initialize %s backend: %w", harness, err)
		}
		if backend == nil {
			return nil, xerrors.Errorf("initialize %s backend: nil backend", harness)
		}
		m.backends[harness] = backend
	}
	if err := m.local.start(); err != nil {
		return nil, err
	}
	return m, nil
}

// List returns independent session snapshots, including archived sessions.
func (m *Manager) List() []Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Session, 0, len(m.state.Sessions))
	for _, r := range m.state.Sessions {
		result = append(result, copySession(r.Session))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// Session returns an immutable snapshot of a known native binding.
func (m *Manager) Session(id string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.state.Sessions[id]
	if r == nil {
		return Session{}, xerrors.New("unknown native session")
	}
	return copySession(r.Session), nil
}

func copySession(s Session) Session { s.State = slices.Clone(s.State); return s }

// commitLocked publishes memory only after the complete replacement snapshot is
// durable. Any write failure permanently closes admission, including when a
// rename succeeded but its directory sync had an ambiguous outcome.
func (m *Manager) commitLocked(next snapshot) error {
	if m.persistErr != nil {
		return m.persistErr
	}
	for id, record := range next.Sessions {
		previous := m.state.Sessions[id]
		if previous != nil && previous.Session.Mode == record.Session.Mode {
			continue
		}
		observed := false
		for i := len(record.Events) - 1; i >= 0; i-- {
			event := record.Events[i]
			if previous != nil && event.Seq <= previous.Session.Cursor {
				break
			}
			if event.Type == "mode" {
				observed = eventString(event, "mode") == record.Session.Mode
				break
			}
		}
		if !observed {
			if err := appendJournalEvent(record, Event{Type: "mode", Data: map[string]any{"mode": record.Session.Mode}}); err != nil {
				return err
			}
		}
	}
	boundJournal(&next)
	if err := m.store.save(next); err != nil {
		m.persistErr = xerrors.New("native persistence failed; work outcome may be unknown, inspect private state")
		close(m.changed)
		m.changed = make(chan struct{})
		return m.persistErr
	}
	m.state = next
	close(m.changed)
	m.changed = make(chan struct{})
	return nil
}

func (m *Manager) availableLocked() error {
	if m.closed || m.stopping {
		return xerrors.New("native session manager is closed or stopping")
	}
	return m.persistErr
}

// Publish atomically commits a batch of observations and optional replacement
// backend receipts. Neither caller-owned maps nor slices are retained.
func (m *Manager) Publish(id string, events []Event, state json.RawMessage) error {
	if len(state) > maxStateBytes || (len(state) > 0 && !json.Valid(state)) || len(events) > maxEvents {
		return xerrors.New("invalid or oversized native observation")
	}
	data, err := json.Marshal(events)
	if err != nil || len(data) > maxSessionBytes {
		return xerrors.New("invalid or oversized native observation batch")
	}
	var owned []Event
	if err := json.Unmarshal(data, &owned); err != nil {
		return err
	}
	for _, event := range owned {
		encoded, _ := json.Marshal(event)
		if event.Type == "" || event.Data == nil || len(encoded)+32 > maxEventBytes {
			return xerrors.New("invalid or oversized native event")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return xerrors.New("native session manager is closed")
	}
	if m.state.Sessions[id] == nil {
		return xerrors.New("unknown native session")
	}
	next := m.state.clone()
	r := next.Sessions[id]
	if len(state) > 0 {
		r.Session.State = slices.Clone(state)
	}
	for _, event := range owned {
		mode := r.Session.Mode
		applyEvent(r, event)
		if event.Type != "mode" {
			if err := appendJournalEvent(r, event); err != nil {
				return err
			}
		}
		if r.Session.Mode != mode {
			if err := appendJournalEvent(r, Event{Type: "mode", Data: map[string]any{"mode": r.Session.Mode}}); err != nil {
				return err
			}
		}
	}
	updateJob(&next, id)
	return m.commitLocked(next)
}

func appendJournalEvent(r *sessionRecord, event Event) error {
	if r.Session.Cursor == ^uint64(0) {
		return xerrors.New("native event cursor exhausted")
	}
	r.Session.Cursor++
	event.Seq = r.Session.Cursor
	r.Events = append(r.Events, event)
	return nil
}

func eventString(e Event, key string) string { value, _ := e.Data[key].(string); return value }

func applyEvent(r *sessionRecord, e Event) {
	s := &r.Session
	id, turn := eventString(e, "requestId"), eventString(e, "turnId")
	request, known := r.Requests[id]
	active := known && (request.Status == "attempted" || request.Status == "running")
	switch e.Type {
	case "mode", "runtime.resumed", "runtime.registered", "session.start":
		mode := eventString(e, "mode")
		if mode == "human" || (mode == "automation" && !s.Archived && !s.ResumePending && !s.PendingInput && s.Fault == "") {
			s.Mode = mode
		}
	case "needs.input":
		pending, ok := e.Data["pending"].(bool)
		s.PendingInput = !ok || pending
		if s.PendingInput {
			s.Mode = "human"
		}
	case "turn.start":
		if turn != "" {
			s.ActiveTurnID = turn
		}
		if active {
			request.Status, request.TurnID = "running", turn
			r.Requests[id] = request
		}
	case "prompt.accepted":
		if active {
			request.Status = "running"
			r.Requests[id] = request
		}
	case "turn.complete", "prompt.rejected":
		if active && (request.TurnID == "" || request.TurnID == turn) {
			request.Status = "completed"
			if e.Type == "prompt.rejected" {
				request.Status = "rejected"
			}
			if eventString(e, "reason") == "error" {
				request.Status = "failed"
			}
			canceled, _ := e.Data["cancelled"].(bool)                                                         //nolint:misspell // Native event protocol spelling.
			if canceled || eventString(e, "reason") == "cancelled" || eventString(e, "reason") == "aborted" { //nolint:misspell // Native event protocol spelling.
				request.Status = "canceled"
			}
			r.Requests[id] = request
			if s.ActiveRequestID == id {
				s.ActiveRequestID = ""
			}
		}
		if e.Type == "turn.complete" && turn != "" && s.ActiveTurnID == turn {
			s.ActiveTurnID = ""
		}
	case "session.end":
		s.Archived, s.Mode = true, "human"
		invalidateRequests(r)
	case "error", "overflow", "command.error":
		s.Fault, s.Mode = "Native observation or command failed; inspect the native terminal before further automation", "human"
		invalidateRequests(r)
	}
}

func invalidateRequests(r *sessionRecord) bool {
	unknown := false
	for id, request := range r.Requests {
		if request.Status == "attempted" || request.Status == "running" {
			request.Status = "unknown"
			r.Requests[id] = request
			unknown = true
		}
	}
	r.Session.ActiveRequestID, r.Session.ActiveTurnID = "", ""
	return unknown
}

// Events reads journal entries strictly after a cursor. Expired cursors fail
// explicitly; callers must reconcile state rather than silently miss history.
// A waiting reader at the current cursor blocks even for archived sessions, and
// never starts native work. Caller cancellation or Close/Shutdown unblocks it.
//
//nolint:revive // The shared event-reader contract uses a long-poll control flag.
func (m *Manager) Events(ctx context.Context, id string, after uint64, wait bool) ([]Event, uint64, error) {
	for {
		m.mu.Lock()
		r := m.state.Sessions[id]
		if r == nil {
			m.mu.Unlock()
			return nil, 0, xerrors.New("unknown native session")
		}
		cursor := r.Session.Cursor
		if after < r.Session.HistoryStartCursor || after > cursor {
			m.mu.Unlock()
			return nil, cursor, xerrors.New("native event cursor expired or invalid; reconcile session state")
		}
		if m.persistErr != nil {
			err := m.persistErr
			m.mu.Unlock()
			return nil, cursor, err
		}
		if wait && after == cursor && (m.closed || m.stopping) {
			m.mu.Unlock()
			return nil, cursor, xerrors.New("native observation is unavailable; saved history remains readable")
		}
		if after < cursor || !wait {
			var events []Event
			for _, event := range r.Events {
				if event.Seq > after {
					events = append(events, event)
				}
			}
			data, err := json.Marshal(events)
			m.mu.Unlock()
			if err != nil {
				return nil, cursor, err
			}
			var owned []Event
			err = json.Unmarshal(data, &owned)
			return owned, cursor, err
		}
		changed := m.changed
		m.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, cursor, ctx.Err()
		}
	}
}

type operationResult[T any] struct {
	value T
	err   error
}

// The caller may stop waiting, but never owns native operation cancellation.
func runManagerOperation[T any](ctx context.Context, m *Manager, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	m.mu.Lock()
	if err := m.availableLocked(); err != nil {
		m.mu.Unlock()
		return zero, err
	}
	m.wg.Add(1)
	m.mu.Unlock()
	result := make(chan operationResult[T], 1)
	go func() { defer m.wg.Done(); value, err := fn(m.ctx); result <- operationResult[T]{value, err} }()
	select {
	case result := <-result:
		return result.value, result.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// Create registers the exact native identity durably before resuming observation.
// It returns only after readiness and native human control have been confirmed.
func (m *Manager) Create(ctx context.Context, harness, cwd string) (Session, error) {
	return runManagerOperation(ctx, m, func(ctx context.Context) (Session, error) { return m.createSession(ctx, harness, cwd) })
}

func (m *Manager) createSession(ctx context.Context, harness, cwd string) (Session, error) {
	if harness == "" {
		harness = "claude"
	}
	if !filepath.IsAbs(cwd) || len(cwd) > 4096 || strings.ContainsRune(cwd, 0) {
		return Session{}, xerrors.New("native cwd must be an absolute path")
	}
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		return Session{}, xerrors.New("native cwd must name an existing directory")
	}
	m.mu.Lock()
	backend := m.backends[harness]
	if backend == nil {
		m.mu.Unlock()
		return Session{}, xerrors.New("unknown or unavailable native harness")
	}
	if err := m.availableLocked(); err != nil {
		m.mu.Unlock()
		return Session{}, err
	}
	if len(m.state.Sessions)+m.creating >= maxSessions {
		m.mu.Unlock()
		return Session{}, xerrors.New("native session archive budget exhausted")
	}
	m.creating++
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.creating--; m.mu.Unlock() }()
	s, err := backend.Create(ctx, cwd)
	if err != nil {
		var setup *creationSetupError
		if errors.As(err, &setup) {
			return Session{}, xerrors.Errorf("native %s setup failed before session creation: %w", harness, setup)
		}
		return Session{}, xerrors.New("native creation outcome is unknown; inspect the native terminal before retrying")
	}
	s.Mode, s.Archived, s.ResumePending = "human", false, true
	s.Cursor, s.HistoryStartCursor, s.ActiveRequestID, s.ActiveTurnID = 0, 0, "", ""
	if s.StartedAt == 0 {
		s.StartedAt = m.clock.Now().UnixMilli()
	}
	if s.Harness != harness || s.Cwd != cwd || !validSession(s) {
		return Session{}, xerrors.New("native creation returned an invalid identity; inspect before retrying")
	}
	m.mu.Lock()
	if m.closed || m.state.Sessions[s.ID] != nil {
		m.mu.Unlock()
		return Session{}, xerrors.New("native identity already exists or manager closed")
	}
	next := m.state.clone()
	next.Sessions[s.ID] = &sessionRecord{Session: copySession(s), Requests: map[string]requestRecord{}, RestoreAttempt: m.attempt}
	err = m.commitLocked(next)
	if err == nil {
		m.busy[s.ID] = true
	}
	m.mu.Unlock()
	if err != nil {
		return Session{}, err
	}
	defer m.release(s.ID)
	if err := backend.Resume(ctx, s); err != nil {
		return s, m.fault(s.ID, "Native readiness outcome is unknown; inspect the native terminal")
	}
	if err := backend.SetMode(ctx, s, "human"); err != nil {
		return s, m.fault(s.ID, "Native human control was not confirmed; inspect the native terminal")
	}
	if err := m.ready(s.ID, "human"); err != nil {
		return s, err
	}
	s, err = m.Session(s.ID)
	if err == nil && s.Fault != "" {
		err = xerrors.New("native readiness was not confirmed; inspect the native terminal")
	}
	return s, err
}

func (m *Manager) release(id string) { m.mu.Lock(); delete(m.busy, id); m.mu.Unlock() }

func (m *Manager) ready(id, mode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.availableLocked(); err != nil {
		return err
	}
	next := m.state.clone()
	r := next.Sessions[id]
	r.Session.ResumePending = false
	if r.Session.Archived {
		return errors.Join(xerrors.New("native session ended before readiness was confirmed"), m.commitLocked(next))
	}
	if mode == "automation" && (r.Session.PendingInput || r.Session.ActiveRequestID != "" || r.Session.ActiveTurnID != "") {
		return errors.Join(xerrors.New("native session became busy before automation was confirmed"), m.commitLocked(next))
	}
	if r.Session.Fault == "" {
		r.Session.Mode = mode
	}
	return m.commitLocked(next)
}

func (m *Manager) fault(id, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return xerrors.New(reason)
	}
	next := m.state.clone()
	r := next.Sessions[id]
	if r == nil {
		return xerrors.New(reason)
	}
	if r.Session.ResumePending {
		r.Session.Archived = true
	}
	r.Session.Fault, r.Session.Mode, r.Session.ResumePending = reason, "human", false
	r.ControlPending = false
	invalidateRequests(r)
	if r.Session.Cursor < ^uint64(0) {
		r.Session.Cursor++
		r.Events = append(r.Events, Event{Seq: r.Session.Cursor, Type: "error", Data: map[string]any{"message": reason}})
	}
	updateJob(&next, id)
	return errors.Join(xerrors.New(reason), m.commitLocked(next))
}

// Restore attempts each saved identity once per manager lifetime. Failures are
// independent and never cause prompt replay or creation of replacement sessions.
func (m *Manager) Restore(ctx context.Context) error {
	_, err := runManagerOperation(ctx, m, func(ctx context.Context) (struct{}, error) {
		sessions := m.List()
		errs := make(chan error, len(sessions))
		var wg sync.WaitGroup
		for _, s := range sessions {
			wg.Go(func() { errs <- m.restoreSession(ctx, s.ID) })
		}
		wg.Wait()
		close(errs)
		var joined error
		for err := range errs {
			joined = errors.Join(joined, err)
		}
		return struct{}{}, joined
	})
	return err
}

func (m *Manager) restoreSession(ctx context.Context, id string) error {
	m.mu.Lock()
	r := m.state.Sessions[id]
	if err := m.availableLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	if r.RestoreAttempt == m.attempt {
		m.mu.Unlock()
		return nil
	}
	if m.busy[id] {
		m.mu.Unlock()
		return xerrors.New("native session operation already in progress")
	}
	backend := m.backends[r.Session.Harness]
	next := m.state.clone()
	next.Sessions[id].RestoreAttempt = m.attempt
	next.Sessions[id].Session.ResumePending = true
	next.Sessions[id].Session.Archived = false
	next.Sessions[id].Session.Mode = "human"
	if err := m.commitLocked(next); err != nil {
		m.mu.Unlock()
		return err
	}
	m.busy[id] = true
	s := copySession(next.Sessions[id].Session)
	m.mu.Unlock()
	defer m.release(id)
	if backend == nil {
		return m.fault(id, "Native harness is unavailable; saved history remains readable")
	}
	if err := backend.Resume(ctx, s); err != nil {
		return m.fault(id, "Native restore outcome is unknown; inspect the native terminal")
	}
	if err := m.ready(id, "human"); err != nil {
		return err
	}
	current, err := m.Session(id)
	if err != nil {
		return err
	}
	if current.Fault != "" || current.PendingInput || current.ActiveTurnID != "" {
		return nil
	}
	if err := backend.SetMode(ctx, current, "automation"); err != nil {
		return m.fault(id, "Native automation control was not confirmed; inspect the native terminal")
	}
	return m.ready(id, "automation")
}

// Submit durably claims a UUID before the sole native dispatch attempt. Repeated
// identical UUIDs are acknowledged without invoking the backend again.
func (m *Manager) Submit(ctx context.Context, id string, prompt Prompt) error {
	_, err := runManagerOperation(ctx, m, func(ctx context.Context) (struct{}, error) { return struct{}{}, m.submitPrompt(ctx, id, prompt) })
	return err
}

func (m *Manager) submitPrompt(ctx context.Context, id string, prompt Prompt) error {
	if !validUUID(prompt.ID) || strings.TrimSpace(prompt.Text) == "" || len(prompt.Text) > maxPromptBytes {
		return xerrors.New("prompt requires a UUID and nonempty text up to 100000 bytes")
	}
	prompt.ID = strings.ToLower(prompt.ID)
	hash := textHash(prompt.Text)
	m.mu.Lock()
	if err := m.availableLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	r := m.state.Sessions[id]
	if r == nil {
		m.mu.Unlock()
		return xerrors.New("unknown native session")
	}
	if old, ok := r.Requests[prompt.ID]; ok {
		m.mu.Unlock()
		if old.TextHash != hash {
			return xerrors.New("native request UUID was already used for different text")
		}
		if old.Status == "unknown" {
			return xerrors.New("native request outcome is unknown; it was not retried")
		}
		return nil
	}
	if m.busy[id] || r.Session.Mode != "automation" || r.Session.Archived || r.Session.ResumePending || r.Session.Fault != "" || r.Session.PendingInput || r.Session.ActiveRequestID != "" || r.Session.ActiveTurnID != "" {
		m.mu.Unlock()
		return xerrors.New("native session is busy, faulted, or under human control")
	}
	if len(r.Requests) >= maxRequests {
		m.mu.Unlock()
		return xerrors.New("native request history budget exhausted")
	}
	backend := m.backends[r.Session.Harness]
	if backend == nil {
		m.mu.Unlock()
		return xerrors.New("native harness unavailable")
	}
	next := m.state.clone()
	next.Sessions[id].Requests[prompt.ID] = requestRecord{ID: prompt.ID, TextHash: hash, Status: "attempted"}
	next.Sessions[id].Session.ActiveRequestID = prompt.ID
	if err := m.commitLocked(next); err != nil {
		m.mu.Unlock()
		return err
	}
	s := copySession(next.Sessions[id].Session)
	m.mu.Unlock()
	if err := backend.Prompt(ctx, s, prompt); err != nil {
		return m.fault(id, "Native prompt outcome may be unknown; inspect the native terminal, nothing was retried")
	}
	return nil
}

// SetMode changes native control without dispatching input.
func (m *Manager) SetMode(ctx context.Context, id, mode string) error {
	_, err := runManagerOperation(ctx, m, func(ctx context.Context) (struct{}, error) { return struct{}{}, m.changeMode(ctx, id, mode) })
	return err
}

func (m *Manager) changeMode(ctx context.Context, id, mode string) error {
	if mode != "human" && mode != "automation" {
		return xerrors.New("expected human or automation mode")
	}
	m.mu.Lock()
	if err := m.availableLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	r := m.state.Sessions[id]
	if r == nil {
		m.mu.Unlock()
		return xerrors.New("unknown native session")
	}
	if m.busy[id] || r.Session.ResumePending {
		m.mu.Unlock()
		return xerrors.New("native session operation already in progress")
	}
	if mode == "automation" && (r.Session.Archived || r.Session.Fault != "" || r.Session.PendingInput || r.Session.ActiveRequestID != "" || r.Session.ActiveTurnID != "") {
		m.mu.Unlock()
		return xerrors.New("native session is not ready for automation")
	}
	backend := m.backends[r.Session.Harness]
	if backend == nil {
		m.mu.Unlock()
		return xerrors.New("native harness unavailable")
	}
	// Claim control before the native call. Admission is blocked by busy, and
	// an interrupted control attempt remains conservative after restart.
	next := m.state.clone()
	next.Sessions[id].ControlPending = true
	if err := m.commitLocked(next); err != nil {
		m.mu.Unlock()
		return err
	}
	m.busy[id] = true
	s := copySession(next.Sessions[id].Session)
	m.mu.Unlock()
	defer m.release(id)
	if err := backend.SetMode(ctx, s, mode); err != nil {
		return m.fault(id, "Native mode change was not confirmed; inspect the native terminal")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.availableLocked(); err != nil {
		return err
	}
	next = m.state.clone()
	next.Sessions[id].ControlPending = false
	current := &next.Sessions[id].Session
	if mode == "automation" && (current.Fault != "" || current.PendingInput || current.Archived || current.ActiveRequestID != "" || current.ActiveTurnID != "") {
		current.Mode = "human"
		return errors.Join(xerrors.New("native session became unavailable during mode change"), m.commitLocked(next))
	}
	next.Sessions[id].Session.Mode = mode
	return m.commitLocked(next)
}

// Cancel makes one cancellation attempt for a known request. It never infers
// cancellation from a successful RPC alone; native events supply the outcome.
func (m *Manager) Cancel(ctx context.Context, id, requestID string) error {
	_, err := runManagerOperation(ctx, m, func(ctx context.Context) (struct{}, error) {
		m.mu.Lock()
		if err := m.availableLocked(); err != nil {
			m.mu.Unlock()
			return struct{}{}, err
		}
		r := m.state.Sessions[id]
		if r == nil {
			m.mu.Unlock()
			return struct{}{}, xerrors.New("unknown native session")
		}
		requestID = strings.ToLower(requestID)
		request, ok := r.Requests[requestID]
		if !ok {
			m.mu.Unlock()
			return struct{}{}, xerrors.New("unknown native request")
		}
		if request.CancelAttempted || (request.Status != "attempted" && request.Status != "running") {
			m.mu.Unlock()
			return struct{}{}, nil
		}
		backend := m.backends[r.Session.Harness]
		if backend == nil {
			m.mu.Unlock()
			return struct{}{}, xerrors.New("native harness unavailable")
		}
		next := m.state.clone()
		request.CancelAttempted = true
		next.Sessions[id].Requests[requestID] = request
		if err := m.commitLocked(next); err != nil {
			m.mu.Unlock()
			return struct{}{}, err
		}
		s := copySession(next.Sessions[id].Session)
		m.mu.Unlock()
		if err := backend.Cancel(ctx, s, requestID); err != nil {
			return struct{}{}, m.fault(id, "Native cancellation outcome is unknown; inspect the native terminal")
		}
		return struct{}{}, nil
	})
	return err
}

// Attach disables automation and independently verifies native attachment, even
// when observation failed. It never calls Resume or Create. Claude additionally
// requires its mod to acknowledge human control before returning a command.
func (m *Manager) Attach(ctx context.Context, id string) (Command, error) {
	return runManagerOperation(ctx, m, func(ctx context.Context) (Command, error) {
		m.mu.Lock()
		if err := m.availableLocked(); err != nil {
			m.mu.Unlock()
			return Command{}, err
		}
		r := m.state.Sessions[id]
		if r == nil {
			m.mu.Unlock()
			return Command{}, xerrors.New("unknown native session")
		}
		if m.busy[id] {
			m.mu.Unlock()
			return Command{}, xerrors.New("native session operation already in progress")
		}
		backend := m.backends[r.Session.Harness]
		if backend == nil {
			m.mu.Unlock()
			return Command{}, xerrors.New("native harness unavailable")
		}
		next := m.state.clone()
		next.Sessions[id].Session.Mode = "human"
		next.Sessions[id].ControlPending = true
		if err := m.commitLocked(next); err != nil {
			m.mu.Unlock()
			return Command{}, err
		}
		m.busy[id] = true
		s := copySession(next.Sessions[id].Session)
		m.mu.Unlock()
		defer m.release(id)

		command, attachErr := backend.Attach(ctx, s)
		var modeErr error
		if attachErr == nil {
			modeErr = backend.SetMode(ctx, s, "human")
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if err := m.availableLocked(); err != nil {
			return Command{}, errors.Join(attachErr, err)
		}
		next = m.state.clone()
		r = next.Sessions[id]
		r.ControlPending = false
		r.Session.Mode = "human"
		const controlFailure = "native human control was not confirmed; inspect the native terminal"
		if modeErr != nil {
			r.Session.Fault = controlFailure
			invalidateRequests(r)
			updateJob(&next, id)
			if err := appendJournalEvent(r, Event{Type: "error", Data: map[string]any{"message": controlFailure}}); err != nil {
				return Command{}, err
			}
		}
		if err := m.commitLocked(next); err != nil {
			return Command{}, errors.Join(attachErr, err)
		}
		if attachErr != nil {
			return Command{}, attachErr
		}
		// Codex and OpenCode control is manager admission policy. Their verified
		// native terminals remain usable without a healthy observer. Claude's
		// mod has its own native gate, so its acknowledgement is mandatory.
		if modeErr != nil && s.Harness == "claude" {
			return Command{}, xerrors.New(controlFailure)
		}
		return command, nil
	})
}

// Handler exposes only the authenticated local REST API, never an ACP endpoint.
func (m *Manager) Handler() http.Handler { return m.local.handler }

// Close detaches observers, cancels their work, and releases the manager lock.
// It never calls native Shutdown and never deletes native or saved histories.
func (m *Manager) Close() error {
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		close(m.changed)
		m.changed = make(chan struct{})
		m.mu.Unlock()
		m.cancel()
		m.closeErr = m.local.close()
		for _, backend := range m.backends {
			m.closeErr = errors.Join(m.closeErr, backend.Close())
		}
		m.wg.Wait()
		m.mu.Lock()
		next := m.state.clone()
		for id, r := range next.Sessions {
			if invalidateRequests(r) {
				r.Session.Fault = "Native work outcome is unknown after observer close; inspect the native terminal"
			}
			r.Session.Mode, r.Session.Archived, r.Session.ResumePending = "human", true, false
			updateJob(&next, id)
		}
		m.closeErr = errors.Join(m.closeErr, m.commitLocked(next), m.store.dir.close())
		m.mu.Unlock()
	})
	return m.closeErr
}

const managerShutdownTimeout = 30 * time.Second

// Shutdown explicitly stops native services. The journal, configuration and
// histories are retained for restoration by the next manager. This manager will
// not admit new native work afterward. The caller owns only its wait; native
// cleanup has an independent bounded lifetime, including after Close.
func (m *Manager) Shutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.shutdownOnce.Do(func() {
		m.mu.Lock()
		if m.closed {
			m.shutdownErr = xerrors.New("native session manager is closed")
			close(m.shutdownDone)
			m.mu.Unlock()
			return
		}
		m.stopping = true
		m.wg.Add(1)
		next := m.state.clone()
		for id, r := range next.Sessions {
			invalidateRequests(r)
			r.Session.Mode, r.Session.Archived = "human", true
			updateJob(&next, id)
		}
		m.shutdownErr = m.commitLocked(next)
		m.mu.Unlock()
		m.cancel()
		shutdownCtx, cancel := context.WithCancelCause(context.Background())
		timer := m.clock.AfterFunc(managerShutdownTimeout, func() { cancel(context.DeadlineExceeded) }, "manager", "shutdown")
		go func() {
			defer m.wg.Done()
			defer close(m.shutdownDone)
			defer cancel(context.Canceled)
			defer timer.Stop()
			errs := make(chan error, len(m.backends))
			var wg sync.WaitGroup
			for _, backend := range m.backends {
				wg.Go(func() { errs <- backend.Shutdown(shutdownCtx) })
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				m.shutdownErr = errors.Join(m.shutdownErr, err)
			}
			m.shutdownErr = errors.Join(m.shutdownErr, context.Cause(shutdownCtx))
		}()
	})
	select {
	case <-m.shutdownDone:
		return m.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
