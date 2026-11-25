package extractors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/tidwall/gjson"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/intercept/messages"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/x/proxy/replaybody"
)

const (
	messagesRespPathID      = "id"
	messagesRespPathModel   = "model"
	messagesRespPathContent = "content"
	messagesRespPathUsage   = "usage"

	// Fields relative to a content block.
	messagesRespFieldID       = "id"
	messagesRespFieldType     = "type"
	messagesRespFieldName     = "name"
	messagesRespFieldInput    = "input"
	messagesRespFieldThinking = "thinking"

	// Paths relative to the usage object.
	messagesUsagePathInputTokens           = "input_tokens"
	messagesUsagePathOutputTokens          = "output_tokens"
	messagesUsagePathCacheReadInputTokens  = "cache_read_input_tokens"
	messagesUsagePathCacheWriteInputTokens = "cache_creation_input_tokens"
	messagesUsagePathServiceTier           = "service_tier"
	messagesUsagePathServerToolUse         = "server_tool_use"
	messagesUsagePathCacheCreation         = "cache_creation"
	messagesUsagePathWebSearchRequests     = "server_tool_use.web_search_requests"
	messagesUsagePathEphemeral1hInput      = "cache_creation.ephemeral_1h_input_tokens"
	messagesUsagePathEphemeral5mInput      = "cache_creation.ephemeral_5m_input_tokens"
)

// Content block types.
var (
	messagesRespTypeThinking      = string(constant.ValueOf[constant.Thinking]())
	messagesRespTypeToolUse       = string(constant.ValueOf[constant.ToolUse]())
	messagesRespTypeServerToolUse = string(constant.ValueOf[constant.ServerToolUse]())
	messagesRespTypeMCPToolUse    = string(constant.ValueOf[constant.MCPToolUse]())
)

// messagesExtractor extracts recording facts from Anthropic Messages API
// traffic.
type messagesExtractor struct {
	logger slog.Logger

	// prompt is the last user prompt in the request, nil when absent. It is
	// recorded once response message ID is known (PromptUsageRecord.MsgID).
	prompt *string
	// streaming reports whether the request asked for a streamed response.
	// Used as fallback, response metadata has priority.
	streaming bool
}

// NewMessages returns an [Extractor] for one Anthropic Messages API request.
func NewMessages(logger slog.Logger) Extractor {
	return &messagesExtractor{logger: logger}
}

func (e *messagesExtractor) ExtractRequest(reqBody []byte, start *recorder.InterceptionRecord) error {
	payload, err := messages.NewRequestPayload(reqBody)
	if err != nil {
		return xerrors.Errorf("parse messages request: %w", err)
	}
	start.Model = payload.Model()
	start.CorrelatingToolCallID = payload.CorrelatingToolCallID()
	e.streaming = payload.Stream()

	prompt, found, err := payload.LastUserPrompt()
	if err != nil {
		return xerrors.Errorf("read last user prompt: %w", err)
	}
	if found {
		e.prompt = &prompt
	}
	return nil
}

func (e *messagesExtractor) ExtractResponse(ctx context.Context, statusCode int, header http.Header, body *replaybody.TeeBuffer) (ResponseResult, error) {
	// No upstream response was accepted, so there is nothing to extract.
	if body == nil {
		return ResponseResult{}, nil
	}
	// Error responses carry an error envelope whatever the request asked for.
	if statusCode >= http.StatusBadRequest {
		return ResponseResult{ResponseError: decodeMessagesError(statusCode, body.Bytes())}, nil
	}
	// The response content type takes precedence over the request's stream
	// flag.
	streaming, err := streamingFromContentType(header)
	if err != nil {
		e.logger.Debug(ctx, "failed to determine response format; using request stream flag", slog.F("content_type", header.Get("Content-Type")), slog.Error(err))
		streaming = e.streaming
	}
	if streaming {
		// TODO: extract streamed responses. https://linear.app/codercom/issue/AIGOV-616
		return ResponseResult{}, nil
	}
	return e.extractBlocking(body.Bytes())
}

// extractBlocking extracts facts from a JSON Message response.
func (e *messagesExtractor) extractBlocking(body []byte) (ResponseResult, error) {
	if !json.Valid(body) {
		return ResponseResult{}, xerrors.New("decode messages response: invalid JSON")
	}
	msg := gjson.ParseBytes(body)
	if !msg.IsObject() {
		return ResponseResult{}, xerrors.New("decode messages response: expected response object")
	}

	msgID, idErr := messagesStringField(msg, messagesRespPathID)
	providerModel, modelErr := messagesStringField(msg, messagesRespPathModel)
	extractErr := errors.Join(idErr, modelErr)
	var result ResponseResult
	if e.prompt != nil && msgID != "" {
		result.Prompt = &recorder.PromptUsageRecord{MsgID: msgID, Prompt: *e.prompt}
	}
	usage, usageErr := messagesTokenUsage(msgID, providerModel, msg.Get(messagesRespPathUsage))
	result.Tokens = []recorder.TokenUsageRecord{usage}
	extractErr = errors.Join(extractErr, usageErr)

	content := msg.Get(messagesRespPathContent)
	if content.Type != gjson.Null && !content.IsArray() {
		return result, errors.Join(extractErr, xerrors.Errorf("unexpected content type: %s", content.Type))
	}
	for index, block := range content.Array() {
		if !block.IsObject() {
			extractErr = errors.Join(extractErr, xerrors.Errorf("unexpected content block at index %d: expected object", index))
			continue
		}
		blockType, typeErr := messagesStringField(block, messagesRespFieldType)
		if typeErr != nil {
			extractErr = errors.Join(extractErr, typeErr)
			continue
		}
		switch blockType {
		case messagesRespTypeThinking:
			thinking, thinkingErr := messagesStringField(block, messagesRespFieldThinking)
			extractErr = errors.Join(extractErr, thinkingErr)
			if thinking != "" {
				result.Thoughts = append(result.Thoughts, recorder.ModelThoughtRecord{
					Content:  thinking,
					Metadata: recorder.Metadata{"source": recorder.ThoughtSourceThinking},
				})
			}
		case messagesRespTypeToolUse, messagesRespTypeServerToolUse, messagesRespTypeMCPToolUse:
			toolCallID, idErr := messagesStringField(block, messagesRespFieldID)
			toolName, nameErr := messagesStringField(block, messagesRespFieldName)
			extractErr = errors.Join(extractErr, idErr, nameErr)
			if idErr != nil || nameErr != nil || toolCallID == "" {
				continue
			}
			var args json.RawMessage
			if input := block.Get(messagesRespFieldInput); input.Type != gjson.Null {
				args = json.RawMessage(input.Raw)
			}
			result.Tools = append(result.Tools, recorder.ToolUsageRecord{
				MsgID:      msgID,
				ToolCallID: toolCallID,
				Tool:       toolName,
				Args:       args,
			})
		}
	}
	return result, extractErr
}

func messagesStringField(parent gjson.Result, path string) (string, error) {
	field := parent.Get(path)
	if field.Type != gjson.Null && field.Type != gjson.String {
		return "", xerrors.Errorf("unexpected %s type: %s", path, field.Type)
	}
	return field.Str, nil
}

// messagesTokenUsage converts usage reported for message msgID to a token
// record, matching the record interception mode writes.
func messagesTokenUsage(msgID, providerModel string, usage gjson.Result) (recorder.TokenUsageRecord, error) {
	record := recorder.TokenUsageRecord{MsgID: msgID, ProviderModel: providerModel}
	if usage.Type != gjson.Null && !usage.IsObject() {
		return record, xerrors.Errorf("unexpected usage type: %s", usage.Type)
	}
	var usageErr error
	for _, path := range []string{messagesUsagePathServerToolUse, messagesUsagePathCacheCreation} {
		field := usage.Get(path)
		if field.Type != gjson.Null && !field.IsObject() {
			usageErr = errors.Join(usageErr, xerrors.Errorf("unexpected %s type: %s", path, field.Type))
		}
	}
	readInt := func(path string) int64 {
		field := usage.Get(path)
		if field.Type == gjson.Null {
			return 0
		}
		if field.Type != gjson.Number {
			usageErr = errors.Join(usageErr, xerrors.Errorf("unexpected %s type: %s", path, field.Type))
			return 0
		}
		value, err := strconv.ParseInt(field.Raw, 10, 64)
		if err != nil {
			usageErr = errors.Join(usageErr, xerrors.Errorf("invalid integer in %s", path))
			return 0
		}
		return value
	}
	record.Input = readInt(messagesUsagePathInputTokens)
	record.Output = readInt(messagesUsagePathOutputTokens)
	record.CacheReadInputTokens = readInt(messagesUsagePathCacheReadInputTokens)
	record.CacheWriteInputTokens = readInt(messagesUsagePathCacheWriteInputTokens)
	serviceTier, tierErr := messagesStringField(usage, messagesUsagePathServiceTier)
	usageErr = errors.Join(usageErr, tierErr)
	if serviceTier != "" {
		record.Metadata = recorder.Metadata{recorder.MetadataKeyServiceTier: serviceTier}
	}
	extra := make(map[string]int64)
	if count := readInt(messagesUsagePathWebSearchRequests); count != 0 {
		extra["web_search_requests"] = count
	}
	if count := readInt(messagesUsagePathEphemeral1hInput); count != 0 {
		extra["cache_ephemeral_1h_input"] = count
	}
	if count := readInt(messagesUsagePathEphemeral5mInput); count != 0 {
		extra["cache_ephemeral_5m_input"] = count
	}
	if len(extra) > 0 {
		record.ExtraTokenTypes = extra
	}
	return record, usageErr
}

// decodeMessagesError decodes the Messages API error envelope of an HTTP
// error response. It returns nil when body has no error message, which leaves
// classification to the response status and raw body.
func decodeMessagesError(statusCode int, body []byte) error {
	var envelope anthropic.ErrorResponse
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Error.Message == "" {
		return nil
	}
	return &messages.ResponseError{ErrorResponse: &envelope, StatusCode: statusCode}
}
