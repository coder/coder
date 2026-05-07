package chatd

import (
	"context"
	"encoding/json"
	"strings"

	"charm.land/fantasy"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

const minTurnStatusSignalConfidence = 70

type turnStatusLabelSource string

const (
	turnStatusLabelSourceForced    turnStatusLabelSource = "forced_status"
	turnStatusLabelSourceTool      turnStatusLabelSource = "tool_signal"
	turnStatusLabelSourceHeuristic turnStatusLabelSource = "heuristic"
	turnStatusLabelSourceLLM       turnStatusLabelSource = "llm"
	turnStatusLabelSourceFallback  turnStatusLabelSource = "fallback"
)

type turnStatusLabelResult struct {
	Label  string
	Source turnStatusLabelSource
}

// TurnStatusSignal records a high-confidence status label from a turn event.
type TurnStatusSignal struct {
	Label      string
	Category   string
	Success    bool
	Confidence int
}

func (p *Server) deriveTurnStatusLabel(
	ctx context.Context,
	chat database.Chat,
	status database.ChatStatus,
	runResult runChatResult,
	logger slog.Logger,
) turnStatusLabelResult {
	if label := forcedTurnStatusLabel(status); label != "" {
		return turnStatusLabelResult{Label: label, Source: turnStatusLabelSourceForced}
	}

	if signal, ok := selectTurnStatusSignal(runResult.StatusSignals); ok {
		return turnStatusLabelResult{
			Label:  signal.Label,
			Source: turnStatusSignalSource(signal),
		}
	}

	assistantText := strings.TrimSpace(runResult.FinalAssistantText)
	if assistantText == "" || runResult.PushSummaryModel == nil {
		return turnStatusLabelResult{}
	}

	label := generatePushSummary(
		ctx,
		chat,
		status,
		assistantText,
		runResult.FallbackProvider,
		runResult.FallbackModel,
		runResult.PushSummaryModel,
		runResult.ProviderKeys,
		logger,
		p.existingDebugService(),
		runResult.TriggerMessageID,
		runResult.HistoryTipMessageID,
	)
	if label != "" {
		return turnStatusLabelResult{Label: label, Source: turnStatusLabelSourceLLM}
	}
	return turnStatusLabelResult{
		Label:  fallbackPushStatusLabel(status),
		Source: turnStatusLabelSourceFallback,
	}
}

func forcedTurnStatusLabel(status database.ChatStatus) string {
	switch status {
	case database.ChatStatusPending:
		return "Still working on request"
	case database.ChatStatusRequiresAction:
		return "Waiting for user input"
	default:
		return ""
	}
}

func fallbackPushStatusLabel(status database.ChatStatus) string {
	switch status {
	case database.ChatStatusWaiting:
		return "Finished latest turn"
	case database.ChatStatusPending:
		return "Still working on request"
	case database.ChatStatusRequiresAction:
		return "Waiting for user input"
	case database.ChatStatusError:
		return "Hit an error"
	default:
		return "Updated chat status"
	}
}

func selectTurnStatusSignal(signals []TurnStatusSignal) (TurnStatusSignal, bool) {
	var best TurnStatusSignal
	for _, signal := range signals {
		label, ok := normalizePushStatusLabel(signal.Label)
		if !ok || signal.Confidence < minTurnStatusSignalConfidence {
			continue
		}
		signal.Label = label
		if best.Label == "" || signal.Confidence >= best.Confidence {
			best = signal
		}
	}
	return best, best.Label != ""
}

func turnStatusSignalSource(signal TurnStatusSignal) turnStatusLabelSource {
	switch signal.Category {
	case string(turnStatusLabelSourceHeuristic):
		return turnStatusLabelSourceHeuristic
	default:
		return turnStatusLabelSourceTool
	}
}

func turnStatusSignalsFromContent(content []fantasy.Content) []TurnStatusSignal {
	toolCalls := make(map[string]fantasy.ToolCallContent)
	for _, block := range content {
		if tc, ok := contentToolCall(block); ok {
			toolCalls[tc.ToolCallID] = tc
		}
	}

	var signals []TurnStatusSignal
	for _, block := range content {
		tr, ok := contentToolResult(block)
		if !ok || tr.ProviderExecuted {
			continue
		}
		if signal, ok := turnStatusSignalFromToolResult(toolCalls[tr.ToolCallID], tr); ok {
			signals = append(signals, signal)
		}
	}
	return signals
}

func contentToolCall(block fantasy.Content) (fantasy.ToolCallContent, bool) {
	if tc, ok := fantasy.AsContentType[fantasy.ToolCallContent](block); ok {
		return tc, true
	}
	if tc, ok := fantasy.AsContentType[*fantasy.ToolCallContent](block); ok && tc != nil {
		return *tc, true
	}
	return fantasy.ToolCallContent{}, false
}

func contentToolResult(block fantasy.Content) (fantasy.ToolResultContent, bool) {
	if tr, ok := fantasy.AsContentType[fantasy.ToolResultContent](block); ok {
		return tr, true
	}
	if tr, ok := fantasy.AsContentType[*fantasy.ToolResultContent](block); ok && tr != nil {
		return *tr, true
	}
	return fantasy.ToolResultContent{}, false
}

func turnStatusSignalFromToolResult(
	toolCall fantasy.ToolCallContent,
	toolResult fantasy.ToolResultContent,
) (TurnStatusSignal, bool) {
	switch toolResult.ToolName {
	case "execute":
		return executeTurnStatusSignal(toolCall, toolResult)
	case "edit_files", "write_file":
		return fileUpdateTurnStatusSignal(toolResult)
	default:
		return TurnStatusSignal{}, false
	}
}

func executeTurnStatusSignal(
	toolCall fantasy.ToolCallContent,
	toolResult fantasy.ToolResultContent,
) (TurnStatusSignal, bool) {
	if toolCall.ToolCallID == "" {
		return TurnStatusSignal{}, false
	}

	var args chattool.ExecuteArgs
	if err := json.Unmarshal([]byte(toolCall.Input), &args); err != nil {
		return TurnStatusSignal{}, false
	}
	command := strings.TrimSpace(args.Command)
	if command == "" {
		return TurnStatusSignal{}, false
	}

	resultText, isToolError := toolResultText(toolResult)
	if isToolError {
		return TurnStatusSignal{}, false
	}
	var result chattool.ExecuteResult
	if err := json.Unmarshal([]byte(resultText), &result); err != nil {
		return TurnStatusSignal{}, false
	}

	switch {
	case isPRCreateCommand(command) && result.Success:
		return TurnStatusSignal{
			Label:      "Submitted PR",
			Category:   string(turnStatusLabelSourceHeuristic),
			Success:    true,
			Confidence: 100,
		}, true
	case isTestCommand(command):
		if result.Success {
			return TurnStatusSignal{
				Label:      "Finished unit tests",
				Category:   string(turnStatusLabelSourceHeuristic),
				Success:    true,
				Confidence: 100,
			}, true
		}
		return TurnStatusSignal{
			Label:      "Tests failing",
			Category:   string(turnStatusLabelSourceHeuristic),
			Success:    false,
			Confidence: 100,
		}, true
	case isGitCommitCommand(command) && result.Success:
		return TurnStatusSignal{
			Label:      "Created commit",
			Category:   string(turnStatusLabelSourceHeuristic),
			Success:    true,
			Confidence: 90,
		}, true
	default:
		return TurnStatusSignal{}, false
	}
}

func fileUpdateTurnStatusSignal(toolResult fantasy.ToolResultContent) (TurnStatusSignal, bool) {
	resultText, isToolError := toolResultText(toolResult)
	if isToolError {
		return TurnStatusSignal{}, false
	}

	var payload struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(resultText), &payload); err != nil || !payload.OK {
		return TurnStatusSignal{}, false
	}
	return TurnStatusSignal{
		Label:      "Updated files",
		Category:   string(turnStatusLabelSourceTool),
		Success:    true,
		Confidence: 70,
	}, true
}

func toolResultText(toolResult fantasy.ToolResultContent) (string, bool) {
	switch output := toolResult.Result.(type) {
	case fantasy.ToolResultOutputContentText:
		return output.Text, false
	case *fantasy.ToolResultOutputContentText:
		if output == nil {
			return "", false
		}
		return output.Text, false
	case fantasy.ToolResultOutputContentError:
		return "", true
	case *fantasy.ToolResultOutputContentError:
		return "", true
	default:
		return "", false
	}
}

func isPRCreateCommand(command string) bool {
	fields := strings.Fields(command)
	for i := 0; i+2 < len(fields); i++ {
		if fields[i] == "gh" && fields[i+1] == "pr" && fields[i+2] == "create" {
			return true
		}
	}
	return false
}

func isGitCommitCommand(command string) bool {
	fields := strings.Fields(command)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "git" && fields[i+1] == "commit" {
			return true
		}
	}
	return false
}

func isTestCommand(command string) bool {
	command = strings.ToLower(command)
	patterns := []string{
		"go test",
		"npm test",
		"pnpm test",
		"yarn test",
		"pytest",
		"vitest",
		"cargo test",
		"make test",
	}
	for _, pattern := range patterns {
		if strings.Contains(command, pattern) {
			return true
		}
	}
	return false
}
