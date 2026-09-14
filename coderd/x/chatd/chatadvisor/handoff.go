package chatadvisor

import (
	"encoding/json"
	"fmt"
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
	recent = textualizeToolExchanges(recent)
	messages = append(messages, recent...)
	messages = append(messages, textMessage(fantasy.MessageRoleUser, trimmedQuestion))
	return messages
}

// textualizeToolExchanges rewrites tool activity as inline text notes.
// Assistant tool-call parts are removed, with their inputs folded into the
// note rendered for the matching tool result, and tool-role messages become
// user-role notes. The nested advisor call defines no tools, so
// assistant-authored tool artifacts in the transcript prime the model to
// imitate them instead of answering: raw tool_use/tool_result blocks yield
// an empty step ("advisor produced no text output"), and a bare
// "[tool call: ...]" text line yields that literal line back as advice.
// Folding each exchange into a single note leaves no assistant tool-call
// pattern to complete while keeping the activity visible, and it removes
// the provider requirement that tool_result blocks pair with a tool_use in
// the same request, so results whose calls were truncated out of the
// window can be kept instead of dropped.
func textualizeToolExchanges(recent []fantasy.Message) []fantasy.Message {
	// Tool results carry only the call ID, so record each call's name and
	// input as the forward walk scrubs assistant messages.
	type callInfo struct {
		name  string
		input string
	}
	calls := make(map[string]callInfo)
	result := make([]fantasy.Message, 0, len(recent))
	for _, msg := range recent {
		switch msg.Role {
		case fantasy.MessageRoleAssistant:
			parts := make([]fantasy.MessagePart, 0, len(msg.Content))
			for _, part := range msg.Content {
				call, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part)
				if !ok {
					parts = append(parts, part)
					continue
				}
				calls[call.ToolCallID] = callInfo{name: call.ToolName, input: call.Input}
			}
			if len(parts) == 0 {
				// The message carried only tool calls; the folded
				// result notes preserve the information.
				continue
			}
			msg.Content = parts
			result = append(result, msg)
		case fantasy.MessageRoleTool:
			parts := make([]fantasy.MessagePart, 0, len(msg.Content))
			for _, part := range msg.Content {
				tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
				if !ok {
					parts = append(parts, part)
					continue
				}
				output := renderToolResultOutput(tr.Output)
				note := fmt.Sprintf("[A tool run by the parent agent returned: %s]", output)
				if call, known := calls[tr.ToolCallID]; known {
					note = fmt.Sprintf(
						"[The parent agent ran the %s tool with input %s. Result: %s]",
						call.name, call.input, output,
					)
				}
				parts = append(parts, fantasy.TextPart{Text: note})
			}
			msg.Role = fantasy.MessageRoleUser
			msg.Content = parts
			result = append(result, msg)
		default:
			result = append(result, msg)
		}
	}
	return result
}

// renderToolResultOutput flattens a tool result payload into text for the
// advisor transcript. Media payloads are summarized instead of inlined
// because base64 data adds prompt bulk without helping a text-only advisor.
func renderToolResultOutput(output fantasy.ToolResultOutputContent) string {
	switch typed := output.(type) {
	case fantasy.ToolResultOutputContentText:
		return typed.Text
	case fantasy.ToolResultOutputContentError:
		if typed.Error != nil {
			return "error: " + typed.Error.Error()
		}
		return "error"
	case fantasy.ToolResultOutputContentMedia:
		if typed.Text != "" {
			return fmt.Sprintf("[%s media] %s", typed.MediaType, typed.Text)
		}
		return fmt.Sprintf("[%s media]", typed.MediaType)
	default:
		return ""
	}
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
