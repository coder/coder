package llmmock

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

const (
	executeToolName        = "execute"
	defaultToolCallCommand = "echo scaletest"
)

func (s *Server) buildOpenAIChoice(req llmRequest) (openAIResponseChoice, error) {
	if !s.needsOpenAIToolCall(req) {
		return openAIResponseChoice{
			Message: openAIMessage{
				Role:    "assistant",
				Content: s.responseText(openAIDefaultResponseText),
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

	completedToolCalls := 0
	for i := len(req.Messages) - 1; i >= 0; i-- {
		switch req.Messages[i].Role {
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
