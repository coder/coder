package agentacp

import (
	"context"
	"encoding/json"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type client struct {
	session    *session
	generation uint64
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

func (c *client) SessionUpdate(_ context.Context, req acp.SessionNotification) error {
	s := c.session
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || c.generation != s.generation || (s.info.ID.SessionID != "" && s.info.ID.SessionID != string(req.SessionId)) {
		return nil
	}
	// The submitted message is recorded locally; live adapter echoes are
	// redundant. Replayed user messages are needed to reconstruct history.
	if s.replay != nil {
		s = s.replay
	} else if req.Update.UserMessageChunk != nil {
		return nil
	}
	raw, err := json.Marshal(req.Update)
	if err != nil {
		return err
	}
	s.appendLocked(workspacesdk.ACPEventKindUpdate, "", raw)
	if chunk := req.Update.AgentMessageChunk; chunk != nil && chunk.Content.Text != nil {
		s.assistant = append(s.assistant, assistantText{seq: s.info.Cursor.Seq, text: chunk.Content.Text.Text})
	}
	if req.Update.UserMessageChunk != nil {
		s.lastUser = s.info.Cursor.Seq
	}
	if update := req.Update.SessionInfoUpdate; update != nil {
		codex, _ := update.Meta["codex"].(map[string]any)
		status, _ := codex["threadStatus"].(map[string]any)
		kind, _ := status["type"].(string)
		switch kind {
		case "active", "idle", "systemError":
			if kind == "active" && s.adapterStatus != "active" {
				s.adapterTurn++
			}
			s.adapterStatus = kind
			if s.detached && s.info.Status != workspacesdk.ACPSessionStatusError {
				s.applyThreadStatusLocked()
			}
		}
	}
	return nil
}

func (s *session) applyThreadStatusLocked() {
	switch s.adapterStatus {
	case "active":
		s.info.Status, s.info.StopReason = workspacesdk.ACPSessionStatusRunning, ""
	case "idle":
		s.info.Status, s.info.StopReason = workspacesdk.ACPSessionStatusIdle, string(acp.StopReasonEndTurn)
		if s.interrupting == s.turn {
			s.info.StopReason = string(acp.StopReasonCancelled)
		}
	case "systemError":
		s.failLocked(ErrUnavailable)
		return
	}
	s.appendLocked(workspacesdk.ACPEventKindStatus, "", nil)
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
