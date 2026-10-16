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

type sessionAccess uint8

const (
	readSession sessionAccess = iota
	recoverSession
)

// createSession returns a new initialized FSM in idle.
func (m *Manager) createSession(ctx context.Context, req workspacesdk.ACPCreateSessionRequest) (*SessionFSM, error) {
	if err := validateSessionRequest(m.fs, req.HarnessSlug, req.WorkingDirectory); err != nil {
		return nil, err
	}
	req.Config = maps.Clone(req.Config)
	if len(req.Config) == 0 {
		req.Config = nil
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrUnavailable
	}
	attempt := &initialization{done: make(chan struct{})}
	m.wg.Go(func() {
		attempt.fsm, attempt.err = m.initializeAndRegisterSession(req, "")
		close(attempt.done)
	})
	m.mu.Unlock()
	return awaitInitialization(ctx, attempt)
}

// initializeAndRegisterSession publishes only initialized FSMs. It runs in a
// manager-owned goroutine; caller cancellation leaves initialization running.
func (m *Manager) initializeAndRegisterSession(req workspacesdk.ACPCreateSessionRequest, nativeID string) (*SessionFSM, error) {
	m.mu.Lock()
	cfg, found := m.configs[req.HarnessSlug]
	envOverrides := m.envOverrides
	m.mu.Unlock()
	if !found {
		return nil, xerrors.Errorf("%w: harness is missing or disabled", ErrUnavailable)
	}
	s, err := startSessionRuntime(m.ctx, cfg, req, nativeID, sessionRuntimeOptions{
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
	if err := m.registerSession(s, nativeID); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (m *Manager) registerSession(s *SessionFSM, nativeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrUnavailable
	}
	if nativeID == "" && m.sessions[s.id] != nil {
		return ErrConflict
	}
	m.sessions[s.id] = s
	m.instances = append(m.instances, s)
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

// get restores absent sessions and returns existing errors without recovery.
func (m *Manager) get(ctx context.Context, id workspacesdk.ACPSessionID) (*SessionFSM, error) {
	return m.obtain(ctx, id, readSession)
}

// recover replaces a failed instance or returns an already recovered one.
// Existing failed instances remain readable until the manager closes.
func (m *Manager) recover(ctx context.Context, id workspacesdk.ACPSessionID) (*SessionFSM, error) {
	return m.obtain(ctx, id, recoverSession)
}

// obtain returns the loaded session or restores it if absent.
// Its third argument, access, controls how a failed session is handled:
// readSession returns it so callers can read its output; recoverSession
// replaces it and its adapter. Concurrent callers share one restoration attempt.
func (m *Manager) obtain(ctx context.Context, id workspacesdk.ACPSessionID, access sessionAccess) (*SessionFSM, error) {
	if id.SessionID == "" || id.HarnessSlug == "" || !filepath.IsAbs(id.WorkingDirectory) {
		return nil, ErrInvalid
	}
	for {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return nil, ErrUnavailable
		}
		s := m.sessions[id]
		// Reads retain the failed instance while recovery runs.
		if s != nil && access == readSession {
			m.mu.Unlock()
			return s, nil
		}
		attempt := m.initializing[id]
		if attempt != nil {
			select {
			case <-attempt.done:
			default:
				m.mu.Unlock()
				return awaitInitialization(ctx, attempt)
			}
		}
		m.mu.Unlock()
		if s != nil {
			r, err := s.Read(ctx, nil)
			if err != nil {
				return nil, err
			}
			if r.Session.Status != workspacesdk.ACPSessionStatusError {
				return s, nil
			}
		}
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return nil, ErrUnavailable
		}
		// Another caller may have changed either entry while the lock was
		// released for s.Read. Start over to avoid starting a second restoration.
		if m.sessions[id] != s || m.initializing[id] != attempt {
			m.mu.Unlock()
			continue
		}
		req := workspacesdk.ACPCreateSessionRequest{HarnessSlug: id.HarnessSlug, WorkingDirectory: id.WorkingDirectory}
		if s != nil {
			req = s.request
		}
		attempt = &initialization{done: make(chan struct{})}
		m.initializing[id] = attempt
		m.wg.Go(func() {
			if s != nil {
				s.proc.close()
			}
			attempt.fsm, attempt.err = m.initializeAndRegisterSession(req, id.SessionID)
			close(attempt.done)
		})
		m.mu.Unlock()
		return awaitInitialization(ctx, attempt)
	}
}
