package extractors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/tidwall/gjson"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/intercept/messages"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/sse"
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

	// Paths relative to a stream event.
	messagesEventPathMessage      = "message"
	messagesEventPathIndex        = "index"
	messagesEventPathContentBlock = "content_block"
	messagesEventPathDelta        = "delta"
	messagesEventPathUsage        = "usage"

	// Fields relative to a content block delta.
	messagesDeltaFieldType        = "type"
	messagesDeltaFieldPartialJSON = "partial_json"
	messagesDeltaFieldThinking    = "thinking"
)

// Content block types.
var (
	messagesRespTypeThinking      = string(constant.ValueOf[constant.Thinking]())
	messagesRespTypeToolUse       = string(constant.ValueOf[constant.ToolUse]())
	messagesRespTypeServerToolUse = string(constant.ValueOf[constant.ServerToolUse]())
	messagesRespTypeMCPToolUse    = string(constant.ValueOf[constant.MCPToolUse]())
)

// Stream event and content block delta types.
var (
	messagesEventMessageStart      = string(constant.ValueOf[constant.MessageStart]())
	messagesEventMessageDelta      = string(constant.ValueOf[constant.MessageDelta]())
	messagesEventMessageStop       = string(constant.ValueOf[constant.MessageStop]())
	messagesEventContentBlockStart = string(constant.ValueOf[constant.ContentBlockStart]())
	messagesEventContentBlockDelta = string(constant.ValueOf[constant.ContentBlockDelta]())

	messagesDeltaTypeInputJSON = string(constant.ValueOf[constant.InputJSONDelta]())
	messagesDeltaTypeThinking  = string(constant.ValueOf[constant.ThinkingDelta]())
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

func (e *messagesExtractor) ExtractResponse(ctx context.Context, statusCode int, header http.Header, body []byte) (ResponseResult, error) {
	// No upstream response was accepted, so there is nothing to extract.
	if body == nil {
		return ResponseResult{}, nil
	}
	// Error responses carry an error envelope whatever the request asked for.
	if statusCode >= http.StatusBadRequest {
		return ResponseResult{ResponseError: decodeMessagesError(statusCode, body)}, nil
	}
	// The response content type takes precedence over the request's stream
	// flag.
	streaming, err := streamingFromContentType(header)
	if err != nil {
		e.logger.Debug(ctx, "failed to determine response format; using request stream flag", slog.F("content_type", header.Get("Content-Type")), slog.Error(err))
		streaming = e.streaming
	}
	if streaming {
		return e.extractStreaming(body)
	}
	return e.extractBlocking(body)
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
	usage := recorder.TokenUsageRecord{MsgID: msgID, ProviderModel: providerModel}
	usageErr := updateMessagesTokenUsage(&usage, msg.Get(messagesRespPathUsage))
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

// messagesStreamBlock holds the facts of one streamed content block.
type messagesStreamBlock struct {
	blockType  string
	toolCallID string
	toolName   string
	// input is the tool input from the start event followed by its streamed
	// fragments, possibly invalid JSON when the stream was truncated.
	input    []byte
	thinking strings.Builder
}

// messagesStream accumulates the facts of a Messages SSE stream as its events
// are applied in stream order.
type messagesStream struct {
	msgID         string
	usage         *recorder.TokenUsageRecord
	responseError error
	// blocks preserves start order; byIndex locates the block for each delta.
	blocks  []*messagesStreamBlock
	byIndex map[int64]*messagesStreamBlock
}

// extractStreaming extracts facts from a Messages SSE stream. It reads only the
// event fields needed for recording; it does not rebuild the message.
func (e *messagesExtractor) extractStreaming(body []byte) (ResponseResult, error) {
	parser := sse.NewParser()
	var extractErr error
	if err := parser.Parse(bytes.NewReader(body)); err != nil {
		extractErr = xerrors.Errorf("parse messages stream: %w", err)
	}

	events := parser.Events()
	stream := &messagesStream{byIndex: make(map[int64]*messagesStreamBlock)}
	var starts, stops int
	for _, event := range events {
		switch event.Type {
		case messagesEventMessageStart:
			starts++
		case messagesEventMessageStop:
			stops++
			continue
		}
		// Don't merge another message's usage or tool calls into the first
		// message's record. Upstream error events still apply to the request.
		if (starts > 1 || stops > 0) && event.Type != sse.EventTypeError {
			continue
		}
		var err error
		switch event.Type {
		case messagesEventMessageStart:
			err = stream.messageStart(event)
		case messagesEventMessageDelta:
			err = stream.messageDelta(event)
		case messagesEventContentBlockStart:
			err = stream.contentBlockStart(event)
		case messagesEventContentBlockDelta:
			err = stream.contentBlockDelta(event)
		case sse.EventTypeError:
			err = stream.streamError(event)
		}
		extractErr = errors.Join(extractErr, err)
	}

	result := stream.result(e.prompt)
	// An upstream error can terminate the stream before either boundary.
	if result.ResponseError != nil && starts <= 1 && stops <= 1 {
		return result, extractErr
	}
	if starts != 1 || stops != 1 {
		extractErr = errors.Join(extractErr, xerrors.Errorf("expected one message_start and one message_stop: message_start=%d, message_stop=%d", starts, stops))
	}
	return result, extractErr
}

// messageStart captures the message identity and initial usage.
func (s *messagesStream) messageStart(event sse.Event) error {
	data, err := messagesEventData(event)
	if err != nil {
		return err
	}
	msg := data.Get(messagesEventPathMessage)
	if !msg.IsObject() {
		return xerrors.Errorf("unexpected %s message type: %s", event.Type, msg.Type)
	}
	var idErr error
	s.msgID, idErr = messagesStringField(msg, messagesRespPathID)
	model, modelErr := messagesStringField(msg, messagesRespPathModel)
	s.usage = &recorder.TokenUsageRecord{MsgID: s.msgID, ProviderModel: model}
	usageErr := updateMessagesTokenUsage(s.usage, msg.Get(messagesRespPathUsage))
	return errors.Join(idErr, modelErr, usageErr)
}

// messageDelta updates the message's cumulative usage.
func (s *messagesStream) messageDelta(event sse.Event) error {
	data, err := messagesEventData(event)
	if err != nil {
		return err
	}
	if s.usage == nil {
		return xerrors.New("message_delta without message_start")
	}
	return updateMessagesTokenUsage(s.usage, data.Get(messagesEventPathUsage))
}

// contentBlockStart adds a content block to the current message.
func (s *messagesStream) contentBlockStart(event sse.Event) error {
	data, index, err := messagesBlockEventData(event)
	if err != nil {
		return err
	}
	if _, ok := s.byIndex[index]; ok {
		return xerrors.Errorf("duplicate content block at index %d", index)
	}
	content := data.Get(messagesEventPathContentBlock)
	if !content.IsObject() {
		return xerrors.Errorf("unexpected content block at index %d: expected object", index)
	}
	blockType, err := messagesStringField(content, messagesRespFieldType)
	if err != nil {
		return err
	}
	block := &messagesStreamBlock{blockType: blockType}
	switch blockType {
	case messagesRespTypeThinking:
		thinking, thinkingErr := messagesStringField(content, messagesRespFieldThinking)
		err = thinkingErr
		_, _ = block.thinking.WriteString(thinking)
	case messagesRespTypeToolUse, messagesRespTypeServerToolUse, messagesRespTypeMCPToolUse:
		toolCallID, idErr := messagesStringField(content, messagesRespFieldID)
		toolName, nameErr := messagesStringField(content, messagesRespFieldName)
		err = errors.Join(idErr, nameErr)
		if err == nil {
			block.toolCallID = toolCallID
			block.toolName = toolName
		}
		if input := content.Get(messagesRespFieldInput); input.Type != gjson.Null {
			block.input = []byte(input.Raw)
		}
	}
	s.byIndex[index] = block
	s.blocks = append(s.blocks, block)
	return err
}

// contentBlockDelta appends a fragment to a content block of the current
// message.
func (s *messagesStream) contentBlockDelta(event sse.Event) error {
	data, index, err := messagesBlockEventData(event)
	if err != nil {
		return err
	}
	block, ok := s.byIndex[index]
	if !ok {
		return xerrors.Errorf("%s for unknown content block at index %d", event.Type, index)
	}
	delta := data.Get(messagesEventPathDelta)
	deltaType, err := messagesStringField(delta, messagesDeltaFieldType)
	if err != nil {
		return err
	}
	switch deltaType {
	case messagesDeltaTypeInputJSON:
		partial, err := messagesStringField(delta, messagesDeltaFieldPartialJSON)
		if partial == "" {
			return err
		}
		// Like the SDK accumulator, the first fragment replaces the empty
		// input object the start event carries.
		if string(block.input) == "{}" {
			block.input = nil
		}
		block.input = append(block.input, partial...)
		return err
	case messagesDeltaTypeThinking:
		thinking, err := messagesStringField(delta, messagesDeltaFieldThinking)
		_, _ = block.thinking.WriteString(thinking)
		return err
	}
	return nil
}

// streamError decodes the first error event as the response's failure.
func (s *messagesStream) streamError(event sse.Event) error {
	if s.responseError != nil {
		return nil
	}
	var envelope anthropic.ErrorResponse
	if err := json.Unmarshal([]byte(event.Data), &envelope); err != nil {
		return xerrors.Errorf("decode %s event: %w", event.Type, err)
	}
	s.responseError = &messages.ResponseError{ErrorResponse: &envelope}
	return nil
}

// result returns the facts of the applied events. prompt is the request's
// last user prompt, nil when absent.
func (s *messagesStream) result(prompt *string) ResponseResult {
	result := ResponseResult{ResponseError: s.responseError}
	if s.usage != nil {
		result.Tokens = []recorder.TokenUsageRecord{*s.usage}
	}
	if prompt != nil && s.msgID != "" {
		result.Prompt = &recorder.PromptUsageRecord{MsgID: s.msgID, Prompt: *prompt}
	}
	for _, block := range s.blocks {
		switch block.blockType {
		case messagesRespTypeThinking:
			if block.thinking.Len() > 0 {
				result.Thoughts = append(result.Thoughts, recorder.ModelThoughtRecord{
					Content:  block.thinking.String(),
					Metadata: recorder.Metadata{"source": recorder.ThoughtSourceThinking},
				})
			}
		case messagesRespTypeToolUse, messagesRespTypeServerToolUse, messagesRespTypeMCPToolUse:
			if block.toolCallID == "" {
				continue
			}
			// Truncated input is recorded as received rather than completed.
			var args recorder.ToolArgs = json.RawMessage(block.input)
			if len(block.input) > 0 && !json.Valid(block.input) {
				args = string(block.input)
			}
			result.Tools = append(result.Tools, recorder.ToolUsageRecord{
				MsgID:      s.msgID,
				ToolCallID: block.toolCallID,
				Tool:       block.toolName,
				Args:       args,
			})
		}
	}
	return result
}

// messagesEventData decodes the JSON object an SSE event carries.
func messagesEventData(event sse.Event) (gjson.Result, error) {
	if !gjson.Valid(event.Data) {
		return gjson.Result{}, xerrors.Errorf("decode %s event: invalid JSON", event.Type)
	}
	data := gjson.Parse(event.Data)
	if !data.IsObject() {
		return gjson.Result{}, xerrors.Errorf("decode %s event: expected object", event.Type)
	}
	return data, nil
}

// messagesBlockEventData decodes a content block event and its block index.
func messagesBlockEventData(event sse.Event) (gjson.Result, int64, error) {
	data, err := messagesEventData(event)
	if err != nil {
		return gjson.Result{}, 0, err
	}
	if !data.Get(messagesEventPathIndex).Exists() {
		return gjson.Result{}, 0, xerrors.Errorf("decode %s event: missing %s", event.Type, messagesEventPathIndex)
	}
	index, err := messagesIntField(data, messagesEventPathIndex, 0)
	if err != nil {
		return gjson.Result{}, 0, xerrors.Errorf("decode %s event: %w", event.Type, err)
	}
	return data, index, nil
}

func messagesStringField(parent gjson.Result, path string) (string, error) {
	field := parent.Get(path)
	if field.Type != gjson.Null && field.Type != gjson.String {
		return "", xerrors.Errorf("unexpected %s type: %s", path, field.Type)
	}
	return field.Str, nil
}

// updateMessagesTokenUsage applies the reported counts as totals. Fields
// omitted by a later stream event retain their last reported values. Only
// positive counts update ExtraTokenTypes.
func updateMessagesTokenUsage(record *recorder.TokenUsageRecord, usage gjson.Result) error {
	if usage.Type != gjson.Null && !usage.IsObject() {
		return xerrors.Errorf("unexpected usage type: %s", usage.Type)
	}
	var usageErr error
	for _, path := range []string{messagesUsagePathServerToolUse, messagesUsagePathCacheCreation} {
		field := usage.Get(path)
		if field.Type != gjson.Null && !field.IsObject() {
			usageErr = errors.Join(usageErr, xerrors.Errorf("unexpected %s type: %s", path, field.Type))
		}
	}
	readInt := func(path string, previous int64) int64 {
		value, err := messagesIntField(usage, path, previous)
		usageErr = errors.Join(usageErr, err)
		return value
	}
	record.Input = readInt(messagesUsagePathInputTokens, record.Input)
	record.Output = readInt(messagesUsagePathOutputTokens, record.Output)
	record.CacheReadInputTokens = readInt(messagesUsagePathCacheReadInputTokens, record.CacheReadInputTokens)
	record.CacheWriteInputTokens = readInt(messagesUsagePathCacheWriteInputTokens, record.CacheWriteInputTokens)
	serviceTier, tierErr := messagesStringField(usage, messagesUsagePathServiceTier)
	usageErr = errors.Join(usageErr, tierErr)
	if serviceTier != "" {
		record.Metadata = recorder.Metadata{recorder.MetadataKeyServiceTier: serviceTier}
	}
	updateExtra := func(key, path string) {
		if count := readInt(path, 0); count > 0 {
			if record.ExtraTokenTypes == nil {
				record.ExtraTokenTypes = make(map[string]int64)
			}
			record.ExtraTokenTypes[key] = count
		}
	}
	updateExtra("web_search_requests", messagesUsagePathWebSearchRequests)
	updateExtra("cache_ephemeral_1h_input", messagesUsagePathEphemeral1hInput)
	updateExtra("cache_ephemeral_5m_input", messagesUsagePathEphemeral5mInput)
	return usageErr
}

// messagesIntField retains the previous value if the field is absent or invalid.
func messagesIntField(parent gjson.Result, path string, previous int64) (int64, error) {
	field := parent.Get(path)
	if field.Type == gjson.Null {
		return previous, nil
	}
	if field.Type != gjson.Number {
		return previous, xerrors.Errorf("unexpected %s type: %s", path, field.Type)
	}
	value, err := strconv.ParseInt(field.Raw, 10, 64)
	if err != nil {
		return previous, xerrors.Errorf("invalid integer in %s", path)
	}
	return value, nil
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
