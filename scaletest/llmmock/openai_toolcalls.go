package llmmock

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
)

const executeToolName = "execute"

func (s *Server) buildOpenAIChoice(req llmRequest) openAIResponseChoice {
	if !s.needsOpenAIToolCall(req) || !slices.ContainsFunc(req.Tools, func(tool openAITool) bool {
		return tool.Function.Name == executeToolName
	}) {
		return openAIResponseChoice{
			Message: openAIMessage{
				Role:    "assistant",
				Content: s.responseText(openAIDefaultResponseText),
			},
			FinishReason: openAIStopFinishReason,
		}
	}

	return openAIResponseChoice{
		Message: openAIMessage{
			Role:      "assistant",
			ToolCalls: []openAIToolCall{executeToolCall(s.toolCallCommand)},
		},
		FinishReason: openAIToolCallFinishReason,
	}
}

func (s *Server) needsOpenAIToolCall(req llmRequest) bool {
	if s.toolCallsPerTurn <= 0 {
		return false
	}

	completedToolCalls := 0
	for _, msg := range slices.Backward(req.Messages) {
		switch msg.Role {
		case "tool":
			completedToolCalls++
		case "user":
			return completedToolCalls < s.toolCallsPerTurn
		}
	}
	return false
}

func executeToolCall(command string) openAIToolCall {
	payload, _ := json.Marshal(map[string]string{"command": command})
	return openAIToolCall{
		ID:   fmt.Sprintf("call_%s", uuid.New().String()[:8]),
		Type: "function",
		Function: openAIToolCallFunction{
			Name:      executeToolName,
			Arguments: string(payload),
		},
	}
}
