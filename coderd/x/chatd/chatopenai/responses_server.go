//go:build !slim

package chatopenai

import (
	"slices"
	"strings"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

// ResolveChainMode scans DB messages from the end to inspect the current
// trailing user turn and detect whether the immediately preceding assistant/tool
// block can chain from a provider response ID.
func ResolveChainMode(messages []database.ChatMessage) ChainModeInfo {
	var info ChainModeInfo
	i := len(messages) - 1
	for ; i >= 0; i-- {
		if messages[i].Role != database.ChatMessageRoleUser {
			break
		}
		if userMessageContributesToChainMode(messages[i]) {
			info.contributingTrailingUserCount++
		}
	}
	for ; i >= 0; i-- {
		switch messages[i].Role {
		case database.ChatMessageRoleAssistant:
			if messages[i].ProviderResponseID.Valid &&
				messages[i].ProviderResponseID.String != "" {
				info.previousResponseID = messages[i].ProviderResponseID.String
				if messages[i].ModelConfigID.Valid {
					info.modelConfigID = messages[i].ModelConfigID.UUID
				}
				info.hasUnresolvedLocalToolCalls = assistantHasUnresolvedLocalToolCalls(messages, i)
				if !info.hasUnresolvedLocalToolCalls {
					info.providerMissingToolResults = providerHasMissingToolResults(messages, i)
				}
				return info
			}
			return info
		case database.ChatMessageRoleTool:
			continue
		default:
			return info
		}
	}
	return info
}

func userMessageContributesToChainMode(msg database.ChatMessage) bool {
	parts, err := chatprompt.ParseContent(msg)
	if err != nil {
		return false
	}
	for _, part := range parts {
		switch part.Type {
		case codersdk.ChatMessagePartTypeText,
			codersdk.ChatMessagePartTypeReasoning:
			if strings.TrimSpace(part.Text) != "" {
				return true
			}
		case codersdk.ChatMessagePartTypeFile,
			codersdk.ChatMessagePartTypeFileReference:
			return true
		case codersdk.ChatMessagePartTypeContextFile:
			if part.ContextFileContent != "" {
				return true
			}
		}
	}
	return false
}

// assistantHasUnresolvedLocalToolCalls reports whether the assistant message
// at assistantIdx contains local tool calls that lack matching tool results. It
// returns true when content parsing fails because full-history replay is safer
// than chaining from state that cannot be inspected.
func assistantHasUnresolvedLocalToolCalls(
	messages []database.ChatMessage,
	assistantIdx int,
) bool {
	if assistantIdx < 0 || assistantIdx >= len(messages) {
		return false
	}

	parts, err := chatprompt.ParseContent(messages[assistantIdx])
	if err != nil {
		// Use full replay when persisted assistant content cannot be parsed.
		return true
	}

	localCallIDs := make(map[string]struct{})
	for _, part := range parts {
		if part.Type != codersdk.ChatMessagePartTypeToolCall ||
			part.ProviderExecuted {
			continue
		}
		localCallIDs[part.ToolCallID] = struct{}{}
	}
	if len(localCallIDs) == 0 {
		return false
	}

	resolvedCallIDs := make(map[string]struct{})
	for i := assistantIdx + 1; i < len(messages); i++ {
		if messages[i].Role != database.ChatMessageRoleTool {
			break
		}
		parts, err := chatprompt.ParseContent(messages[i])
		if err != nil {
			// Use full replay when persisted tool content cannot be parsed.
			return true
		}
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeToolResult {
				continue
			}
			if _, ok := localCallIDs[part.ToolCallID]; ok {
				resolvedCallIDs[part.ToolCallID] = struct{}{}
			}
		}
	}

	return len(resolvedCallIDs) != len(localCallIDs)
}

// providerHasMissingToolResults reports whether the assistant message at
// assistantIdx has local tool calls whose results exist in the database but
// were never sent back to the provider. This is detected by the absence of a
// follow-up assistant message after the tool results. In normal flow the LLM
// processes tool results and produces a follow-up response, but StopAfterTool
// skips that round-trip.
func providerHasMissingToolResults(
	messages []database.ChatMessage,
	assistantIdx int,
) bool {
	if assistantIdx < 0 || assistantIdx >= len(messages) {
		return false
	}

	parts, err := chatprompt.ParseContent(messages[assistantIdx])
	if err != nil {
		// Parsing errors are already handled by
		// assistantHasUnresolvedLocalToolCalls.
		return false
	}

	if !slices.ContainsFunc(parts, func(p codersdk.ChatMessagePart) bool {
		return p.Type == codersdk.ChatMessagePartTypeToolCall && !p.ProviderExecuted
	}) {
		return false
	}

	// Scan forward past tool messages. If the first non-tool message is not an
	// assistant, the tool results were never round-tripped to the provider.
	for i := assistantIdx + 1; i < len(messages); i++ {
		switch messages[i].Role {
		case database.ChatMessageRoleTool:
			continue
		case database.ChatMessageRoleAssistant:
			// A follow-up assistant exists, so results were sent.
			return false
		default:
			// User or system message with no follow-up assistant.
			return true
		}
	}

	// Reached end of messages without a follow-up assistant.
	return true
}
