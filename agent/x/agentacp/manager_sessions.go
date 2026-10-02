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
		return nil, ErrManagerClosed
	}
	attempt := &initialization{done: make(chan struct{})}
	m.wg.Go(func() {
		attempt.fsm, attempt.err = m.initializeAndRegisterSession(initReq)
		close(attempt.done)
	})
	m.mu.Unlock()
	return m.awaitInitialization(ctx, attempt)
}

// initializeAndRegisterSession publishes only initialized FSMs. It runs in a
// manager-owned goroutine so shutdown tracks in-flight setup.
func (m *Manager) initializeAndRegisterSession(req sessionInitRequest) (*SessionFSM, error) {
	m.mu.Lock()
	cfg, found := m.configs[req.harnessSlug]
	m.mu.Unlock()
	if !found {
		return nil, xerrors.Errorf("%w: harness is missing or disabled", ErrUnavailable)
	}
	env := m.envInfo.Environ()
	if m.updateEnv != nil {
		var err error
		env, err = m.updateEnv(env)
		if err != nil {
			return nil, xerrors.Errorf("%w: prepare ACP environment: %v", ErrUnavailable, err)
		}
	}
	s, err := startSessionRuntime(m.ctx, cfg, req, sessionRuntimeOptions{
		launch: launchOptions{
			clock:   m.clock,
			execer:  m.execer,
			envInfo: m.envInfo,
			env:     normalizeEnv(env),
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
		return ErrManagerClosed
	}
	if mode == registrationCreate && m.sessions[s.id] != nil {
		return ErrConflict
	}
	m.sessions[s.id] = s
	return nil
}

func (m *Manager) discardSession(s *SessionFSM) {
	// Keep the session registered until it closes so manager shutdown waits
	// for its process even when creation is canceled concurrently.
	s.Close()
	m.mu.Lock()
	if m.sessions[s.id] == s {
		delete(m.sessions, s.id)
	}
	m.mu.Unlock()
}

func (m *Manager) awaitInitialization(ctx context.Context, attempt *initialization) (*SessionFSM, error) {
	select {
	case <-ctx.Done():
		<-attempt.done
		if attempt.fsm != nil {
			m.discardSession(attempt.fsm)
		}
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
		return nil, ErrManagerClosed
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
		return nil, ErrManagerClosed
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
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-attempt.done:
		return attempt.fsm, attempt.err
	}
}

// restoreSession runs in the initialization goroutine, which reserves this
// session's recovery without holding the manager lock during FSM operations.
func (m *Manager) restoreSession(id workspacesdk.ACPSessionID, oldSession *SessionFSM) (*SessionFSM, error) {
	req := sessionInitRequest{harnessSlug: id.HarnessSlug, workingDirectory: id.WorkingDirectory}
	if oldSession != nil {
		unlock, err := oldSession.lock(m.ctx)
		if err != nil {
			return nil, err
		}
		failed := oldSession.info.Status == workspacesdk.ACPSessionStatusError
		unlock()
		if !failed {
			return oldSession, nil
		}
		req = oldSession.request
	}
	req.nativeID = id.SessionID
	restored, err := m.initializeAndRegisterSession(req)
	if err != nil {
		return nil, err
	}
	if oldSession != nil {
		oldSession.Close()
	}
	return restored, nil
}
