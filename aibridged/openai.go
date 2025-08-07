package aibridged

import (
	"regexp"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	"github.com/tidwall/gjson"
	"golang.org/x/xerrors"
	"tailscale.com/types/ptr"
)

// ChatCompletionNewParamsWrapper exists because the "stream" param is not included in openai.ChatCompletionNewParams.
type ChatCompletionNewParamsWrapper struct {
	openai.ChatCompletionNewParams `json:""`
	Stream                         bool `json:"stream,omitempty"`
}

func (c ChatCompletionNewParamsWrapper) MarshalJSON() ([]byte, error) {
	type shadow ChatCompletionNewParamsWrapper
	return param.MarshalWithExtras(c, (*shadow)(&c), map[string]any{
		"stream": c.Stream,
	})
}

func (c *ChatCompletionNewParamsWrapper) UnmarshalJSON(raw []byte) error {
	err := c.ChatCompletionNewParams.UnmarshalJSON(raw)
	if err != nil {
		return err
	}

	in := gjson.ParseBytes(raw)
	if stream := in.Get("stream"); stream.Exists() {
		c.Stream = stream.Bool()
		if c.Stream {
			c.ChatCompletionNewParams.StreamOptions = openai.ChatCompletionStreamOptionsParam{
				IncludeUsage: openai.Bool(true), // Always include usage when streaming.
			}
		} else {
			c.ChatCompletionNewParams.StreamOptions = openai.ChatCompletionStreamOptionsParam{}
		}
	} else {
		c.ChatCompletionNewParams.StreamOptions = openai.ChatCompletionStreamOptionsParam{}
	}

	return nil
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

func sumUsage(ref, in openai.CompletionUsage) openai.CompletionUsage {
	return openai.CompletionUsage{
		CompletionTokens: ref.CompletionTokens + in.CompletionTokens,
		PromptTokens:     ref.PromptTokens + in.PromptTokens,
		TotalTokens:      ref.TotalTokens + in.TotalTokens,
		CompletionTokensDetails: openai.CompletionUsageCompletionTokensDetails{
			AcceptedPredictionTokens: ref.CompletionTokensDetails.AcceptedPredictionTokens + in.CompletionTokensDetails.AcceptedPredictionTokens,
			AudioTokens:              ref.CompletionTokensDetails.AudioTokens + in.CompletionTokensDetails.AudioTokens,
			ReasoningTokens:          ref.CompletionTokensDetails.ReasoningTokens + in.CompletionTokensDetails.ReasoningTokens,
			RejectedPredictionTokens: ref.CompletionTokensDetails.RejectedPredictionTokens + in.CompletionTokensDetails.RejectedPredictionTokens,
		},
		PromptTokensDetails: openai.CompletionUsagePromptTokensDetails{
			AudioTokens:  ref.PromptTokensDetails.AudioTokens + in.PromptTokensDetails.AudioTokens,
			CachedTokens: ref.PromptTokensDetails.CachedTokens + in.PromptTokensDetails.CachedTokens,
		},
	}
}
