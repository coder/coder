package chatcompletions

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/xerrors"
)

// ChatCompletionNewParamsWrapper exists because the "stream" param is not included in openai.ChatCompletionNewParams.
type ChatCompletionNewParamsWrapper struct {
	openai.ChatCompletionNewParams `json:""`
	Stream                         bool `json:"stream,omitempty"`
	// ExtraBody preserves the OpenAI SDK's extra_body passthrough object,
	// which the typed params drop on unmarshal. It is forwarded only to
	// Google upstreams, which read provider-specific settings such as
	// Gemini's thinking_config from it.
	ExtraBody json.RawMessage `json:"-"`
	// PreservedFields holds client-sent JSON that the typed params drop on
	// unmarshal but that must reach the upstream at the same path, such as
	// the cache_control markers OpenRouter honors for Anthropic models.
	PreservedFields []preservedJSONField `json:"-"`
}

// preservedJSONField is a raw JSON value keyed by its gjson/sjson path
// in the request body.
type preservedJSONField struct {
	Path string
	Raw  json.RawMessage
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

	if extraBody := gjson.GetBytes(raw, "extra_body"); extraBody.IsObject() {
		c.ExtraBody = json.RawMessage(extraBody.Raw)
	}
	c.PreservedFields = preservedCacheControlFields(raw)

	c.Stream = gjson.GetBytes(raw, "stream").Bool()
	if c.Stream {
		c.StreamOptions = openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true), // Always include usage when streaming.
		}
	} else {
		c.StreamOptions = openai.ChatCompletionStreamOptionsParam{}
	}

	return nil
}

// maxPreservedCacheControlFields bounds the per-field body rewrites in
// applyPreservedFields, which each copy the whole body. Anthropic accepts
// at most four cache_control breakpoints per request, so a valid client
// never reaches the cap; markers past it are dropped.
const maxPreservedCacheControlFields = 4

func preservedCacheControlFields(raw []byte) []preservedJSONField {
	var fields []preservedJSONField
	record := func(path string, cc gjson.Result) {
		if cc.Exists() && len(fields) < maxPreservedCacheControlFields {
			fields = append(fields, preservedJSONField{Path: path, Raw: json.RawMessage(cc.Raw)})
		}
	}
	for i, message := range gjson.GetBytes(raw, "messages").Array() {
		record(fmt.Sprintf("messages.%d.cache_control", i), message.Get("cache_control"))
		if content := message.Get("content"); content.IsArray() {
			for j, part := range content.Array() {
				record(fmt.Sprintf("messages.%d.content.%d.cache_control", i, j), part.Get("cache_control"))
			}
		}
	}
	return fields
}

// applyPreservedFields re-applies PreservedFields to a body marshaled
// from the typed params. The bridge only appends messages for injected
// tool round trips, so recorded paths stay valid; a field whose parent
// object no longer exists in the rebuilt body is skipped rather than
// padded in by sjson.
func (c *ChatCompletionNewParamsWrapper) applyPreservedFields(body []byte) ([]byte, error) {
	for _, field := range c.PreservedFields {
		parent := field.Path[:strings.LastIndex(field.Path, ".")]
		if !gjson.GetBytes(body, parent).IsObject() {
			continue
		}
		var err error
		body, err = sjson.SetRawBytes(body, field.Path, field.Raw)
		if err != nil {
			return nil, xerrors.Errorf("re-apply %s: %w", field.Path, err)
		}
	}
	return body, nil
}

func (c *ChatCompletionNewParamsWrapper) lastUserPrompt() (*string, error) {
	if c == nil {
		return nil, xerrors.New("nil struct")
	}

	if len(c.Messages) == 0 {
		return nil, xerrors.New("no messages")
	}

	// We only care if the last message was issued by a user.
	msg := c.Messages[len(c.Messages)-1]
	if msg.OfUser == nil {
		return nil, nil //nolint:nilnil // no user prompt found is not an error
	}

	if msg.OfUser.Content.OfString.String() != "" {
		return new(msg.OfUser.Content.OfString.String()), nil
	}

	// Walk backwards on "user"-initiated message content. Clients often inject
	// content ahead of the actual prompt to provide context to the model,
	// so the last item in the slice is most likely the user's prompt.
	for i := len(msg.OfUser.Content.OfArrayOfContentParts) - 1; i >= 0; i-- {
		// Only text content is supported currently.
		if textContent := msg.OfUser.Content.OfArrayOfContentParts[i].OfText; textContent != nil {
			return &textContent.Text, nil
		}
	}

	return nil, nil //nolint:nilnil // no text content found is not an error
}
