package chatd

import (
	"encoding/json"
	"strings"

	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type acpTranscriptTool struct {
	call                         acp.SessionUpdateToolCall
	message, part, resultMessage int
}

// acpTranscript reduces replacement-style ACP tool updates while keeping
// only the conversation beginning with the most recent user message.
type acpTranscript struct {
	messages  []codersdk.ChatACPTranscriptMessage
	tools     map[string]*acpTranscriptTool
	response  strings.Builder
	messageID string
}

func (t *acpTranscript) reset() {
	t.messages = []codersdk.ChatACPTranscriptMessage{}
	t.tools = make(map[string]*acpTranscriptTool)
	t.response.Reset()
	t.messageID = ""
}

func (t *acpTranscript) appendPart(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart, messageID *string) bool {
	id := ""
	if messageID != nil {
		id = *messageID
	}
	newMessage := len(t.messages) == 0 || t.messages[len(t.messages)-1].Role != role || (id != "" && id != t.messageID)
	if newMessage {
		t.messages = append(t.messages, codersdk.ChatACPTranscriptMessage{Role: role, Content: []codersdk.ChatMessagePart{}})
	}
	message := &t.messages[len(t.messages)-1]
	if len(message.Content) > 0 && (part.Type == codersdk.ChatMessagePartTypeText || part.Type == codersdk.ChatMessagePartTypeReasoning) && message.Content[len(message.Content)-1].Type == part.Type {
		message.Content[len(message.Content)-1].Text += part.Text
	} else {
		message.Content = append(message.Content, part)
	}
	t.messageID = id
	return true
}

func acpContentText(content acp.ContentBlock, update json.RawMessage) (string, bool) {
	var raw struct {
		Content json.RawMessage `json:"content"`
	}
	_ = json.Unmarshal(update, &raw)
	var header struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(raw.Content, &header)
	if header.Type == "text" && content.Text != nil {
		return content.Text.Text, true
	}
	return string(raw.Content), false
}

// consume includes user and agent messages, agent reasoning, and tool activity in
// the wait transcript and live preview. It omits protocol metadata such as command
// lists, usage, plans, capability changes, and tool metadata, plus unknown updates.
// Session errors are returned separately by acpWaitResponse.
func (t *acpTranscript) consume(event workspacesdk.ACPEvent) (changed bool, reset bool, err error) {
	if t.tools == nil {
		t.reset()
	}
	switch event.Kind {
	case workspacesdk.ACPEventKindReset:
		t.reset()
		return true, true, nil
	case workspacesdk.ACPEventKindUserMessage:
		t.reset()
		return t.appendPart(codersdk.ChatMessageRoleUser, codersdk.ChatMessageText(event.Text), nil), true, nil
	case workspacesdk.ACPEventKindError:
		// Session errors are returned separately by acpWaitResponse.
		return false, false, nil
	case workspacesdk.ACPEventKindUpdate:
		var header struct {
			Kind string `json:"sessionUpdate"`
		}
		if err := json.Unmarshal(event.Update, &header); err != nil {
			return false, false, xerrors.Errorf("decode ACP update: %w", err)
		}
		switch header.Kind {
		case "user_message_chunk", "agent_message_chunk", "agent_thought_chunk", "tool_call", "tool_call_update":
		default:
			return false, false, nil
		}
		var update acp.SessionUpdate
		if err := json.Unmarshal(event.Update, &update); err != nil {
			return false, false, xerrors.Errorf("decode ACP update: %w", err)
		}
		switch {
		case update.UserMessageChunk != nil:
			chunk := update.UserMessageChunk
			if len(t.messages) == 0 || t.messages[len(t.messages)-1].Role != codersdk.ChatMessageRoleUser || (chunk.MessageId != nil && *chunk.MessageId != t.messageID) {
				t.reset()
				reset = true
			}
			text, _ := acpContentText(chunk.Content, event.Update)
			return t.appendPart(codersdk.ChatMessageRoleUser, codersdk.ChatMessageText(text), chunk.MessageId), reset, nil
		case update.AgentMessageChunk != nil:
			chunk := update.AgentMessageChunk
			text, isText := acpContentText(chunk.Content, event.Update)
			if isText {
				_, _ = t.response.WriteString(text)
			}
			return t.appendPart(codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText(text), chunk.MessageId), false, nil
		case update.AgentThoughtChunk != nil:
			chunk := update.AgentThoughtChunk
			text, _ := acpContentText(chunk.Content, event.Update)
			return t.appendPart(codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeReasoning, Text: text}, chunk.MessageId), false, nil
		case update.ToolCall != nil:
			return t.updateTool(*update.ToolCall), false, nil
		case update.ToolCallUpdate != nil:
			u := update.ToolCallUpdate
			call := acp.SessionUpdateToolCall{ToolCallId: u.ToolCallId, Title: string(u.ToolCallId)}
			if existing := t.tools[string(u.ToolCallId)]; existing != nil {
				call = existing.call
			}
			if u.Title != nil {
				call.Title = *u.Title
			}
			if u.Status != nil {
				call.Status = *u.Status
			}
			if u.Kind != nil {
				call.Kind = *u.Kind
			}
			if u.Content != nil {
				call.Content = u.Content
			}
			if u.Locations != nil {
				call.Locations = u.Locations
			}
			if u.RawInput != nil {
				call.RawInput = u.RawInput
			}
			if u.RawOutput != nil {
				call.RawOutput = u.RawOutput
			}
			return t.updateTool(call), false, nil
		default:
			return false, false, nil
		}
	}
	return false, false, nil
}

func (t *acpTranscript) updateTool(call acp.SessionUpdateToolCall) bool {
	id := string(call.ToolCallId)
	tool := t.tools[id]
	if tool == nil {
		t.appendPart(codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: id}, nil)
		tool = &acpTranscriptTool{message: len(t.messages) - 1, part: len(t.messages[len(t.messages)-1].Content) - 1}
		t.messages = append(t.messages, codersdk.ChatACPTranscriptMessage{Role: codersdk.ChatMessageRoleTool, Content: []codersdk.ChatMessagePart{{Type: codersdk.ChatMessagePartTypeToolResult, ToolCallID: id}}})
		tool.resultMessage = len(t.messages) - 1
		t.tools[id] = tool
	}
	tool.call = call
	args := json.RawMessage(`{}`)
	if call.RawInput != nil {
		args, _ = json.Marshal(call.RawInput)
	}
	name := call.Title
	if name == "" {
		name = id
	}
	t.messages[tool.message].Content[tool.part] = codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: id, ToolName: name, Args: args}
	result, _ := json.Marshal(map[string]any{"status": call.Status, "output": call.RawOutput, "content": call.Content, "locations": call.Locations, "kind": call.Kind})
	t.messages[tool.resultMessage].Content[0] = codersdk.ChatMessageToolResult(id, name, result, call.Status == acp.ToolCallStatusFailed, false)
	t.messageID = ""
	return true
}
