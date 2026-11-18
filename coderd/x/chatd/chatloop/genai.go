package chatloop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
)

// The OpenTelemetry GenAI semantic conventions are still in development.
// Span names and gen_ai.* attributes follow semconv v1.41.0, the last
// release that shipped them in the core registry.

// GenAIAgentName is the gen_ai.agent.name of Coder Agents.
const GenAIAgentName = "coder"

// Coder-specific span attribute keys that accompany the GenAI attributes.
const (
	// AttrStage carries the lifecycle stage on every stage span, since
	// GenAI spans are named after the operation instead of the stage.
	AttrStage          = "stage"
	AttrOrganizationID = "coder.organization.id"
	AttrWorkspaceID    = "coder.workspace.id"
	AttrParentChatID   = "coder.chat.parent_id"
	AttrRootChatID     = "coder.chat.root_id"
)

// MaxContentAttributeBytes bounds each captured content attribute so one
// large prompt or tool payload cannot exceed exporter message limits.
const MaxContentAttributeBytes = 64 << 10

// ChatIdentity attributes GenAI spans to a chat and its owner.
type ChatIdentity struct {
	ChatID         uuid.UUID
	OwnerID        uuid.UUID
	OrganizationID uuid.UUID
	WorkspaceID    uuid.NullUUID
	ParentChatID   uuid.NullUUID
	RootChatID     uuid.NullUUID
}

type chatIdentityKey struct{}

// ContextWithChatIdentity returns ctx carrying the chat identity that
// GenAI spans started on it record.
func ContextWithChatIdentity(ctx context.Context, identity ChatIdentity) context.Context {
	return context.WithValue(ctx, chatIdentityKey{}, identity)
}

func chatIdentityFromContext(ctx context.Context) (ChatIdentity, bool) {
	identity, ok := ctx.Value(chatIdentityKey{}).(ChatIdentity)
	return identity, ok && identity.ChatID != uuid.Nil
}

func (c ChatIdentity) attributes() []attribute.KeyValue {
	attrs := []attribute.KeyValue{semconv.GenAIConversationID(c.ChatID.String())}
	if c.OwnerID != uuid.Nil {
		attrs = append(attrs, semconv.UserID(c.OwnerID.String()))
	}
	if c.OrganizationID != uuid.Nil {
		attrs = append(attrs, attribute.String(AttrOrganizationID, c.OrganizationID.String()))
	}
	if c.WorkspaceID.Valid {
		attrs = append(attrs, attribute.String(AttrWorkspaceID, c.WorkspaceID.UUID.String()))
	}
	if c.ParentChatID.Valid {
		attrs = append(attrs, attribute.String(AttrParentChatID, c.ParentChatID.UUID.String()))
	}
	if c.RootChatID.Valid {
		attrs = append(attrs, attribute.String(AttrRootChatID, c.RootChatID.UUID.String()))
	}
	return attrs
}

// genAIOperation returns the GenAI operation a stage represents, if any.
func genAIOperation(stage Stage) (attribute.KeyValue, bool) {
	switch stage {
	case StageChatTurn:
		return semconv.GenAIOperationNameInvokeAgent, true
	case StageStream:
		return semconv.GenAIOperationNameChat, true
	case StageToolCall:
		return semconv.GenAIOperationNameExecuteTool, true
	default:
		return attribute.KeyValue{}, false
	}
}

// stageSpanStart returns the span name and start options for a stage.
// Sampling-relevant GenAI attributes and the span kind must be set at
// creation, since head samplers do not see later changes.
func stageSpanStart(ctx context.Context, stage Stage, model StageModel, toolName string) (string, []trace.SpanStartOption) {
	attrs := []attribute.KeyValue{attribute.String(AttrStage, string(stage))}
	operation, ok := genAIOperation(stage)
	if !ok {
		return string(stage), []trace.SpanStartOption{trace.WithAttributes(attrs...)}
	}
	attrs = append(attrs, operation)
	if identity, ok := chatIdentityFromContext(ctx); ok {
		attrs = append(attrs, identity.attributes()...)
	}
	name := operation.Value.AsString()
	opts := []trace.SpanStartOption{}
	switch stage {
	case StageChatTurn:
		name += " " + GenAIAgentName
		attrs = append(attrs, semconv.GenAIAgentName(GenAIAgentName))
	case StageStream:
		// Models are called over HTTP, so inference spans are clients.
		opts = append(opts, trace.WithSpanKind(trace.SpanKindClient))
		if model.Model != "" {
			name += " " + model.Model
		}
		attrs = append(attrs, semconv.GenAIRequestStream(true))
		attrs = append(attrs, genAIModelAttributes(model)...)
	case StageToolCall:
		if toolName != "" {
			name += " " + toolName
		}
	}
	return name, append(opts, trace.WithAttributes(attrs...))
}

// genAIProviderName maps a configured provider type, or the wire protocol
// when the type is unknown, to gen_ai.provider.name.
func genAIProviderName(model StageModel) string {
	switch model.ProviderType {
	case "openai":
		return semconv.GenAIProviderNameOpenAI.Value.AsString()
	case "anthropic":
		return semconv.GenAIProviderNameAnthropic.Value.AsString()
	case "azure":
		return semconv.GenAIProviderNameAzureAIOpenAI.Value.AsString()
	case "bedrock":
		return semconv.GenAIProviderNameAWSBedrock.Value.AsString()
	case "google":
		return semconv.GenAIProviderNameGCPGemini.Value.AsString()
	case "":
		return model.Provider
	default:
		return model.ProviderType
	}
}

func genAIModelAttributes(model StageModel) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 2)
	if provider := genAIProviderName(model); provider != "" {
		attrs = append(attrs, semconv.GenAIProviderNameKey.String(provider))
	}
	if model.Model != "" {
		attrs = append(attrs, semconv.GenAIRequestModel(model.Model))
	}
	return attrs
}

// errorType returns error.type for a failed stage.
func errorType(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if kind := chaterror.Classify(err).Kind; kind != "" {
		return string(kind)
	}
	return "_OTHER"
}

// genAIToolType returns gen_ai.tool.type: "function" for tools Coder
// Agents implements and "extension" for tools served by MCP servers or
// callers.
func genAIToolType(builtinToolNames map[string]bool, name string) string {
	if builtinToolNames[name] {
		return "function"
	}
	return "extension"
}

func genAIToolAttributes(call fantasy.ToolCallContent, toolType string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{semconv.GenAIToolName(call.ToolName)}
	if call.ToolCallID != "" {
		attrs = append(attrs, semconv.GenAIToolCallID(call.ToolCallID))
	}
	if toolType != "" {
		attrs = append(attrs, semconv.GenAIToolType(toolType))
	}
	return attrs
}

func (s *StageSpan) setToolArguments(input string) {
	if s.capturesContent() && input != "" {
		s.span.SetAttributes(attribute.String(string(semconv.GenAIToolCallArgumentsKey), jsonAttributeValue(input)))
	}
}

// SetToolResult records the tool result when content capture is enabled.
func (s *StageSpan) SetToolResult(result fantasy.ToolResultContent) {
	if !s.capturesContent() || result.Result == nil {
		return
	}
	s.span.SetAttributes(attribute.String(string(semconv.GenAIToolCallResultKey), jsonAttributeValue(toolResultText(result.Result))))
}

// SetInferenceRequest records request attributes of a chat span. With
// content capture enabled it records the messages added since the last
// assistant message, so each span carries only its own step's input.
func (s *StageSpan) SetInferenceRequest(call fantasy.Call) {
	if s == nil || s.ended {
		return
	}
	if call.MaxOutputTokens != nil {
		s.span.SetAttributes(semconv.GenAIRequestMaxTokens(int(*call.MaxOutputTokens)))
	}
	if call.Temperature != nil {
		s.span.SetAttributes(semconv.GenAIRequestTemperature(*call.Temperature))
	}
	if !s.capturesContent() {
		return
	}
	if encoded, ok := encodeMessages(stepInputMessages(call.Prompt)); ok {
		s.span.SetAttributes(attribute.String(string(semconv.GenAIInputMessagesKey), encoded))
	}
}

// InferenceResponse is the outcome of one chat span.
type InferenceResponse struct {
	ResponseID   string
	FinishReason fantasy.FinishReason
	Usage        fantasy.Usage
	Content      []fantasy.Content
}

// SetInferenceResponse records response attributes of a chat span.
func (s *StageSpan) SetInferenceResponse(resp InferenceResponse) {
	if s == nil || s.ended {
		return
	}
	attrs := make([]attribute.KeyValue, 0, 8)
	if resp.ResponseID != "" {
		attrs = append(attrs, semconv.GenAIResponseID(resp.ResponseID))
	}
	attrs = append(attrs, semconv.GenAIResponseFinishReasons(genAIFinishReason(resp.FinishReason)))
	attrs = append(attrs, usageAttributes(resp.Usage)...)
	s.span.SetAttributes(attrs...)
	if !s.capturesContent() || len(resp.Content) == 0 {
		return
	}
	msg := genAIMessage{
		Role:         string(fantasy.MessageRoleAssistant),
		Parts:        contentParts(resp.Content),
		FinishReason: genAIFinishReason(resp.FinishReason),
	}
	if encoded, ok := encodeMessages([]genAIMessage{msg}); ok {
		s.span.SetAttributes(attribute.String(string(semconv.GenAIOutputMessagesKey), encoded))
	}
}

// genAIFinishReason maps a fantasy finish reason to the GenAI finish
// reason enum. The output message schema requires one, and the enum has
// no value for a missing or unclassified reason. GenerateAssistant marks
// every failed call as error, so those map to stop.
func genAIFinishReason(reason fantasy.FinishReason) string {
	switch reason {
	case fantasy.FinishReasonLength:
		return "length"
	case fantasy.FinishReasonToolCalls:
		return "tool_call"
	case fantasy.FinishReasonContentFilter:
		return "content_filter"
	case fantasy.FinishReasonError:
		return "error"
	default:
		return "stop"
	}
}

// usageAttributes reports input tokens including cache reads and writes,
// as the conventions require; fantasy reports them separately.
func usageAttributes(usage fantasy.Usage) []attribute.KeyValue {
	if usage == (fantasy.Usage{}) {
		return nil
	}
	input := usage.InputTokens + usage.CacheReadTokens + usage.CacheCreationTokens
	attrs := []attribute.KeyValue{
		semconv.GenAIUsageInputTokens(int(input)),
		semconv.GenAIUsageOutputTokens(int(usage.OutputTokens)),
		semconv.GenAIUsageCacheReadInputTokens(int(usage.CacheReadTokens)),
		semconv.GenAIUsageCacheCreationInputTokens(int(usage.CacheCreationTokens)),
	}
	if usage.ReasoningTokens != 0 {
		attrs = append(attrs, semconv.GenAIUsageReasoningOutputTokens(int(usage.ReasoningTokens)))
	}
	return attrs
}

func (s *StageSpan) capturesContent() bool {
	return s != nil && !s.ended && s.tracer != nil && s.tracer.captureContent && s.span.IsRecording()
}

// genAIMessage and genAIPart follow the GenAI input and output message
// JSON schemas.
type genAIMessage struct {
	Role         string      `json:"role"`
	Parts        []genAIPart `json:"parts"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

type genAIPart struct {
	Type      string          `json:"type"`
	Content   string          `json:"content,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Response  string          `json:"response,omitempty"`
}

// MarshalJSON keeps the fields each part type requires, even when they
// are empty.
func (p genAIPart) MarshalJSON() ([]byte, error) {
	switch p.Type {
	case "text", "reasoning":
		return json.Marshal(struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		}{p.Type, p.Content})
	case "tool_call_response":
		return json.Marshal(struct {
			Type     string `json:"type"`
			ID       string `json:"id,omitempty"`
			Response string `json:"response"`
		}{p.Type, p.ID, p.Response})
	default:
		type plain genAIPart
		return json.Marshal(plain(p))
	}
}

// stepInputMessages returns the non-system messages after the last
// assistant message: the user prompt or the tool results the step
// responds to.
func stepInputMessages(prompt []fantasy.Message) []genAIMessage {
	start := 0
	for i := len(prompt) - 1; i >= 0; i-- {
		if prompt[i].Role == fantasy.MessageRoleAssistant {
			start = i + 1
			break
		}
	}
	messages := make([]genAIMessage, 0, len(prompt)-start)
	for _, msg := range prompt[start:] {
		if msg.Role == fantasy.MessageRoleSystem {
			continue
		}
		parts := make([]genAIPart, 0, len(msg.Content))
		for _, part := range msg.Content {
			if p, ok := messagePart(part); ok {
				parts = append(parts, p)
			}
		}
		messages = append(messages, genAIMessage{Role: string(msg.Role), Parts: parts})
	}
	return messages
}

func messagePart(part fantasy.MessagePart) (genAIPart, bool) {
	switch p := part.(type) {
	case fantasy.TextPart:
		return genAIPart{Type: "text", Content: p.Text}, true
	case fantasy.ReasoningPart:
		return genAIPart{Type: "reasoning", Content: p.Text}, true
	case fantasy.ToolCallPart:
		if p.ProviderExecuted {
			// Server tool parts require provider-specific details that
			// fantasy does not keep.
			return genAIPart{}, false
		}
		return toolCallPart(p.ToolCallID, p.ToolName, p.Input), true
	case fantasy.ToolResultPart:
		if p.ProviderExecuted {
			return genAIPart{}, false
		}
		return genAIPart{Type: "tool_call_response", ID: p.ToolCallID, Response: toolResultText(p.Output)}, true
	default:
		// File parts are left out: the schema requires their bytes.
		return genAIPart{}, false
	}
}

func contentParts(content []fantasy.Content) []genAIPart {
	parts := make([]genAIPart, 0, len(content))
	for _, c := range content {
		switch p := c.(type) {
		case fantasy.TextContent:
			parts = append(parts, genAIPart{Type: "text", Content: p.Text})
		case fantasy.ReasoningContent:
			parts = append(parts, genAIPart{Type: "reasoning", Content: p.Text})
		case fantasy.ToolCallContent:
			// Server tool parts require provider-specific details that
			// fantasy does not keep.
			if !p.ProviderExecuted {
				parts = append(parts, toolCallPart(p.ToolCallID, p.ToolName, p.Input))
			}
		case fantasy.ToolResultContent:
			if p.Result != nil && !p.ProviderExecuted {
				parts = append(parts, genAIPart{Type: "tool_call_response", ID: p.ToolCallID, Response: toolResultText(p.Result)})
			}
		}
		// File and source content is left out: the schema requires file
		// bytes and a modality that citations do not have.
	}
	return parts
}

func toolCallPart(id, name, input string) genAIPart {
	part := genAIPart{Type: "tool_call", ID: id, Name: name}
	if json.Valid([]byte(input)) {
		part.Arguments = json.RawMessage(input)
	} else if input != "" {
		if encoded, err := json.Marshal(input); err == nil {
			part.Arguments = encoded
		}
	}
	return part
}

// toolResultText omits media bytes, keeping any accompanying text.
func toolResultText(output fantasy.ToolResultOutputContent) string {
	switch o := output.(type) {
	case fantasy.ToolResultOutputContentText:
		return o.Text
	case fantasy.ToolResultOutputContentError:
		if o.Error == nil {
			return ""
		}
		return o.Error.Error()
	case fantasy.ToolResultOutputContentMedia:
		return o.Text
	default:
		return ""
	}
}

// encodeMessages marshals messages within MaxContentAttributeBytes. When
// they do not fit, it clips every part's text to a shrinking budget so the
// value stays valid JSON.
func encodeMessages(messages []genAIMessage) (string, bool) {
	if len(messages) == 0 {
		return "", false
	}
	encoded, err := json.Marshal(messages)
	if err != nil {
		return "", false
	}
	if len(encoded) <= MaxContentAttributeBytes {
		return string(encoded), true
	}
	parts := 0
	for _, msg := range messages {
		parts += len(msg.Parts)
	}
	// JSON escaping can expand text, so halve the budget until it fits.
	for budget := MaxContentAttributeBytes / max(parts, 1); budget > 0; budget /= 2 {
		encoded, err = json.Marshal(clipMessages(messages, budget))
		if err != nil {
			return "", false
		}
		if len(encoded) <= MaxContentAttributeBytes {
			return string(encoded), true
		}
	}
	return "", false
}

func clipMessages(messages []genAIMessage, budget int) []genAIMessage {
	clipped := make([]genAIMessage, len(messages))
	for i, msg := range messages {
		clipped[i] = msg
		clipped[i].Parts = make([]genAIPart, len(msg.Parts))
		for j, part := range msg.Parts {
			part.Content = truncateBytes(part.Content, budget)
			part.Response = truncateBytes(part.Response, budget)
			if len(part.Arguments) > budget {
				// Clipped arguments are no longer a JSON object, so they
				// are recorded as a string.
				if encoded, err := json.Marshal(truncateBytes(string(part.Arguments), budget)); err == nil {
					part.Arguments = encoded
				} else {
					part.Arguments = nil
				}
			}
			clipped[i].Parts[j] = part
		}
	}
	return clipped
}

// jsonAttributeValue returns s when it is JSON that fits. Otherwise it
// returns a JSON string of s, or of a prefix that fits, since the
// conventions record tool payloads as serialized JSON on spans.
func jsonAttributeValue(s string) string {
	if len(s) <= MaxContentAttributeBytes && json.Valid([]byte(s)) {
		return s
	}
	// JSON escaping can expand text, so halve the budget until it fits.
	for budget := MaxContentAttributeBytes; budget > 0; budget /= 2 {
		encoded, err := json.Marshal(truncateBytes(s, budget))
		if err == nil && len(encoded) <= MaxContentAttributeBytes {
			return string(encoded)
		}
	}
	return `""`
}

// truncateBytes cuts s to at most limit bytes on a rune boundary.
func truncateBytes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.ToValidUTF8(s[:cut], "\uFFFD")
}
