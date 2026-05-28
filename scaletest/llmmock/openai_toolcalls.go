package llmmock

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

const (
	// coderChatIDHeader mirrors chatprovider.HeaderCoderChatID without adding a
	// production chatd dependency to this scaletest mock.
	coderChatIDHeader      = "X-Coder-Chat-Id"
	executeToolName        = "execute"
	defaultToolCallCommand = "echo scaletest"
	// maxToolCallsPerTurnLimit is 1200 - 1, leaving room for the final
	// assistant text under chatd's maxChatSteps.
	// See coderd/x/chatd/chatd.go maxChatSteps.
	maxToolCallsPerTurnLimit   = 1199
	openAIToolCallFinishReason = "tool_calls"
	openAIStopFinishReason     = "stop"
)

type toolCallConfig struct {
	MinToolCallsPerTurn int
	MaxToolCallsPerTurn int
	ToolCallCommand     string
	Seed                uint64
}

func (c Config) Validate() error {
	if c.MinStreamDuration < 0 {
		return xerrors.Errorf("validate min_stream_duration: must not be negative: %s", c.MinStreamDuration)
	}
	if c.MaxStreamDuration < 0 {
		return xerrors.Errorf("validate max_stream_duration: must not be negative: %s", c.MaxStreamDuration)
	}
	if c.MinStreamDuration == 0 && c.MaxStreamDuration != 0 {
		return xerrors.New("validate min_stream_duration: must be set when max_stream_duration is set")
	}
	if c.MaxStreamDuration == 0 && c.MinStreamDuration != 0 {
		return xerrors.New("validate max_stream_duration: must be set when min_stream_duration is set")
	}
	if c.MinStreamDuration > c.MaxStreamDuration {
		return xerrors.Errorf("validate min_stream_duration: must be <= max_stream_duration: %s > %s", c.MinStreamDuration, c.MaxStreamDuration)
	}
	if c.MinToolCallsPerTurn < 0 {
		return xerrors.Errorf("validate min_tool_calls_per_turn: must not be negative: %d", c.MinToolCallsPerTurn)
	}
	if c.MaxToolCallsPerTurn < 0 {
		return xerrors.Errorf("validate max_tool_calls_per_turn: must not be negative: %d", c.MaxToolCallsPerTurn)
	}
	if c.MinToolCallsPerTurn > c.MaxToolCallsPerTurn {
		return xerrors.Errorf("validate min_tool_calls_per_turn: must be <= max_tool_calls_per_turn: %d > %d", c.MinToolCallsPerTurn, c.MaxToolCallsPerTurn)
	}
	if c.MaxToolCallsPerTurn > maxToolCallsPerTurnLimit {
		return xerrors.Errorf("validate max_tool_calls_per_turn: must be <= %d", maxToolCallsPerTurnLimit)
	}
	return nil
}

func buildOpenAIResponse(req llmRequest, requestID uuid.UUID, now time.Time, responsePayloadSize int, chatID string, cfg toolCallConfig) (openAIResponse, error) {
	message := openAIMessage{Role: "assistant"}
	finishReason := openAIStopFinishReason

	turnIndex, completedToolCalls := currentTurnState(req)
	if turnIndex >= 0 && cfg.MaxToolCallsPerTurn > 0 {
		target := targetForTurn(cfg.Seed, chatID, turnIndex, cfg.MinToolCallsPerTurn, cfg.MaxToolCallsPerTurn)
		if completedToolCalls < target {
			if !slices.ContainsFunc(req.Tools, func(tool openAITool) bool {
				return tool.Function.Name == executeToolName
			}) {
				return openAIResponse{}, xerrors.Errorf("requested tool %q not present in tools list", executeToolName)
			}

			payload, err := json.Marshal(map[string]string{"command": cfg.ToolCallCommand})
			if err != nil {
				return openAIResponse{}, xerrors.Errorf("marshal %s arguments: %w", executeToolName, err)
			}

			message.ToolCalls = []openAIToolCall{{
				Index: 0,
				ID:    fmt.Sprintf("call_%s", uuid.New().String()[:8]),
				Type:  "function",
				Function: openAIToolCallFunction{
					Name:      executeToolName,
					Arguments: string(payload),
				},
			}}
			finishReason = openAIToolCallFinishReason
		}
	}
	if len(message.ToolCalls) == 0 {
		message.Content = responseText(responsePayloadSize, openAIDefaultResponseText)
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
	resp.Usage.PromptTokens = mockInputTokens
	resp.Usage.CompletionTokens = mockOutputTokens
	resp.Usage.TotalTokens = mockInputTokens + mockOutputTokens
	return resp, nil
}

// currentTurnState returns the zero-based index of the most recent user turn
// and the tool responses observed since that turn. It assumes the request
// contains the full conversation history.
func currentTurnState(req llmRequest) (turnIndex int, completedToolCalls int) {
	lastUserIndex := -1
	userCount := 0
	for i, message := range req.Messages {
		if message.Role == "user" {
			lastUserIndex = i
			userCount++
		}
	}
	if lastUserIndex < 0 {
		return -1, 0
	}

	for _, message := range req.Messages[lastUserIndex+1:] {
		if message.Role == "tool" {
			completedToolCalls++
		}
	}

	return userCount - 1, completedToolCalls
}

func targetForTurn(seed uint64, chatID string, turnIndex int, minToolCalls, maxToolCalls int) int {
	if maxToolCalls <= minToolCalls {
		return minToolCalls
	}

	rng := rand.New(rand.NewSource(seedForTurn(seed, chatID, turnIndex)))
	return minToolCalls + rng.Intn(maxToolCalls-minToolCalls+1)
}

func seedForTurn(seed uint64, chatID string, turnIndex int) int64 {
	h := fnv.New64a()

	var seedBytes [8]byte
	binary.LittleEndian.PutUint64(seedBytes[:], seed)
	_, _ = h.Write(seedBytes[:])
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(chatID))
	_, _ = h.Write([]byte{0})

	var turnBytes [8]byte
	binary.LittleEndian.PutUint64(turnBytes[:], uint64(turnIndex))
	_, _ = h.Write(turnBytes[:])

	// #nosec G115 - Safe conversion to generate int64 hash from Sum64, data loss acceptable.
	return int64(h.Sum64())
}
