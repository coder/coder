package chatloop

import (
	"context"
	"errors"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
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
	case StageStream, StageModelCall:
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
	case StageStream, StageModelCall:
		// Models are called over HTTP, so inference spans are clients.
		opts = append(opts, trace.WithSpanKind(trace.SpanKindClient))
		if model.Model != "" {
			name += " " + model.Model
		}
		if stage == StageStream {
			attrs = append(attrs, semconv.GenAIRequestStream(true))
		}
		attrs = append(attrs, genAIModelAttributes(model)...)
	case StageToolCall:
		if toolName != "" {
			name += " " + toolName
		}
	}
	return name, append(opts, trace.WithAttributes(attrs...))
}

// genAIProviderName maps a configured provider type, or the wire protocol
// when the type is unknown, to gen_ai.provider.name. Types without a
// well-known value in the conventions pass through as their own name.
// TestGenAIProviderNameCoversEveryProviderType fails when a provider type
// is added, so this mapping is revisited.
func genAIProviderName(model StageModel) string {
	if model.Bedrock {
		return semconv.GenAIProviderNameAWSBedrock.Value.AsString()
	}
	switch database.AIProviderType(model.ProviderType) {
	case database.AIProviderTypeOpenai:
		return semconv.GenAIProviderNameOpenAI.Value.AsString()
	case database.AIProviderTypeAnthropic:
		return semconv.GenAIProviderNameAnthropic.Value.AsString()
	case database.AIProviderTypeAzure:
		return semconv.GenAIProviderNameAzureAIOpenAI.Value.AsString()
	case database.AIProviderTypeBedrock:
		return semconv.GenAIProviderNameAWSBedrock.Value.AsString()
	case database.AIProviderTypeGoogle:
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

// SetGenAIProvider records gen_ai.provider.name. invoke_agent spans
// require it but start before the turn resolves its model.
func (s *StageSpan) SetGenAIProvider(model StageModel) {
	if s == nil || s.ended {
		return
	}
	if provider := genAIProviderName(model); provider != "" {
		s.span.SetAttributes(semconv.GenAIProviderNameKey.String(provider))
	}
}

// errToolResult marks a tool call that returned an error result, such as
// an unreachable MCP server. The result text is not recorded because it
// can contain tool output.
var errToolResult = xerrors.New("tool returned an error result")

// toolSpanError is the error an execute_tool span ends with: the run
// error, or errToolResult when the tool reported a failure as its result.
func toolSpanError(execErr error, result fantasy.ToolResultContent) error {
	if execErr != nil {
		return execErr
	}
	if _, ok := result.Result.(fantasy.ToolResultOutputContentError); ok {
		return errToolResult
	}
	return nil
}

// errorType returns error.type for a failed stage.
func errorType(err error) string {
	if errors.Is(err, errToolResult) {
		return "tool_error"
	}
	// A transport reset wraps context.Canceled but is a provider failure.
	if errors.Is(err, context.Canceled) && !errors.Is(err, chaterror.ErrProviderTransportReset) {
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

// SetInferenceRequest records request attributes of a chat span.
func (s *StageSpan) SetInferenceRequest(call fantasy.Call) {
	s.setRequestParams(call.MaxOutputTokens, call.Temperature)
}

func (s *StageSpan) setRequestParams(maxOutputTokens *int64, temperature *float64) {
	if s == nil || s.ended {
		return
	}
	if maxOutputTokens != nil {
		s.span.SetAttributes(semconv.GenAIRequestMaxTokens(int(*maxOutputTokens)))
	}
	if temperature != nil {
		s.span.SetAttributes(semconv.GenAIRequestTemperature(*temperature))
	}
}

// InferenceResponse is the outcome of one chat span.
type InferenceResponse struct {
	ResponseID   string
	FinishReason fantasy.FinishReason
	Usage        fantasy.Usage
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
	if reason, ok := genAIFinishReason(resp.FinishReason); ok {
		attrs = append(attrs, semconv.GenAIResponseFinishReasons(reason))
	}
	attrs = append(attrs, usageAttributes(resp.Usage)...)
	s.span.SetAttributes(attrs...)
}

// genAIFinishReason maps the finish reason the provider reported to its
// gen_ai.response.finish_reasons value. It reports false when the
// provider sent no reason or an unclassified one, so the span does not
// claim a reason the model never gave. A failed call is described by the
// span status and error.type, not by its finish reason.
func genAIFinishReason(reason fantasy.FinishReason) (string, bool) {
	switch reason {
	case fantasy.FinishReasonStop:
		return "stop", true
	case fantasy.FinishReasonLength:
		return "length", true
	case fantasy.FinishReasonToolCalls:
		return "tool_call", true
	case fantasy.FinishReasonContentFilter:
		return "content_filter", true
	case fantasy.FinishReasonError:
		return "error", true
	default:
		return "", false
	}
}

// usageAttributes reports input tokens including cache reads and writes,
// as the conventions require. Every client chatd builds reports them
// apart from InputTokens: Google providers use the OpenAI-compatible
// client, which subtracts cached tokens from the prompt count.
//
// The cache breakdown is recorded only when nonzero: the OpenAI-compatible
// client never reports cache writes, so a zero can mean "not reported".
//
// For Anthropic, fantasy reports the input counts of the first internal
// iteration, which measure context occupancy, so a response that ran
// provider tools such as web search undercounts its billed input.
func usageAttributes(usage fantasy.Usage) []attribute.KeyValue {
	if usage == (fantasy.Usage{}) {
		return nil
	}
	input := usage.InputTokens + usage.CacheReadTokens + usage.CacheCreationTokens
	attrs := []attribute.KeyValue{
		semconv.GenAIUsageInputTokens(int(input)),
		semconv.GenAIUsageOutputTokens(int(usage.OutputTokens)),
	}
	if usage.CacheReadTokens != 0 {
		attrs = append(attrs, semconv.GenAIUsageCacheReadInputTokens(int(usage.CacheReadTokens)))
	}
	if usage.CacheCreationTokens != 0 {
		attrs = append(attrs, semconv.GenAIUsageCacheCreationInputTokens(int(usage.CacheCreationTokens)))
	}
	if usage.ReasoningTokens != 0 {
		attrs = append(attrs, semconv.GenAIUsageReasoningOutputTokens(int(usage.ReasoningTokens)))
	}
	return attrs
}
