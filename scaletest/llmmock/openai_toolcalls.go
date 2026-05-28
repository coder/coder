package llmmock

import (
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

const (
	executeToolName = "execute"
	// DefaultToolCallCommand is the command emitted when no override is configured.
	DefaultToolCallCommand     = "echo scaletest"
	openAIToolCallFinishReason = "tool_calls"
	openAIStopFinishReason     = "stop"
)

func (s *Server) buildOpenAIChoice(req llmRequest) (openAIResponseChoice, error) {
	if !s.needsOpenAIToolCall(req) {
		return openAIResponseChoice{
			Index: 0,
			Message: openAIMessage{
				Role:    "assistant",
				Content: responseText(s.responsePayloadSize, openAIDefaultResponseText),
			},
			FinishReason: openAIStopFinishReason,
		}, nil
	}

	if !slices.ContainsFunc(req.Tools, func(tool openAITool) bool {
		return tool.Function.Name == executeToolName
	}) {
		return openAIResponseChoice{}, xerrors.Errorf("requested tool %q not present in tools list", executeToolName)
	}

	return openAIResponseChoice{
		Index: 0,
		Message: openAIMessage{
			Role:      "assistant",
			ToolCalls: []openAIToolCall{executeToolCall(s.toolCallCommand)},
		},
		FinishReason: openAIToolCallFinishReason,
	}, nil
}

func (s *Server) needsOpenAIToolCall(req llmRequest) bool {
	if s.toolCallsPerTurn <= 0 {
		return false
	}

	lastUserIndex := -1
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			lastUserIndex = i
			break
		}
	}
	if lastUserIndex < 0 {
		return false
	}

	completedToolCalls := 0
	for _, message := range req.Messages[lastUserIndex+1:] {
		if message.Role == "tool" {
			completedToolCalls++
		}
	}

	return completedToolCalls < s.toolCallsPerTurn
}

func executeToolCall(command string) openAIToolCall {
	payload, _ := json.Marshal(map[string]string{"command": command})
	return openAIToolCall{
		ID:   "call_" + uuid.New().String()[:8],
		Type: "function",
		Function: openAIToolCallFunction{
			Name:      executeToolName,
			Arguments: string(payload),
		},
	}
}
