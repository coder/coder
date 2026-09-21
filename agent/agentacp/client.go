package agentacp

import (
	"context"
	"strings"

	"github.com/google/uuid"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/codersdk"
)

func (s *session) RequestPermission(ctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	s.mu.Lock()
	canceled := s.data.Status == "interrupting" || s.dead
	s.mu.Unlock()
	if ctx.Err() == nil && !canceled {
		for _, option := range p.Options {
			if option.Kind == "allow_once" || option.Kind == "allow_always" {
				return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeSelected(option.OptionId)}, nil
			}
		}
	}
	return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
}

func (s *session) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := n.Update
	switch {
	case u.AgentMessageChunk != nil:
		if u.AgentMessageChunk.Content.Text != nil {
			s.textLocked("text", u.AgentMessageChunk.Content.Text.Text)
		}
	case u.AgentThoughtChunk != nil:
		if u.AgentThoughtChunk.Content.Text != nil {
			s.textLocked("reasoning", u.AgentThoughtChunk.Content.Text.Text)
		}
	case u.ToolCall != nil:
		t := u.ToolCall

		s.data.Entries = append(s.data.Entries, codersdk.ACPEntry{ID: string(t.ToolCallId), Role: "assistant", Kind: "tool", Title: t.Title, Status: string(t.Status), Input: t.RawInput, Output: t.RawOutput, Text: toolContent(t.Content)})
	case u.ToolCallUpdate != nil:
		t := u.ToolCallUpdate
		for i := len(s.data.Entries) - 1; i >= 0; i-- {
			e := &s.data.Entries[i]
			if e.ID != string(t.ToolCallId) {
				continue
			}
			if t.Title != nil {
				e.Title = *t.Title
			}
			if t.Status != nil {
				e.Status = string(*t.Status)
			}
			if t.RawInput != nil {
				e.Input = t.RawInput
			}
			if t.RawOutput != nil {
				e.Output = t.RawOutput
			}
			if t.Content != nil {
				e.Text = toolContent(t.Content)
			}
			break
		}
	case u.Plan != nil:
		var lines []string
		for _, e := range u.Plan.Entries {
			lines = append(lines, "- ["+string(e.Status)+"] "+e.Content)
		}
		s.data.Entries = append(s.data.Entries, codersdk.ACPEntry{ID: uuid.NewString(), Role: "assistant", Kind: "plan", Text: strings.Join(lines, "\n")})
	default:
		return nil
	}
	s.notifyLocked()
	return nil
}
func (s *session) textLocked(kind, text string) {
	if len(s.data.Entries) > 0 {
		e := &s.data.Entries[len(s.data.Entries)-1]
		if e.Role == "assistant" && e.Kind == kind {
			e.Text += text
			return
		}
	}
	s.data.Entries = append(s.data.Entries, codersdk.ACPEntry{ID: uuid.NewString(), Role: "assistant", Kind: kind, Text: text})
}
func toolContent(content []acp.ToolCallContent) string {
	var parts []string
	for _, c := range content {
		if c.Content != nil && c.Content.Content.Text != nil {
			parts = append(parts, c.Content.Content.Text.Text)
		}
		if c.Diff != nil {
			parts = append(parts, "```diff\n"+c.Diff.Path+"\n"+c.Diff.NewText+"\n```")
		}
	}
	return strings.Join(parts, "\n")
}

func (*session) ReadTextFile(context.Context, acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, acp.NewMethodNotFound("ReadTextFile")
}

func (*session) WriteTextFile(context.Context, acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, acp.NewMethodNotFound("WriteTextFile")
}

func (*session) CreateTerminal(context.Context, acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, acp.NewMethodNotFound("CreateTerminal")
}

func (*session) KillTerminal(context.Context, acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, acp.NewMethodNotFound("KillTerminal")
}

func (*session) TerminalOutput(context.Context, acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, acp.NewMethodNotFound("TerminalOutput")
}

func (*session) ReleaseTerminal(context.Context, acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, acp.NewMethodNotFound("ReleaseTerminal")
}

func (*session) WaitForTerminalExit(context.Context, acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, acp.NewMethodNotFound("WaitForTerminalExit")
}
