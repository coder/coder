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

type turnStatusSignal struct {
	Label      string
	Source     turnStatusLabelSource
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
			Source: signal.Source,
		}
	}

	assistantText := strings.TrimSpace(runResult.FinalAssistantText)
	if assistantText == "" || runResult.StatusLabelModel == nil {
		return turnStatusLabelResult{
			Label:  fallbackTurnStatusLabel(status),
			Source: turnStatusLabelSourceFallback,
		}
	}

	label := generateTurnStatusLabel(
		ctx,
		chat,
		status,
		assistantText,
		runResult.FallbackProvider,
		runResult.FallbackModel,
		runResult.StatusLabelModel,
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
		Label:  fallbackTurnStatusLabel(status),
		Source: turnStatusLabelSourceFallback,
	}
}

func forcedTurnStatusLabel(status database.ChatStatus) string {
	switch status {
	case database.ChatStatusPending, database.ChatStatusRequiresAction:
		return fallbackTurnStatusLabel(status)
	default:
		return ""
	}
}

func fallbackTurnStatusLabel(status database.ChatStatus) string {
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

func selectTurnStatusSignal(signals []turnStatusSignal) (turnStatusSignal, bool) {
	var best turnStatusSignal
	for _, signal := range signals {
		label, ok := normalizeTurnStatusLabel(signal.Label)
		if !ok || signal.Confidence < minTurnStatusSignalConfidence {
			continue
		}
		signal.Label = label
		if signal.Source == "" {
			signal.Source = turnStatusLabelSourceTool
		}
		// Later signals win confidence ties because they better represent the
		// current turn state after earlier actions have completed.
		if best.Label == "" || signal.Confidence >= best.Confidence {
			best = signal
		}
	}
	return best, best.Label != ""
}

func turnStatusSignalsFromContent(content []fantasy.Content) []turnStatusSignal {
	toolCalls := make(map[string]fantasy.ToolCallContent)
	for _, block := range content {
		if tc, ok := contentToolCall(block); ok {
			toolCalls[tc.ToolCallID] = tc
		}
	}

	var signals []turnStatusSignal
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
) (turnStatusSignal, bool) {
	switch toolResult.ToolName {
	case "execute":
		return executeTurnStatusSignal(toolCall, toolResult)
	case "edit_files", "write_file":
		return fileUpdateTurnStatusSignal(toolResult)
	default:
		return turnStatusSignal{}, false
	}
}

func executeTurnStatusSignal(
	toolCall fantasy.ToolCallContent,
	toolResult fantasy.ToolResultContent,
) (turnStatusSignal, bool) {
	if toolCall.ToolCallID == "" {
		return turnStatusSignal{}, false
	}

	var args chattool.ExecuteArgs
	if err := json.Unmarshal([]byte(toolCall.Input), &args); err != nil {
		return turnStatusSignal{}, false
	}
	command := strings.TrimSpace(args.Command)
	if command == "" {
		return turnStatusSignal{}, false
	}

	resultText, isToolError := toolResultText(toolResult)
	if isToolError {
		return turnStatusSignal{}, false
	}
	var result chattool.ExecuteResult
	if err := json.Unmarshal([]byte(resultText), &result); err != nil {
		return turnStatusSignal{}, false
	}

	switch {
	case isPRCreateCommand(command) && result.Success:
		return turnStatusSignal{
			Label:      "Submitted PR",
			Source:     turnStatusLabelSourceHeuristic,
			Success:    true,
			Confidence: 100,
		}, true
	case isTestCommand(command):
		if result.Success {
			return turnStatusSignal{
				Label:      "Finished tests",
				Source:     turnStatusLabelSourceHeuristic,
				Success:    true,
				Confidence: 100,
			}, true
		}
		return turnStatusSignal{
			Label:      "Tests failing",
			Source:     turnStatusLabelSourceHeuristic,
			Success:    false,
			Confidence: 100,
		}, true
	case isGitCommitCommand(command) && result.Success:
		return turnStatusSignal{
			Label:      "Created commit",
			Source:     turnStatusLabelSourceHeuristic,
			Success:    true,
			Confidence: 100,
		}, true
	default:
		return turnStatusSignal{}, false
	}
}

func fileUpdateTurnStatusSignal(toolResult fantasy.ToolResultContent) (turnStatusSignal, bool) {
	resultText, isToolError := toolResultText(toolResult)
	if isToolError {
		return turnStatusSignal{}, false
	}

	var payload struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(resultText), &payload); err != nil || !payload.OK {
		return turnStatusSignal{}, false
	}
	return turnStatusSignal{
		Label:      "Updated files",
		Source:     turnStatusLabelSourceTool,
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
	return isCommandInvocation(command, "gh", "pr", "create")
}

func isGitCommitCommand(command string) bool {
	return isCommandInvocation(command, "git", "commit")
}

func isTestCommand(command string) bool {
	testCommands := [][]string{
		{"go", "test"},
		{"npm", "test"},
		{"npm", "run", "test"},
		{"pnpm", "test"},
		{"pnpm", "run", "test"},
		{"yarn", "test"},
		{"yarn", "run", "test"},
		{"pytest"},
		{"vitest"},
		{"cargo", "test"},
		{"make", "test"},
	}
	for _, tokens := range testCommands {
		if isCommandInvocation(command, tokens...) {
			return true
		}
	}
	return false
}

func isCommandInvocation(command string, tokens ...string) bool {
	fields := normalizedCommandFields(command)
	for i := 0; i+len(tokens) <= len(fields); i++ {
		if !canStartCommandAt(fields, i) {
			continue
		}
		matched := true
		for j, token := range tokens {
			if fields[i+j] != token {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func normalizedCommandFields(command string) []string {
	rawFields := strings.Fields(strings.ToLower(command))
	fields := make([]string, 0, len(rawFields))
	for _, field := range rawFields {
		field = strings.Trim(field, "\"'`")
		trimmed := strings.TrimRight(field, ";")
		if trimmed != "" {
			fields = append(fields, trimmed)
		}
		if trimmed != field {
			fields = append(fields, ";")
		}
	}
	return fields
}

func canStartCommandAt(fields []string, index int) bool {
	if index == 0 || isShellCommandSeparator(fields[index-1]) {
		return true
	}
	for i := index - 1; i >= 0; i-- {
		if isShellCommandSeparator(fields[i]) {
			return true
		}
		if !isEnvironmentAssignment(fields[i]) {
			return false
		}
	}
	return true
}

func isEnvironmentAssignment(field string) bool {
	separator := strings.Index(field, "=")
	return separator > 0
}

func isShellCommandSeparator(field string) bool {
	switch field {
	case "&&", "||", ";", "|":
		return true
	default:
		return false
	}
}
