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

// stageSpanStart returns the span name and GenAI attributes for a stage.
// Inference and tool spans are renamed once the model or tool is known.
func stageSpanStart(ctx context.Context, stage Stage) (string, []attribute.KeyValue) {
	attrs := []attribute.KeyValue{attribute.String(AttrStage, string(stage))}
	operation, ok := genAIOperation(stage)
	if !ok {
		return string(stage), attrs
	}
	attrs = append(attrs, operation)
	if identity, ok := chatIdentityFromContext(ctx); ok {
		attrs = append(attrs, identity.attributes()...)
	}
	name := operation.Value.AsString()
	if stage == StageChatTurn {
		name += " " + GenAIAgentName
		attrs = append(attrs, semconv.GenAIAgentName(GenAIAgentName))
	}
	return name, attrs
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

// SetTool names an execute_tool span after the tool it runs.
func (s *StageSpan) SetTool(call fantasy.ToolCallContent, toolType string) {
	if s == nil || s.ended {
		return
	}
	s.span.SetName(semconv.GenAIOperationNameExecuteTool.Value.AsString() + " " + call.ToolName)
	attrs := []attribute.KeyValue{semconv.GenAIToolName(call.ToolName)}
	if call.ToolCallID != "" {
		attrs = append(attrs, semconv.GenAIToolCallID(call.ToolCallID))
	}
	if toolType != "" {
		attrs = append(attrs, semconv.GenAIToolType(toolType))
	}
	s.span.SetAttributes(attrs...)
	if s.capturesContent() && call.Input != "" {
		s.span.SetAttributes(attribute.String(string(semconv.GenAIToolCallArgumentsKey), truncateContent(call.Input)))
	}
}

// SetToolResult records the tool result when content capture is enabled.
func (s *StageSpan) SetToolResult(result fantasy.ToolResultContent) {
	if !s.capturesContent() || result.Result == nil {
		return
	}
	s.span.SetAttributes(attribute.String(string(semconv.GenAIToolCallResultKey), truncateContent(toolResultText(result.Result))))
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
	if resp.FinishReason != "" {
		attrs = append(attrs, semconv.GenAIResponseFinishReasons(string(resp.FinishReason)))
	}
	attrs = append(attrs, usageAttributes(resp.Usage)...)
	s.span.SetAttributes(attrs...)
	if !s.capturesContent() || len(resp.Content) == 0 {
		return
	}
	msg := genAIMessage{
		Role:         string(fantasy.MessageRoleAssistant),
		Parts:        contentParts(resp.Content),
		FinishReason: string(resp.FinishReason),
	}
	if encoded, ok := encodeMessages([]genAIMessage{msg}); ok {
		s.span.SetAttributes(attribute.String(string(semconv.GenAIOutputMessagesKey), encoded))
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
	MimeType  string          `json:"mime_type,omitempty"`
	URI       string          `json:"uri,omitempty"`
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
	case fantasy.FilePart:
		// File bytes are omitted; the media type identifies the attachment.
		return genAIPart{Type: "blob", MimeType: p.MediaType}, true
	case fantasy.ToolCallPart:
		return toolCallPart(p.ToolCallID, p.ToolName, p.Input), true
	case fantasy.ToolResultPart:
		return genAIPart{Type: "tool_call_response", ID: p.ToolCallID, Response: toolResultText(p.Output)}, true
	default:
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
			parts = append(parts, toolCallPart(p.ToolCallID, p.ToolName, p.Input))
		case fantasy.ToolResultContent:
			if p.Result != nil {
				parts = append(parts, genAIPart{Type: "tool_call_response", ID: p.ToolCallID, Response: toolResultText(p.Result)})
			}
		case fantasy.FileContent:
			parts = append(parts, genAIPart{Type: "blob", MimeType: p.MediaType})
		case fantasy.SourceContent:
			parts = append(parts, genAIPart{Type: "uri", URI: p.URL, MimeType: p.MediaType})
		}
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

func encodeMessages(messages []genAIMessage) (string, bool) {
	if len(messages) == 0 {
		return "", false
	}
	encoded, err := json.Marshal(messages)
	if err != nil {
		return "", false
	}
	return truncateContent(string(encoded)), true
}

// truncateContent cuts s to MaxContentAttributeBytes on a rune boundary.
// Truncated JSON is no longer valid; consumers should treat the value as
// text once it reaches the limit.
func truncateContent(s string) string {
	if len(s) <= MaxContentAttributeBytes {
		return s
	}
	cut := MaxContentAttributeBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.ToValidUTF8(s[:cut], "\uFFFD")
}
