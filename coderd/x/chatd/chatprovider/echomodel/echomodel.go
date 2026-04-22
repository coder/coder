// Package echomodel provides a test language model that can call
// tools to create workspaces and respond with canned text. It
// implements the fantasy.LanguageModel interface and is used in
// development mode to test the agents UI without requiring
// external AI provider keys.
package echomodel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/fantasy"
)

const (
	ProviderName = "echo"
	ModelName    = "echo-1"
)

// Model is a test language model that can call tools to create
// workspaces and echoes back canned responses for other messages.
type Model struct{}

// New creates a new echo model.
func New() *Model {
	return &Model{}
}

func (*Model) Provider() string { return ProviderName }
func (*Model) Model() string    { return ModelName }

// Stream returns a streaming response. On the first call with tools
// available, it calls list_templates then create_workspace to set up
// a test workspace. Subsequent messages get text echo responses.
func (*Model) Stream(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	state := analyzeConversation(call.Prompt)
	hasTools := len(call.Tools) > 0

	// Drive workspace creation flow when tools are available.
	if hasTools {
		switch {
		case !state.calledListTemplates:
			return emitToolCall("list_templates", `{}`), nil

		case state.calledListTemplates && !state.calledCreateWorkspace:
			templateID := parseFirstTemplateID(state.listTemplatesOutput)
			if templateID != "" {
				input, _ := json.Marshal(map[string]string{
					"template_id": templateID,
					"name":        "echo-test",
				})
				return emitToolCall("create_workspace", string(input)), nil
			}

		case state.calledCreateWorkspace && !state.calledStartWorkspace:
			return emitToolCall("start_workspace", `{}`), nil
		}
	}

	// Default: respond with text.
	lastUserMsg := extractLastUserMessage(call.Prompt)
	response := buildResponse(lastUserMsg, state.calledStartWorkspace)
	return emitText(response), nil
}

// Generate returns a non-streaming response.
func (*Model) Generate(_ context.Context, call fantasy.Call) (*fantasy.Response, error) {
	lastUserMsg := extractLastUserMessage(call.Prompt)
	state := analyzeConversation(call.Prompt)
	response := buildResponse(lastUserMsg, state.calledStartWorkspace)
	return &fantasy.Response{
		Content:      fantasy.ResponseContent{fantasy.TextContent{Text: response}},
		FinishReason: fantasy.FinishReasonStop,
		Usage:        fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
	}, nil
}

// GenerateObject is not supported.
func (*Model) GenerateObject(_ context.Context, _ fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return &fantasy.ObjectResponse{
		Object:       "{}",
		FinishReason: fantasy.FinishReasonStop,
	}, nil
}

// StreamObject is not supported.
func (*Model) StreamObject(_ context.Context, _ fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return func(yield func(fantasy.ObjectStreamPart) bool) {
		yield(fantasy.ObjectStreamPart{
			Type:         fantasy.ObjectStreamPartTypeFinish,
			FinishReason: fantasy.FinishReasonStop,
		})
	}, nil
}

// ─── Conversation analysis ────────────────────────────────────

type conversationInfo struct {
	calledListTemplates  bool
	calledCreateWorkspace bool
	calledStartWorkspace  bool
	listTemplatesOutput  string
}

// analyzeConversation examines the message history to determine
// which tools have been called and their results.
func analyzeConversation(prompt fantasy.Prompt) conversationInfo {
	var info conversationInfo

	// First pass: collect tool call IDs → tool names from
	// assistant messages.
	toolNames := make(map[string]string) // callID → toolName
	for _, msg := range prompt {
		if msg.Role != fantasy.MessageRoleAssistant {
			continue
		}
		for _, part := range msg.Content {
			tc, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part)
			if !ok {
				continue
			}
			toolNames[tc.ToolCallID] = tc.ToolName
		}
	}

	// Second pass: match tool results to tool names.
	for _, msg := range prompt {
		if msg.Role != fantasy.MessageRoleTool {
			continue
		}
		for _, part := range msg.Content {
			tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
			if !ok {
				continue
			}
			name := toolNames[tr.ToolCallID]
			switch name {
			case "list_templates":
				info.calledListTemplates = true
				info.listTemplatesOutput = toolResultText(tr)
			case "create_workspace":
				info.calledCreateWorkspace = true
			case "start_workspace":
				info.calledStartWorkspace = true
			}
		}
	}

	return info
}

// toolResultText extracts text from a tool result's output.
func toolResultText(tr fantasy.ToolResultPart) string {
	if tr.Output == nil {
		return ""
	}
	if text, ok := tr.Output.(fantasy.ToolResultOutputContentText); ok {
		return text.Text
	}
	return ""
}

// ─── Extract data from conversations ──────────────────────────

func extractLastUserMessage(prompt fantasy.Prompt) string {
	var lastMsg string
	for _, msg := range prompt {
		if msg.Role == fantasy.MessageRoleUser {
			for _, part := range msg.Content {
				if tp, ok := fantasy.AsMessagePart[fantasy.TextPart](part); ok {
					lastMsg = tp.Text
				}
			}
		}
	}
	return lastMsg
}

func parseFirstTemplateID(jsonText string) string {
	var result struct {
		Templates []struct {
			ID string `json:"id"`
		} `json:"templates"`
	}
	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		return ""
	}
	if len(result.Templates) > 0 {
		return result.Templates[0].ID
	}
	return ""
}

// ─── Stream helpers ───────────────────────────────────────────

var toolCallCounter int

func emitToolCall(name, input string) fantasy.StreamResponse {
	toolCallCounter++
	callID := fmt.Sprintf("echo_call_%d", toolCallCounter)

	return func(yield func(fantasy.StreamPart) bool) {
		if !yield(fantasy.StreamPart{
			Type:          fantasy.StreamPartTypeToolCall,
			ID:            callID,
			ToolCallName:  name,
			ToolCallInput: input,
		}) {
			return
		}
		yield(fantasy.StreamPart{
			Type:         fantasy.StreamPartTypeFinish,
			FinishReason: fantasy.FinishReasonToolCalls,
			Usage:        fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
		})
	}
}

func emitText(text string) fantasy.StreamResponse {
	return func(yield func(fantasy.StreamPart) bool) {
		if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart}) {
			return
		}
		for _, chunk := range splitIntoChunks(text, 40) {
			if !yield(fantasy.StreamPart{
				Type:  fantasy.StreamPartTypeTextDelta,
				Delta: chunk,
			}) {
				return
			}
		}
		if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd}) {
			return
		}
		yield(fantasy.StreamPart{
			Type:         fantasy.StreamPartTypeFinish,
			FinishReason: fantasy.FinishReasonStop,
			Usage:        fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
		})
	}
}

// ─── Response builders ────────────────────────────────────────

func buildResponse(userMsg string, workspaceReady bool) string {
	if workspaceReady {
		return "Your test workspace is ready! " +
			"Check the **right panel** — you should see the " +
			"**Demo Plugin** tab alongside Git and Terminal.\n\n" +
			"Click on it to activate the plugin iframe and " +
			"test the postMessage SDK."
	}
	if userMsg == "" {
		return "Hello! I'm the **Echo test provider**. " +
			"I'll set up a test workspace so you can try " +
			"the plugin system. Just send me a message to start!"
	}
	return fmt.Sprintf(
		"I received your message:\n\n> %s\n\n"+
			"I'm the **Echo test provider** — a built-in mock "+
			"for development testing.",
		truncate(userMsg, 200),
	)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func splitIntoChunks(s string, chunkSize int) []string {
	var chunks []string
	words := strings.Fields(s)
	var current strings.Builder
	for _, word := range words {
		if current.Len() > 0 && current.Len()+1+len(word) > chunkSize {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			_, _ = current.WriteRune(' ')
		}
		_, _ = current.WriteString(word)
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}
