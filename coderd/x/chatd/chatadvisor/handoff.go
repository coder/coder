package chatadvisor

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"charm.land/fantasy"
)

const (
	// advisorRecentMessageLimit caps how many recent non-system messages
	// from the parent conversation are forwarded to the advisor. The
	// advisor only needs enough tail to ground its guidance, not the full
	// history.
	advisorRecentMessageLimit = 20
	// advisorConversationJSONByteBudget caps the combined size of the
	// forwarded recent messages, measured as JSON-serialized bytes (not
	// raw text runes). The JSON wrapping inflates the count relative to
	// user-visible text, so the effective text budget is smaller than the
	// number suggests. The walk stops at the first message that would
	// overflow, trading breadth for contiguity.
	advisorConversationJSONByteBudget = 12000
	// advisorSystemJSONByteBudget caps the combined size of inherited
	// system messages forwarded to the advisor. Without a cap, a large
	// parent system prompt (long injected instructions, accumulated
	// context) could push the advisor call past the model's context
	// window on top of the advisor contract, the recent tail, and the
	// question, surfacing as a provider error instead of advice.
	advisorSystemJSONByteBudget = 12000
	defaultAdvisorQuestion      = "Provide concise strategic guidance for the parent agent."
)

// BuildAdvisorMessages prepares a nested advisor prompt using the recent chat
// context plus the explicit advisor question.
func BuildAdvisorMessages(
	question string,
	conversationSnapshot []fantasy.Message,
) []fantasy.Message {
	trimmedQuestion := strings.TrimSpace(question)
	if trimmedQuestion == "" {
		trimmedQuestion = defaultAdvisorQuestion
	}

	messages := make([]fantasy.Message, 0, len(conversationSnapshot)+2)

	// Place inherited system messages before AdvisorSystemPrompt so the
	// advisor contract is the final system instruction the model sees.
	// Later system directives win when they conflict, and the parent's
	// prompt may tell the model to address the end user directly or use
	// tools. The advisor must override those behaviors, not be overridden
	// by them.
	//
	// Walk system messages newest-to-oldest when consuming the byte
	// budget so that truncation preserves the most recent directives.
	// The parent may have injected recent safety or user-instruction
	// blocks that should win over older foundational prompts, and later
	// directives override earlier ones anyway. After selection, restore
	// the original order before appending so the advisor still sees the
	// parent's intended directive sequence.
	inheritedSystem := make([]fantasy.Message, 0)
	remainingSystemBudget := advisorSystemJSONByteBudget
	for i := len(conversationSnapshot) - 1; i >= 0; i-- {
		msg := conversationSnapshot[i]
		if msg.Role != fantasy.MessageRoleSystem {
			continue
		}
		messageBytes := messageJSONByteCount(msg)
		if messageBytes > remainingSystemBudget {
			// Skip oversized inherited system messages rather
			// than forwarding them wholesale. A single massive
			// parent system prompt could otherwise push the
			// advisor prompt past the model's context window,
			// returning a provider error instead of advice.
			// Continue walking so smaller older directives can
			// still contribute; stopping here would drop them
			// solely because a newer sibling was oversized.
			continue
		}
		inheritedSystem = append(inheritedSystem, cloneMessage(msg))
		remainingSystemBudget -= messageBytes
	}
	slices.Reverse(inheritedSystem)
	messages = append(messages, inheritedSystem...)
	messages = append(messages, textMessage(fantasy.MessageRoleSystem, AdvisorSystemPrompt))

	recent := make([]fantasy.Message, 0, min(len(conversationSnapshot), advisorRecentMessageLimit))
	remainingBudget := advisorConversationJSONByteBudget
	for i := len(conversationSnapshot) - 1; i >= 0; i-- {
		msg := conversationSnapshot[i]
		if msg.Role == fantasy.MessageRoleSystem {
			continue
		}
		if len(recent) >= advisorRecentMessageLimit {
			break
		}

		messageBytes := messageJSONByteCount(msg)
		if messageBytes > remainingBudget {
			// Stop at the first message that doesn't fit so the
			// advisor window stays contiguous from most recent
			// backward. Skipping an oversized message would leave
			// the advisor with an invisible hole in the history,
			// where later messages reference context that is no
			// longer present.
			break
		}

		recent = append(recent, cloneMessage(msg))
		remainingBudget -= messageBytes
	}
	slices.Reverse(recent)
	recent = dropOrphanToolMessages(recent)
	messages = append(messages, recent...)
	messages = append(messages, textMessage(fantasy.MessageRoleUser, trimmedQuestion))
	return messages
}

// dropOrphanToolMessages removes tool-role messages whose tool-call references
// have been truncated out of the recent window. Providers reject prompts with
// tool_result blocks that do not have a matching tool_use, so a truncation cut
// that lands between an assistant tool-call message and its tool-result message
// would otherwise produce a provider error rather than advice. The backward
// walk always picks up tool results before their originating assistant
// message, so orphan results can only appear at the leading edge of the
// recent window. A single forward pass tracking known tool-call IDs is
// sufficient to drop them.
func dropOrphanToolMessages(recent []fantasy.Message) []fantasy.Message {
	if len(recent) == 0 {
		return recent
	}
	known := make(map[string]struct{})
	result := make([]fantasy.Message, 0, len(recent))
	for _, msg := range recent {
		if msg.Role == fantasy.MessageRoleAssistant {
			for _, part := range msg.Content {
				call, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part)
				if !ok {
					continue
				}
				known[call.ToolCallID] = struct{}{}
			}
			result = append(result, msg)
			continue
		}
		if msg.Role != fantasy.MessageRoleTool {
			result = append(result, msg)
			continue
		}

		kept := make([]fantasy.MessagePart, 0, len(msg.Content))
		for _, part := range msg.Content {
			tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
			if !ok {
				kept = append(kept, part)
				continue
			}
			if _, matched := known[tr.ToolCallID]; matched {
				kept = append(kept, part)
			}
		}
		if len(kept) == 0 {
			continue
		}
		trimmed := msg
		trimmed.Content = kept
		result = append(result, trimmed)
	}
	return result
}

func textMessage(role fantasy.MessageRole, text string) fantasy.Message {
	return fantasy.Message{
		Role: role,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: text},
		},
	}
}

func cloneMessage(msg fantasy.Message) fantasy.Message {
	cloned := msg
	cloned.Content = append([]fantasy.MessagePart(nil), msg.Content...)
	cloned.ProviderOptions = maps.Clone(msg.ProviderOptions)
	return cloned
}

// messageJSONByteCount approximates the message's contribution to the
// advisor prompt using the length of its JSON serialization. The JSON
// wrapping ({"role":"...","content":[{"type":"text","text":"..."}]}) is
// counted alongside the user-visible text; the measurement is intended
// for budget accounting, not for reporting visible character counts.
func messageJSONByteCount(msg fantasy.Message) int {
	data, err := json.Marshal(msg)
	if err == nil {
		return len(data)
	}

	total := 0
	for _, part := range msg.Content {
		partData, partErr := json.Marshal(part)
		if partErr == nil {
			total += len(partData)
		}
	}
	return total
}
