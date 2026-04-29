package bedrock

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream/eventstreamapi"
	"github.com/tidwall/gjson"

	"github.com/coder/coder/v2/aibridge/recorder"
)

// extractStreamingAudit parses a buffered AWS EventStream response
// and records audit metadata.
func (i *Interceptor) extractStreamingAudit(ctx context.Context, data []byte, promptText string, promptFound bool) {
	decoder := eventstream.NewDecoder()
	reader := bytes.NewReader(data)

	var msgID string

	type toolBlock struct {
		id   string
		name string
		args bytes.Buffer
	}
	var toolBlocks []toolBlock
	thinkingBlocks := map[int]*bytes.Buffer{}
	blockTypes := map[int]string{}

	for {
		msg, err := decoder.Decode(reader, nil)
		if err != nil {
			break
		}

		messageType := msg.Headers.Get(eventstreamapi.MessageTypeHeader)
		if messageType == nil || messageType.String() != eventstreamapi.EventMessageType {
			continue
		}
		eventType := msg.Headers.Get(eventstreamapi.EventTypeHeader)
		if eventType == nil || eventType.String() != "chunk" {
			continue
		}

		var chunk struct {
			Bytes string `json:"bytes"`
		}
		if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(chunk.Bytes)
		if err != nil {
			continue
		}

		eventKind := gjson.GetBytes(decoded, "type").String()

		switch eventKind {
		case "message_start":
			msgID = gjson.GetBytes(decoded, "message.id").String()
			usage := gjson.GetBytes(decoded, "message.usage")
			if usage.Exists() {
				_ = i.recorder.RecordTokenUsage(ctx, &recorder.TokenUsageRecord{
					InterceptionID:        i.id.String(),
					MsgID:                 msgID,
					Input:                 usage.Get("input_tokens").Int(),
					Output:                usage.Get("output_tokens").Int(),
					CacheReadInputTokens:  usage.Get("cache_read_input_tokens").Int(),
					CacheWriteInputTokens: usage.Get("cache_creation_input_tokens").Int(),
				})
			}
			if promptFound {
				_ = i.recorder.RecordPromptUsage(ctx, &recorder.PromptUsageRecord{
					InterceptionID: i.id.String(),
					MsgID:          msgID,
					Prompt:         promptText,
				})
				promptFound = false
			}

		case "message_delta":
			usage := gjson.GetBytes(decoded, "usage")
			if usage.Exists() {
				_ = i.recorder.RecordTokenUsage(ctx, &recorder.TokenUsageRecord{
					InterceptionID: i.id.String(),
					MsgID:          msgID,
					Output:         usage.Get("output_tokens").Int(),
				})
			}

		case "content_block_start":
			idx := int(gjson.GetBytes(decoded, "index").Int())
			blockType := gjson.GetBytes(decoded, "content_block.type").String()
			blockTypes[idx] = blockType

			if blockType == "tool_use" {
				toolBlocks = append(toolBlocks, toolBlock{
					id:   gjson.GetBytes(decoded, "content_block.id").String(),
					name: gjson.GetBytes(decoded, "content_block.name").String(),
				})
			}
			if blockType == "thinking" {
				thinkingBlocks[idx] = &bytes.Buffer{}
			}

		case "content_block_delta":
			idx := int(gjson.GetBytes(decoded, "index").Int())
			switch blockTypes[idx] {
			case "tool_use":
				partialJSON := gjson.GetBytes(decoded, "delta.partial_json").String()
				for ti := range toolBlocks {
					if toolBlocks[ti].id != "" {
						toolBlocks[len(toolBlocks)-1].args.WriteString(partialJSON)
						break
					}
				}
			case "thinking":
				if buf, ok := thinkingBlocks[idx]; ok {
					buf.WriteString(gjson.GetBytes(decoded, "delta.thinking").String())
				}
			}

		case "message_stop":
			for _, tb := range toolBlocks {
				var args json.RawMessage
				if tb.args.Len() > 0 {
					args = json.RawMessage(tb.args.Bytes())
				}
				_ = i.recorder.RecordToolUsage(ctx, &recorder.ToolUsageRecord{
					InterceptionID: i.id.String(),
					MsgID:          msgID,
					ToolCallID:     tb.id,
					Tool:           tb.name,
					Args:           args,
					Injected:       false,
				})
			}
			for _, buf := range thinkingBlocks {
				if buf.Len() > 0 {
					_ = i.recorder.RecordModelThought(ctx, &recorder.ModelThoughtRecord{
						InterceptionID: i.id.String(),
						Content:        buf.String(),
						Metadata:       recorder.Metadata{"source": recorder.ThoughtSourceThinking},
					})
				}
			}
		}
	}
}

// extractBlockingAudit parses a JSON response body and records audit
// metadata.
func (i *Interceptor) extractBlockingAudit(ctx context.Context, data []byte, promptText string, promptFound bool) {
	msgID := gjson.GetBytes(data, "id").String()

	if promptFound {
		_ = i.recorder.RecordPromptUsage(ctx, &recorder.PromptUsageRecord{
			InterceptionID: i.id.String(),
			MsgID:          msgID,
			Prompt:         promptText,
		})
	}

	usage := gjson.GetBytes(data, "usage")
	if usage.Exists() {
		_ = i.recorder.RecordTokenUsage(ctx, &recorder.TokenUsageRecord{
			InterceptionID:        i.id.String(),
			MsgID:                 msgID,
			Input:                 usage.Get("input_tokens").Int(),
			Output:                usage.Get("output_tokens").Int(),
			CacheReadInputTokens:  usage.Get("cache_read_input_tokens").Int(),
			CacheWriteInputTokens: usage.Get("cache_creation_input_tokens").Int(),
		})
	}

	content := gjson.GetBytes(data, "content")
	if content.IsArray() {
		content.ForEach(func(_, block gjson.Result) bool {
			switch block.Get("type").String() {
			case "tool_use":
				_ = i.recorder.RecordToolUsage(ctx, &recorder.ToolUsageRecord{
					InterceptionID: i.id.String(),
					MsgID:          msgID,
					ToolCallID:     block.Get("id").String(),
					Tool:           block.Get("name").String(),
					Args:           json.RawMessage(block.Get("input").Raw),
					Injected:       false,
				})
			case "thinking":
				thinking := block.Get("thinking").String()
				if thinking != "" {
					_ = i.recorder.RecordModelThought(ctx, &recorder.ModelThoughtRecord{
						InterceptionID: i.id.String(),
						Content:        thinking,
						Metadata:       recorder.Metadata{"source": recorder.ThoughtSourceThinking},
					})
				}
			}
			return true
		})
	}
}
