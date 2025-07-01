package aibridged

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	ant_param "github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/tidwall/gjson"
)

type streamer interface {
	UseStreaming() bool
}

// ConvertStringContentToArrayTest exports the function for testing
func ConvertStringContentToArrayTest(raw []byte) ([]byte, error) {
	return convertStringContentToArray(raw)
}

// convertStringContentToArray converts string content to array format for Anthropic messages.
// https://docs.anthropic.com/en/api/messages#body-messages
//
// Each input message content may be either a single string or an array of content blocks, where each block has a
// specific type. Using a string for content is shorthand for an array of one content block of type "text".
//
func convertStringContentToArray(raw []byte) ([]byte, error) {
	in := gjson.ParseBytes(raw)

	// Check if messages exist and need content conversion
	if messages := in.Get("messages"); messages.Exists() {
		var modifiedJSON map[string]interface{}
		if err := json.Unmarshal(raw, &modifiedJSON); err != nil {
			return raw, err
		}

		convertStringContentRecursive(modifiedJSON)

		// Marshal back to JSON
		return json.Marshal(modifiedJSON)
	}

	return raw, nil
}

// convertStringContentRecursive recursively scans JSON data and converts string "content" fields
// to proper text block arrays where needed for Anthropic SDK compatibility
func convertStringContentRecursive(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		// Check if this object has a "content" field with string value
		if content, hasContent := v["content"]; hasContent {
			if contentStr, isString := content.(string); isString {
				// Check if this needs conversion based on context
				if shouldConvertContentField(v) {
					v["content"] = []map[string]interface{}{
						{
							"type": "text",
							"text": contentStr,
						},
					}
				}
			}
		}

		// Recursively process all values in the map
		for _, value := range v {
			convertStringContentRecursive(value)
		}

	case []interface{}:
		// Recursively process all items in the array
		for _, item := range v {
			convertStringContentRecursive(item)
		}
	}
}

// shouldConvertContentField determines if a "content" string field should be converted to text block array
func shouldConvertContentField(obj map[string]interface{}) bool {
	// Check if this is a message-level content (has "role" field)
	if _, hasRole := obj["role"]; hasRole {
		return true
	}

	// Check if this is a tool_result block (but not mcp_tool_result which supports strings)
	if objType, hasType := obj["type"].(string); hasType {
		switch objType {
		case "tool_result":
			return true // Regular tool_result needs array format
		case "mcp_tool_result":
			return false // MCP tool_result supports strings
		}
	}

	return false
}

// extractStreamFlag extracts the stream flag from JSON
func extractStreamFlag(raw []byte) bool {
	in := gjson.ParseBytes(raw)
	if streamVal := in.Get("stream"); streamVal.Exists() {
		return streamVal.Bool()
	}
	return false
}

// MessageNewParamsWrapper exists because the "stream" param is not included in anthropic.MessageNewParams.
type MessageNewParamsWrapper struct {
	anthropic.MessageNewParams `json:""`
	Stream                     bool `json:"stream,omitempty"`
}

func (b MessageNewParamsWrapper) MarshalJSON() ([]byte, error) {
	type shadow MessageNewParamsWrapper
	return ant_param.MarshalWithExtras(b, (*shadow)(&b), map[string]any{
		"stream": b.Stream,
	})
}

func (b *MessageNewParamsWrapper) UnmarshalJSON(raw []byte) error {
	convertedRaw, err := convertStringContentToArray(raw)
	if err != nil {
		return err
	}

	err = b.MessageNewParams.UnmarshalJSON(convertedRaw)
	if err != nil {
		return err
	}

	b.Stream = extractStreamFlag(raw)
	return nil
}
func (b *MessageNewParamsWrapper) UseStreaming() bool {
	return b.Stream
}

// BetaMessageNewParamsWrapper exists because the "stream" param is not included in anthropic.BetaMessageNewParams.
type BetaMessageNewParamsWrapper struct {
	anthropic.BetaMessageNewParams `json:""`
	Stream                         bool `json:"stream,omitempty"`
}

func (b BetaMessageNewParamsWrapper) MarshalJSON() ([]byte, error) {
	type shadow BetaMessageNewParamsWrapper
	return ant_param.MarshalWithExtras(b, (*shadow)(&b), map[string]any{
		"stream": b.Stream,
	})
}

func (b *BetaMessageNewParamsWrapper) UnmarshalJSON(raw []byte) error {
	convertedRaw, err := convertStringContentToArray(raw)
	if err != nil {
		return err
	}

	err = b.BetaMessageNewParams.UnmarshalJSON(convertedRaw)
	if err != nil {
		return err
	}

	b.Stream = extractStreamFlag(raw)
	return nil
}
func (b *BetaMessageNewParamsWrapper) UseStreaming() bool {
	return b.Stream
}
