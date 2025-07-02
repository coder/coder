package aibridged

import (
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go"
	"golang.org/x/xerrors"
	"tailscale.com/types/ptr"
)

type UsageExtractor interface {
	LastUserPrompt() (*string, error)
	LastToolCalls() ([]string, error)
}

func (c *ChatCompletionNewParamsWrapper) LastUserPrompt() (*string, error) {
	if c == nil {
		return nil, xerrors.New("nil struct")
	}

	if len(c.Messages) == 0 {
		return nil, xerrors.New("no messages")
	}

	var msg *openai.ChatCompletionUserMessageParam
	for i := len(c.Messages) - 1; i >= 0; i-- {
		m := c.Messages[i]
		if m.OfUser != nil {
			msg = m.OfUser
			break
		}
	}

	if msg == nil {
		return nil, nil
	}

	userMessage := msg.Content.OfString.String()
	if isCursor, _ := regexp.MatchString("<user_query>", userMessage); isCursor {
		userMessage = extractCursorUserQuery(userMessage)
	}

	return ptr.To(strings.TrimSpace(userMessage)), nil
}

func (b *BetaMessageNewParamsWrapper) LastUserPrompt() (*string, error) {
	if b == nil {
		return nil, xerrors.New("nil struct")
	}

	if len(b.Messages) == 0 {
		return nil, xerrors.New("no messages")
	}

	var userMessage string
	for i := len(b.Messages) - 1; i >= 0; i-- {
		m := b.Messages[i]
		if m.Role != anthropic.BetaMessageParamRoleUser {
			continue
		}
		if len(m.Content) == 0 {
			continue
		}

		for j := len(m.Content) - 1; j >= 0; j-- {
			if textContent := m.Content[j].GetText(); textContent != nil {
				userMessage = *textContent
			}

			// Ignore internal Claude Code prompts.
			if userMessage == "test" ||
				strings.Contains(userMessage, "<system-reminder>") {
				userMessage = ""
				continue
			}

			// Handle Cursor-specific formatting by extracting content from <user_query> tags
			if isCursor, _ := regexp.MatchString("<user_query>", userMessage); isCursor {
				userMessage = extractCursorUserQuery(userMessage)
			}
			return ptr.To(strings.TrimSpace(userMessage)), nil
		}
	}

	return nil, nil
}

func extractCursorUserQuery(message string) string {
	pat := regexp.MustCompile(`<user_query>(?P<content>[\s\S]*?)</user_query>`)
	match := pat.FindStringSubmatch(message)
	if match != nil {
		// Get the named group by index
		contentIndex := pat.SubexpIndex("content")
		if contentIndex != -1 {
			message = match[contentIndex]
		}
	}
	return strings.TrimSpace(message)
}
