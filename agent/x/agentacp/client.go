package agentacp

import (
	"context"
	"sync"

	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
)

// The lock protects only the initialization handoff. Once bound, callbacks
// apply updates to their own instance and never consult the registry.
type client struct {
	mu       sync.Mutex
	session  *SessionFSM
	replayID string
	replay   []acp.SessionNotification
}

func (c *client) beginReplay(id string) { c.mu.Lock(); defer c.mu.Unlock(); c.replayID = id }
func (c *client) replaying() bool       { c.mu.Lock(); defer c.mu.Unlock(); return c.replayID != "" }
func (c *client) bind(s *SessionFSM) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := s.lock(s.ctx); err != nil {
		return err
	}
	defer s.mu.Unlock()
	for _, req := range c.replay {
		_ = s.appendNotificationLocked(req)
	}
	c.replay = nil
	c.session = s
	return nil
}

func (*client) RequestPermission(ctx context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	if ctx.Err() == nil {
		for _, kind := range []acp.PermissionOptionKind{acp.PermissionOptionKindAllowAlways, acp.PermissionOptionKindAllowOnce} {
			for _, option := range req.Options {
				if option.Kind == kind {
					return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{OptionId: option.OptionId}}}, nil
				}
			}
		}
	}
	//nolint:misspell // ACP names this protocol variant Cancelled.
	return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}}}, nil
}

func (c *client) SessionUpdate(ctx context.Context, req acp.SessionNotification) error {
	c.mu.Lock()
	s := c.session
	if s == nil {
		if c.replayID != "" {
			if string(req.SessionId) != c.replayID {
				c.mu.Unlock()
				return xerrors.Errorf("%w: notification session ID mismatch", ErrInvalid)
			}
			c.replay = append(c.replay, req)
		}
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	return s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req})
}

func (*client) ReadTextFile(context.Context, acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, acp.NewMethodNotFound("fs/read_text_file")
}

func (*client) WriteTextFile(context.Context, acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, acp.NewMethodNotFound("fs/write_text_file")
}

func (*client) CreateTerminal(context.Context, acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, acp.NewMethodNotFound("terminal/create")
}

func (*client) KillTerminal(context.Context, acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, acp.NewMethodNotFound("terminal/kill")
}

func (*client) TerminalOutput(context.Context, acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, acp.NewMethodNotFound("terminal/output")
}

func (*client) ReleaseTerminal(context.Context, acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, acp.NewMethodNotFound("terminal/release")
}

func (*client) WaitForTerminalExit(context.Context, acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, acp.NewMethodNotFound("terminal/wait_for_exit")
}

var _ acp.Client = (*client)(nil)
