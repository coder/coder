package chatadvisor

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
)

const (
	advisorRecentMessageLimit     = 20
	advisorConversationCharBudget = 12000
	defaultAdvisorQuestion        = "Provide concise strategic guidance for the parent agent."
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
	remainingChars := advisorConversationCharBudget
	for i := len(conversationSnapshot) - 1; i >= 0; i-- {
		msg := conversationSnapshot[i]
		if msg.Role == fantasy.MessageRoleSystem {
			continue
		}
		if len(recent) >= advisorRecentMessageLimit {
			break
		}

		messageChars := messageCharCount(msg)
		if messageChars > remainingChars {
			continue
		}

		recent = append(recent, cloneMessage(msg))
		remainingChars -= messageChars
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

func messageCharCount(msg fantasy.Message) int {
	data, err := json.Marshal(msg)
	if err == nil {
		return utf8.RuneCount(data)
	}

	total := 0
	for _, part := range msg.Content {
		partData, partErr := json.Marshal(part)
		if partErr == nil {
			total += utf8.RuneCount(partData)
		}
	}
	return total
}
