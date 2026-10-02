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
	if err := m.registerSession(s); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (m *Manager) registerSession(s *SessionFSM) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrManagerClosed
	}
	if m.sessions[s.id] != nil {
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

// get returns an initialized session without creating or restoring one.
func (m *Manager) get(ctx context.Context, id workspacesdk.ACPSessionID) (*SessionFSM, error) {
	if id.SessionID == "" || id.HarnessSlug == "" || !filepath.IsAbs(id.WorkingDirectory) {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrManagerClosed
	}
	s := m.sessions[id]
	if s == nil {
		return nil, ErrNotFound
	}
	return s, nil
}
