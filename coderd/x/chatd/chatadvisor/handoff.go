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
	defaultAdvisorQuestion            = "Provide concise strategic guidance for the parent agent."
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
	messages = append(messages, textMessage(fantasy.MessageRoleSystem, AdvisorSystemPrompt))

	for _, msg := range conversationSnapshot {
		if msg.Role != fantasy.MessageRoleSystem {
			continue
		}
		messages = append(messages, cloneMessage(msg))
	}

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
	messages = append(messages, recent...)
	messages = append(messages, textMessage(fantasy.MessageRoleUser, trimmedQuestion))
	return messages
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
