package chatd

import (
	"bytes"
	"encoding/json"
	"strings"

	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type acpTranscriptTool struct {
	call                         acp.SessionUpdateToolCall
	message, part, resultMessage int
}

// acpTranscriptPart identifies a part within the final transcript's messages.
// Text and reasoning append; tool calls and results update by tool_call_id.
type acpTranscriptPart struct {
	MessageIndex int                      `json:"message_index"`
	Role         codersdk.ChatMessageRole `json:"role"`
	Part         codersdk.ChatMessagePart `json:"part"`
}

// acpTranscript reduces replacement-style ACP tool updates within one generation.
type acpTranscript struct {
	messages  []codersdk.ChatACPTranscriptMessage
	tools     map[string]*acpTranscriptTool
	response  strings.Builder
	partText  strings.Builder
	messageID string
}

func newACPTranscript() *acpTranscript {
	return &acpTranscript{
		messages: []codersdk.ChatACPTranscriptMessage{},
		tools:    make(map[string]*acpTranscriptTool),
	}
}

func (t *acpTranscript) appendPart(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart, messageID *string) acpTranscriptPart {
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
		last := &message.Content[len(message.Content)-1]
		if t.partText.Len() == 0 {
			_, _ = t.partText.WriteString(last.Text)
		}
		_, _ = t.partText.WriteString(part.Text)
		// String shares the buffer without copying the accumulated prefix.
		last.Text = t.partText.String()
	} else {
		t.partText.Reset()
		message.Content = append(message.Content, part)
	}
	t.messageID = id
	return acpTranscriptPart{MessageIndex: len(t.messages) - 1, Role: role, Part: part}
}

func acpContentText(update json.RawMessage) (string, bool, error) {
	var raw struct {
		Content *json.RawMessage `json:"content" validate:"required"`
	}
	if err := json.Unmarshal(update, &raw); err != nil {
		return "", false, err
	}
	if err := httpapi.Validate.Struct(raw); err != nil {
		return "", false, err
	}
	var content struct {
		Type string `json:"type" validate:"required"`
		// Decode text only for text blocks; other variants remain opaque.
		Text *json.RawMessage `json:"text" validate:"required_if=Type text"`
	}
	if err := json.Unmarshal(*raw.Content, &content); err != nil {
		return "", false, err
	}
	if err := httpapi.Validate.Struct(content); err != nil {
		return "", false, err
	}
	if content.Type != "text" {
		return string(*raw.Content), false, nil
	}
	var text string
	if err := json.Unmarshal(*content.Text, &text); err != nil {
		return "", false, err
	}
	return text, true, nil
}

// validateACPToolUpdate checks fields that the SDK decodes without validation.
// Unknown fields remain allowed for protocol extensions.
func validateACPToolUpdate(update json.RawMessage) error {
	var fields struct {
		SessionUpdate string              `json:"sessionUpdate" validate:"required,oneof=tool_call tool_call_update"`
		ToolCallID    string              `json:"toolCallId" validate:"required"`
		Title         *string             `json:"title" validate:"required_if=SessionUpdate tool_call"`
		Status        *acp.ToolCallStatus `json:"status" validate:"omitempty,oneof=pending in_progress completed failed"`
		Kind          *acp.ToolKind       `json:"kind" validate:"omitempty,oneof=read edit delete move search execute think fetch switch_mode other"`
	}
	if err := json.Unmarshal(update, &fields); err != nil {
		return err
	}
	return httpapi.Validate.Struct(fields)
}

// consume includes agent messages, agent reasoning, and tool activity in
// the wait transcript and live preview. It omits protocol metadata such as command
// lists, usage, plans, capability changes, and tool metadata, plus unknown updates.
// Session errors are returned separately by acpWaitResponse.
func (t *acpTranscript) consume(event workspacesdk.ACPEvent) ([]acpTranscriptPart, error) {
	switch event.Kind {
	case workspacesdk.ACPEventKindError:
		// Session errors are returned separately by acpWaitResponse.
		return nil, nil
	case workspacesdk.ACPEventKindUpdate:
		var header struct {
			Kind string `json:"sessionUpdate" validate:"required"`
		}
		if err := json.Unmarshal(event.Update, &header); err != nil {
			return nil, xerrors.Errorf("decode ACP update: %w", err)
		}
		if err := httpapi.Validate.Struct(header); err != nil {
			return nil, xerrors.Errorf("invalid ACP update: %w", err)
		}
		switch header.Kind {
		case "agent_message_chunk", "agent_thought_chunk", "tool_call", "tool_call_update":
		default:
			return nil, nil
		}
		var (
			text   string
			isText bool
			err    error
		)
		switch header.Kind {
		case "agent_message_chunk", "agent_thought_chunk":
			text, isText, err = acpContentText(event.Update)
		case "tool_call", "tool_call_update":
			err = validateACPToolUpdate(event.Update)
		}
		if err != nil {
			return nil, xerrors.Errorf("invalid ACP %s: %w", header.Kind, err)
		}
		var update acp.SessionUpdate
		if err := json.Unmarshal(event.Update, &update); err != nil {
			return nil, xerrors.Errorf("decode ACP update: %w", err)
		}
		switch {
		case update.AgentMessageChunk != nil:
			chunk := update.AgentMessageChunk
			if isText {
				_, _ = t.response.WriteString(text)
			}
			return []acpTranscriptPart{t.appendPart(codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText(text), chunk.MessageId)}, nil
		case update.AgentThoughtChunk != nil:
			chunk := update.AgentThoughtChunk
			return []acpTranscriptPart{t.appendPart(codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageReasoning(text), chunk.MessageId)}, nil
		case update.ToolCall != nil:
			return t.updateTool(*update.ToolCall)
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
			return t.updateTool(call)
		default:
			return nil, nil
		}
	}
	return nil, nil
}

func (t *acpTranscript) updateTool(call acp.SessionUpdateToolCall) ([]acpTranscriptPart, error) {
	args := json.RawMessage(`{}`)
	if call.RawInput != nil {
		var err error
		args, err = json.Marshal(call.RawInput)
		if err != nil {
			return nil, xerrors.Errorf("encode ACP tool input: %w", err)
		}
	}
	result, err := json.Marshal(map[string]any{"status": call.Status, "output": call.RawOutput, "content": call.Content, "locations": call.Locations, "kind": call.Kind})
	if err != nil {
		return nil, xerrors.Errorf("encode ACP tool result: %w", err)
	}
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
	name := call.Title
	if name == "" {
		name = id
	}
	callPart := codersdk.ChatMessageToolCall(id, name, args)
	previousCall := t.messages[tool.message].Content[tool.part]
	t.messages[tool.message].Content[tool.part] = callPart
	resultPart := codersdk.ChatMessageToolResult(id, name, result, call.Status == acp.ToolCallStatusFailed, false)
	previousResult := t.messages[tool.resultMessage].Content[0]
	t.messages[tool.resultMessage].Content[0] = resultPart
	t.messageID = ""
	var parts []acpTranscriptPart
	if previousCall.ToolName != name || !bytes.Equal(previousCall.Args, args) {
		parts = append(parts, acpTranscriptPart{MessageIndex: tool.message, Role: codersdk.ChatMessageRoleAssistant, Part: callPart})
	}
	// A populated result completes a tool in the chat renderer. Keep running
	// tools as calls until ACP reports a terminal status.
	if (call.Status == acp.ToolCallStatusCompleted || call.Status == acp.ToolCallStatusFailed) &&
		(previousResult.ToolName != name || !bytes.Equal(previousResult.Result, result)) {
		parts = append(parts, acpTranscriptPart{MessageIndex: tool.resultMessage, Role: codersdk.ChatMessageRoleTool, Part: resultPart})
	}
	return parts, nil
}
