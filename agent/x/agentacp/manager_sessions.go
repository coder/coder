package agentacp

import (
	"context"
	"maps"
	"path/filepath"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type initialization struct {
	done chan struct{}
	fsm  *SessionFSM
	err  error
}

type registrationMode int

const (
	registrationCreate registrationMode = iota
	registrationRestore
)

// createSession returns a new initialized FSM in idle.
func (m *Manager) createSession(ctx context.Context, req workspacesdk.ACPCreateSessionRequest) (*SessionFSM, error) {
	if err := validateSessionRequest(m.fs, req.HarnessSlug, req.WorkingDirectory); err != nil {
		return nil, err
	}
	initReq := sessionInitRequest{
		harnessSlug:      req.HarnessSlug,
		workingDirectory: req.WorkingDirectory,
		config:           maps.Clone(req.Config),
	}
	if len(initReq.config) == 0 {
		initReq.config = nil
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrUnavailable
	}
	attempt := &initialization{done: make(chan struct{})}
	m.wg.Go(func() {
		attempt.fsm, attempt.err = m.initializeAndRegisterSession(initReq)
		close(attempt.done)
	})
	m.mu.Unlock()
	return awaitInitialization(ctx, attempt)
}

// initializeAndRegisterSession publishes only initialized FSMs. It runs in a
// manager-owned goroutine; caller cancellation leaves initialization running.
func (m *Manager) initializeAndRegisterSession(req sessionInitRequest) (*SessionFSM, error) {
	m.mu.Lock()
	cfg, found := m.configs[req.harnessSlug]
	envOverrides := m.envOverrides
	m.mu.Unlock()
	if !found {
		return nil, xerrors.Errorf("%w: harness is missing or disabled", ErrUnavailable)
	}
	s, err := startSessionRuntime(m.ctx, cfg, req, sessionRuntimeOptions{
		launch: launchOptions{
			clock:        m.clock,
			execer:       m.execer,
			envInfo:      m.envInfo,
			envOverrides: envOverrides,
		},
		fs:     m.fs,
		logger: m.logger,
	})
	if err != nil {
		return nil, err
	}
	mode := registrationCreate
	if req.nativeID != "" {
		mode = registrationRestore
	}
	if err := m.registerSession(s, mode); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (m *Manager) registerSession(s *SessionFSM, mode registrationMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrUnavailable
	}
	if mode == registrationCreate && m.sessions[s.id] != nil {
		return ErrConflict
	}
	m.sessions[s.id] = s
	return nil
}

func awaitInitialization(ctx context.Context, attempt *initialization) (*SessionFSM, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-attempt.done:
		return attempt.fsm, attempt.err
	}
}

// get returns the current session, restoring it only if absent.
func (m *Manager) get(ctx context.Context, id workspacesdk.ACPSessionID) (*SessionFSM, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrUnavailable
	}
	s := m.sessions[id]
	m.mu.Unlock()
	if s != nil {
		return s, nil
	}
	return m.recover(ctx, id)
}

// recover loads absent sessions or replaces failed instances. Concurrent callers
// share one manager-owned attempt; cancellation only stops the caller's wait.
// Failed instances remain readable until successfully replaced or closed.
func (m *Manager) recover(ctx context.Context, id workspacesdk.ACPSessionID) (*SessionFSM, error) {
	if id.SessionID == "" || id.HarnessSlug == "" || !filepath.IsAbs(id.WorkingDirectory) {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrUnavailable
	}
	attempt := m.initializing[id]
	if attempt == nil {
		s := m.sessions[id]
		attempt = &initialization{done: make(chan struct{})}
		m.initializing[id] = attempt
		m.wg.Go(func() {
			attempt.fsm, attempt.err = m.restoreSession(id, s)
			m.mu.Lock()
			defer m.mu.Unlock()
			delete(m.initializing, id)
			close(attempt.done)
		})
	}
	m.mu.Unlock()
	return awaitInitialization(ctx, attempt)
}

// restoreSession runs in the initialization goroutine, which reserves this
// session's recovery without holding the manager lock during FSM operations.
func (m *Manager) restoreSession(id workspacesdk.ACPSessionID, s *SessionFSM) (*SessionFSM, error) {
	req := sessionInitRequest{harnessSlug: id.HarnessSlug, workingDirectory: id.WorkingDirectory}
	if s != nil {
		if err := s.lock(m.ctx); err != nil {
			return nil, err
		}
		failed := s.info.Status == workspacesdk.ACPSessionStatusError
		s.mu.Unlock()
		if !failed {
			return s, nil
		}
		req = s.request
		s.proc.close()
	}
	req.nativeID = id.SessionID
	restored, err := m.initializeAndRegisterSession(req)
	if err != nil {
		return nil, err
	}
	if s != nil {
		s.Close()
	}
	return restored, nil
}
