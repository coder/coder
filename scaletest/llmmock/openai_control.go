package llmmock

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/scaletest/chatcontrol"
)

func buildOpenAIResponse(req llmRequest, requestID uuid.UUID, now time.Time, responsePayloadSize int) (openAIResponse, error) {
	message := openAIMessage{
		Role:    "assistant",
		Content: openAIResponseText(responsePayloadSize),
	}
	finishReason := "stop"

	control, completedToolCalls, found, err := currentTurnControl(req)
	if err != nil {
		return openAIResponse{}, xerrors.Errorf("resolve current turn control: %w", err)
	}
	if found && completedToolCalls < control.ToolCallsThisTurn {
		if err := validateRequestedTool(req.Tools, control.Tool); err != nil {
			return openAIResponse{}, err
		}
		arguments, err := buildToolArguments(control)
		if err != nil {
			return openAIResponse{}, err
		}
		message.Content = ""
		message.ToolCalls = []openAIToolCall{newOpenAIToolCall(control.Tool, arguments)}
		finishReason = "tool_calls"
	}

	resp := openAIResponse{
		ID:      fmt.Sprintf("chatcmpl-%s", requestID.String()[:8]),
		Object:  "chat.completion",
		Created: now.Unix(),
		Model:   req.Model,
		Choices: []openAIResponseChoice{{
			Index:        0,
			Message:      message,
			FinishReason: finishReason,
		}},
	}
	resp.Usage.PromptTokens = 10
	resp.Usage.CompletionTokens = 5
	resp.Usage.TotalTokens = 15
	return resp, nil
}

func currentTurnControl(req llmRequest) (chatcontrol.Control, int, bool, error) {
	lastUserIndex := -1
	for i, message := range req.Messages {
		if message.Role == "user" {
			lastUserIndex = i
		}
	}
	if lastUserIndex < 0 {
		return chatcontrol.Control{}, 0, false, nil
	}

	control, _, found, err := chatcontrol.ParsePrompt(req.Messages[lastUserIndex].Content)
	if err != nil {
		return chatcontrol.Control{}, 0, false, xerrors.Errorf("parse latest user prompt: %w", err)
	}
	if !found {
		return chatcontrol.Control{}, 0, false, nil
	}

	completedToolCalls := 0
	for _, message := range req.Messages[lastUserIndex+1:] {
		if message.Role == "tool" {
			completedToolCalls++
		}
	}

	return control, completedToolCalls, true, nil
}

func validateRequestedTool(tools []openAITool, toolName string) error {
	for _, tool := range tools {
		if tool.Function.Name == toolName {
			return nil
		}
	}
	return xerrors.Errorf("requested tool %q not present in tools list", toolName)
}

func buildToolArguments(control chatcontrol.Control) (string, error) {
	switch control.Tool {
	case chatcontrol.DefaultToolName:
		payload, err := json.Marshal(map[string]string{"command": control.Command})
		if err != nil {
			return "", xerrors.Errorf("marshal execute arguments: %w", err)
		}
		return string(payload), nil
	default:
		return "", xerrors.Errorf("unsupported tool %q", control.Tool)
	}
}

func newOpenAIToolCall(toolName string, arguments string) openAIToolCall {
	return openAIToolCall{
		Index: 0,
		ID:    fmt.Sprintf("call_%s", uuid.New().String()[:8]),
		Type:  "function",
		Function: openAIToolCallFunction{
			Name:      toolName,
			Arguments: arguments,
		},
	}
}

func openAIResponseText(responsePayloadSize int) string {
	if responsePayloadSize > 0 {
		pattern := "x"
		repeated := strings.Repeat(pattern, responsePayloadSize)
		return repeated[:responsePayloadSize]
	}
	return "This is a mock response from OpenAI."
}
