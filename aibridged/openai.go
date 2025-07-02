package aibridged

import (
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	"github.com/tidwall/gjson"
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
