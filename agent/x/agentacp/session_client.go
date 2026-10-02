package agentacp

import (
	"context"
	"sync"

	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
)

// sessionClient collects history during load, then delivers live updates to
// its attached session. mu makes the history handoff atomic with callbacks.
type sessionClient struct {
	client
	mu               sync.Mutex
	session          *SessionFSM
	historySessionID string
	pendingHistory   []acp.SessionNotification
}

func (c *sessionClient) beginHistoryReplay(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.historySessionID = id
}

func (c *sessionClient) attachSession(s *SessionFSM) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := s.importHistory(c.pendingHistory); err != nil {
		return err
	}
	c.pendingHistory = nil
	c.historySessionID = ""
	c.session = s
	return nil
}

func (c *sessionClient) SessionUpdate(ctx context.Context, req acp.SessionNotification) error {
	c.mu.Lock()
	if s := c.session; s != nil {
		c.mu.Unlock()
		return s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req})
	}
	defer c.mu.Unlock()
	// Before attachment, only session/load history belongs in the transcript.
	if c.historySessionID == "" {
		return nil
	}
	if string(req.SessionId) != c.historySessionID {
		return xerrors.Errorf("%w: notification session ID mismatch", ErrInvalid)
	}
	c.pendingHistory = append(c.pendingHistory, req)
	return nil
}

var _ acp.Client = (*sessionClient)(nil)
