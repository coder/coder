package agentacp

import (
	"context"
	"sync"

	acp "github.com/coder/acp-go-sdk"
)

// sessionClient delivers updates after initialization attaches its session.
// mu makes attachment atomic with callbacks.
type sessionClient struct {
	client
	mu      sync.Mutex
	session *SessionFSM
}

func (c *sessionClient) attachSession(s *SessionFSM) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := s.lock(s.ctx); err != nil {
		return err
	}
	s.mu.Unlock()
	c.session = s
	return nil
}

func (c *sessionClient) SessionUpdate(ctx context.Context, req acp.SessionNotification) error {
	c.mu.Lock()
	if s := c.session; s != nil {
		c.mu.Unlock()
		return s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req})
	}
	c.mu.Unlock()
	// Setup notifications arrive before a session is available to receive them.
	return nil
}

var _ acp.Client = (*sessionClient)(nil)
