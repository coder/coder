package agentacp

import (
	"context"
	"maps"
	"path/filepath"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type initialization struct {
	request workspacesdk.ACPCreateSessionRequest
	done    chan struct{}
	fsm     *SessionFSM
	err     error
}

type sessionAccess uint8

const (
	readSession sessionAccess = iota
	recoverSession
)

// createSession returns a new initialized FSM in idle.
func (m *Manager) createSession(ctx context.Context, req workspacesdk.ACPCreateSessionRequest) (*SessionFSM, error) {
	if err := m.validateSessionRequest(req.HarnessSlug, req.WorkingDirectory); err != nil {
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
	attempt := &initialization{request: req, done: make(chan struct{})}
	m.wg.Go(func() { m.initializeSession(attempt, workspacesdk.ACPSessionID{}) })
	m.mu.Unlock()
	return awaitInitialization(ctx, attempt)
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
		attempt = &initialization{request: req, done: make(chan struct{})}
		m.initializing[id] = attempt
		m.wg.Go(func() {
			if s != nil {
				s.proc.close()
			}
			m.initializeSession(attempt, id)
		})
		m.mu.Unlock()
		return awaitInitialization(ctx, attempt)
	}
}
