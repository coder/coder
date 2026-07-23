package messages

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/xerrors"
)

const (
	// Absolute JSON paths from the request root.
	messagesReqPathMessages                  = "messages"
	messagesReqPathMaxTokens                 = "max_tokens"
	messagesReqPathModel                     = "model"
	messagesReqPathOutputConfig              = "output_config"
	messagesReqPathOutputConfigEffort        = "output_config.effort"
	messagesReqPathOutputConfigFormat        = "output_config.format"
	messagesReqPathMetadata                  = "metadata"
	messagesReqPathServiceTier               = "service_tier"
	messagesReqPathContainer                 = "container"
	messagesReqPathInferenceGeo              = "inference_geo"
	messagesReqPathContextManagement         = "context_management"
	messagesReqPathStream                    = "stream"
	messagesReqPathThinking                  = "thinking"
	messagesReqPathThinkingBudgetTokens      = "thinking.budget_tokens"
	messagesReqPathThinkingType              = "thinking.type"
	messagesReqPathToolChoice                = "tool_choice"
	messagesReqPathToolChoiceDisableParallel = "tool_choice.disable_parallel_tool_use"
	messagesReqPathToolChoiceType            = "tool_choice.type"
	messagesReqPathTools                     = "tools"

	// Relative field names used within sub-objects.
	messagesReqFieldContent   = "content"
	messagesReqFieldRole      = "role"
	messagesReqFieldText      = "text"
	messagesReqFieldToolUseID = "tool_use_id"
	messagesReqFieldType      = "type"
)

const (
	constAdaptive = "adaptive"
	constDisabled = "disabled"
	constEnabled  = "enabled"
)

var (
	constAny        = string(constant.ValueOf[constant.Any]())
	constAuto       = string(constant.ValueOf[constant.Auto]())
	constNone       = string(constant.ValueOf[constant.None]())
	constText       = string(constant.ValueOf[constant.Text]())
	constTool       = string(constant.ValueOf[constant.Tool]())
	constToolResult = string(constant.ValueOf[constant.ToolResult]())
	constUser       = string(anthropic.MessageParamRoleUser)
	constSystem     = "system"

	// bedrockUnsupportedFields are top-level fields present in the Anthropic Messages
	// API that are absent from the Bedrock request body schema. Sending them results
	// in a 400 "Extra inputs are not permitted" error.
	//
	// Anthropic API fields: https://platform.claude.com/docs/en/api/messages/create
	// Bedrock request body: https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-anthropic-claude-messages-request-response.html
	bedrockUnsupportedFields = []string{
		messagesReqPathMetadata,
		messagesReqPathServiceTier,
		messagesReqPathContainer,
		messagesReqPathInferenceGeo,
	}

	// bedrockBetaGatedFields maps body fields to the beta flag that enables them.
	// If the beta flag is present in the (already-filtered) Anthropic-Beta header,
	// the field is kept; otherwise it is stripped. Model-specific beta flags must
	// be removed from the header before this check (see filterBedrockBetaFlags).
	// Adaptive-only models (Opus 4.7+) are exempt for output_config since they
	// support it natively without a beta flag, see
	// bedrockModelRequiresAdaptiveThinking.
	bedrockBetaGatedFields = map[string]string{
		// output_config requires the effort beta (Opus 4.5 only).
		messagesReqPathOutputConfig: "effort-2025-11-24",
		// context_management requires the context-management beta (Sonnet 4.5, Haiku 4.5).
		messagesReqPathContextManagement: "context-management-2025-06-27",
	}
)

// RequestPayload is raw JSON bytes of an Anthropic Messages API request.
// Methods provide package-specific reads and rewrites while preserving the
// original body for upstream pass-through.
type RequestPayload []byte

func NewRequestPayload(raw []byte) (RequestPayload, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, xerrors.New("messages empty request body")
	}
	if !json.Valid(raw) {
		return nil, xerrors.New("messages invalid JSON request body")
	}

	return RequestPayload(raw), nil
}

func (p RequestPayload) Stream() bool {
	v := gjson.GetBytes(p, messagesReqPathStream)
	if !v.IsBool() {
		return false
	}
	return v.Bool()
}

func (p RequestPayload) model() string {
	return gjson.GetBytes(p, messagesReqPathModel).Str
}

func (p RequestPayload) correlatingToolCallID() *string {
	messages := gjson.GetBytes(p, messagesReqPathMessages)
	if !messages.IsArray() {
		return nil
	}

	messageItems := messages.Array()
	if len(messageItems) == 0 {
		return nil
	}

	content := messageItems[len(messageItems)-1].Get(messagesReqFieldContent)
	if !content.IsArray() {
		return nil
	}

	contentItems := content.Array()
	for idx := len(contentItems) - 1; idx >= 0; idx-- {
		contentItem := contentItems[idx]
		if contentItem.Get(messagesReqFieldType).String() != constToolResult {
			continue
		}

		toolUseID := contentItem.Get(messagesReqFieldToolUseID).String()
		if toolUseID == "" {
			continue
		}

		return &toolUseID
	}

	return nil
}

// lastUserPrompt returns the prompt text from the last user message. If no prompt
// is found, it returns empty string, false, nil. Unexpected shapes are treated as
// unsupported and do not fail the request path.
func (p RequestPayload) lastUserPrompt() (string, bool, error) {
	messages := gjson.GetBytes(p, messagesReqPathMessages)
	if !messages.Exists() || messages.Type == gjson.Null {
		return "", false, nil
	}
	if !messages.IsArray() {
		return "", false, xerrors.Errorf("unexpected messages type: %s", messages.Type)
	}

	messageItems := messages.Array()
	if len(messageItems) == 0 {
		return "", false, nil
	}

	lastMessage := messageItems[len(messageItems)-1]
	// Clients using the mid-conversation system beta (e.g. Claude Code with
	// anthropic-beta: mid-conversation-system-*) append a trailing role=system
	// message after the user's prompt, such as an injected skills list. When the
	// last message is that system message, step back exactly one message to find
	// the user's prompt. We only step back past a single trailing system message
	// so we never re-record a stale prompt from an earlier turn that contained no
	// new user input. See https://docs.claude.com/en/api/beta-headers.
	if lastMessage.Get(messagesReqFieldRole).String() == constSystem && len(messageItems) >= 2 {
		lastMessage = messageItems[len(messageItems)-2]
	}
	if lastMessage.Get(messagesReqFieldRole).String() != constUser {
		return "", false, nil
	}

	content := lastMessage.Get(messagesReqFieldContent)
	if !content.Exists() || content.Type == gjson.Null {
		return "", false, nil
	}
	if content.Type == gjson.String {
		return content.String(), true, nil
	}
	if !content.IsArray() {
		return "", false, xerrors.Errorf("unexpected message content type: %s", content.Type)
	}

	contentItems := content.Array()
	for idx := len(contentItems) - 1; idx >= 0; idx-- {
		contentItem := contentItems[idx]
		if contentItem.Get(messagesReqFieldType).String() != constText {
			continue
		}

		text := contentItem.Get(messagesReqFieldText)
		if text.Type != gjson.String {
			continue
		}

		return text.String(), true, nil
	}

	return "", false, nil
}

func (p RequestPayload) injectTools(injected []anthropic.ToolUnionParam) (RequestPayload, error) {
	if len(injected) == 0 {
		return p, nil
	}

	existing, err := p.tools()
	if err != nil {
		return p, xerrors.Errorf("get existing tools: %w", err)
	}

	// Using []json.Marshaler to merge differently-typed slices ([]anthropic.ToolUnionParam
	// and []json.Marshaler containing json.RawMessage) keeps JSON re-marshalings to a minimum:
	// sjson.SetBytes marshals each element exactly once, and json.RawMessage
	// elements are passed through without re-serialization.
	allTools := make([]json.Marshaler, 0, len(injected)+len(existing))
	for _, tool := range injected {
		allTools = append(allTools, tool)
	}

	for _, e := range existing {
		allTools = append(allTools, e)
	}

	return p.set(messagesReqPathTools, allTools)
}

func (p RequestPayload) disableParallelToolCalls() (RequestPayload, error) {
	toolChoice := gjson.GetBytes(p, messagesReqPathToolChoice)

	// If no tool_choice was defined, assume auto.
	// See https://platform.claude.com/docs/en/agents-and-tools/tool-use/implement-tool-use#parallel-tool-use.
	if !toolChoice.Exists() || toolChoice.Type == gjson.Null {
		updated, err := p.set(messagesReqPathToolChoiceType, constAuto)
		if err != nil {
			return p, xerrors.Errorf("set tool choice type: %w", err)
		}
		return updated.set(messagesReqPathToolChoiceDisableParallel, true)
	}
	if !toolChoice.IsObject() {
		return p, xerrors.Errorf("unsupported tool_choice type: %s", toolChoice.Type)
	}

	toolChoiceType := gjson.GetBytes(p, messagesReqPathToolChoiceType)
	if toolChoiceType.Exists() && toolChoiceType.Type != gjson.String {
		return p, xerrors.Errorf("unsupported tool_choice.type type: %s", toolChoiceType.Type)
	}

	switch toolChoiceType.String() {
	case "":
		updated, err := p.set(messagesReqPathToolChoiceType, constAuto)
		if err != nil {
			return p, xerrors.Errorf("set tool_choice.type: %w", err)
		}
		return updated.set(messagesReqPathToolChoiceDisableParallel, true)
	case constAuto, constAny, constTool:
		return p.set(messagesReqPathToolChoiceDisableParallel, true)
	case constNone:
		return p, nil
	default:
		return p, xerrors.Errorf("unsupported tool_choice.type value: %q", toolChoiceType.String())
	}
}

func (p RequestPayload) appendedMessages(newMessages []anthropic.MessageParam) (RequestPayload, error) {
	if len(newMessages) == 0 {
		return p, nil
	}

	existing, err := p.messages()
	if err != nil {
		return p, xerrors.Errorf("get existing messages: %w", err)
	}

	// Using []json.Marshaler to merge differently-typed slices ([]json.Marshaler containing
	// json.RawMessage and []anthropic.MessageParam) keeps JSON re-marshalings
	// to a minimum: sjson.SetBytes marshals each element exactly once, and
	// json.RawMessage elements are passed through without re-serialization.
	allMessages := make([]json.Marshaler, 0, len(existing)+len(newMessages))

	for _, e := range existing {
		allMessages = append(allMessages, e)
	}

	for _, new := range newMessages {
		allMessages = append(allMessages, new)
	}

	return p.set(messagesReqPathMessages, allMessages)
}

func (p RequestPayload) withModel(model string) (RequestPayload, error) {
	return p.set(messagesReqPathModel, model)
}

func (p RequestPayload) messages() ([]json.RawMessage, error) {
	messages := gjson.GetBytes(p, messagesReqPathMessages)
	if !messages.Exists() || messages.Type == gjson.Null {
		return nil, nil
	}
	if !messages.IsArray() {
		return nil, xerrors.Errorf("unsupported messages type: %s", messages.Type)
	}

	return p.resultToRawMessage(messages.Array()), nil
}

func (p RequestPayload) tools() ([]json.RawMessage, error) {
	tools := gjson.GetBytes(p, messagesReqPathTools)
	if !tools.Exists() || tools.Type == gjson.Null {
		return nil, nil
	}
	if !tools.IsArray() {
		return nil, xerrors.Errorf("unsupported tools type: %s", tools.Type)
	}

	return p.resultToRawMessage(tools.Array()), nil
}

func (RequestPayload) resultToRawMessage(items []gjson.Result) []json.RawMessage {
	// gjson.Result conversion to json.RawMessage is needed because
	// gjson.Result does not implement json.Marshaler. It would
	// serialize its struct fields instead of the raw JSON it represents.
	rawMessages := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		rawMessages = append(rawMessages, json.RawMessage(item.Raw))
	}
	return rawMessages
}

// The two Bedrock thinking-type conversions below are a temporary shim.
// AI Gateway relays the Anthropic Messages API shape to Bedrock, whose Claude
// models accept a disjoint subset on each generation (older models reject
// "adaptive"; Opus 4.7+ rejects "enabled"). A planned native Bedrock provider
// removes the impedance mismatch and lets us delete this whole block. Hopefully.

// bedrockThinkingEffortRatios maps an output_config.effort hint to the fraction
// of max_tokens to allocate as thinking budget. The mapping is a heuristic
// with no canonical source; ratios adapted from OpenRouter:
// https://openrouter.ai/docs/guides/best-practices/reasoning-tokens#reasoning-effort-level
var bedrockThinkingEffortRatios = map[string]float64{
	"low":    0.2,
	"medium": 0.5,
	"high":   0.8,
	"max":    0.95,
}

// bedrockThinkingDefaultEffortRatio is used when output_config.effort is
// absent or unrecognized. Kept as a separate const rather than a runtime
// lookup so a misnamed map key can't silently zero out the budget.
const bedrockThinkingDefaultEffortRatio = 0.8 // matches "high"

// convertAdaptiveThinkingForBedrock converts thinking.type "adaptive" to
// "enabled" with a calculated budget_tokens. Needed for Bedrock models that
// do not support the "adaptive" thinking.type.
//
// This direction has to invent a number, since "enabled" requires budget_tokens.
// We bias the budget by output_config.effort when present, since that's the
// only signal we have about caller intent.
func (p RequestPayload) convertAdaptiveThinkingForBedrock() (RequestPayload, error) {
	if gjson.GetBytes(p, messagesReqPathThinkingType).String() != constAdaptive {
		return p, nil
	}

	maxTokens := gjson.GetBytes(p, messagesReqPathMaxTokens).Int()
	if maxTokens <= 0 {
		// max_tokens is required by messages API
		return p, xerrors.New("max_tokens: field required")
	}

	ratio, ok := bedrockThinkingEffortRatios[gjson.GetBytes(p, messagesReqPathOutputConfigEffort).String()]
	if !ok {
		ratio = bedrockThinkingDefaultEffortRatio
	}

	// budget_tokens must be ≥ 1024 && < max_tokens. If the calculated budget
	// doesn't meet the minimum, disable thinking entirely rather than forcing
	// an artificially high budget that would starve the output.
	// https://platform.claude.com/docs/en/api/messages/create#create.thinking
	// https://platform.claude.com/docs/en/build-with-claude/extended-thinking#how-to-use-extended-thinking
	budgetTokens := int64(float64(maxTokens) * ratio)
	if budgetTokens < 1024 {
		return p.set(messagesReqPathThinking, map[string]string{"type": constDisabled})
	}

	return p.set(messagesReqPathThinking, map[string]any{
		"type":          constEnabled,
		"budget_tokens": budgetTokens,
	})
}

// convertEnabledThinkingForBedrock rewrites thinking.type "enabled" to plain
// "adaptive", dropping budget_tokens. Needed for Bedrock models that only
// support adaptive thinking (Opus 4.7+).
//
// We deliberately do not derive output_config.effort from the budget. Any
// such mapping would be invented (no canonical budget-to-effort relationship
// exists), and adaptive thinking already has well-defined platform behavior
// when no effort hint is provided. An explicit output_config.effort from the
// caller is preserved naturally because we never touch that field.
//
// See https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-opus-4-7.html
// and https://docs.aws.amazon.com/bedrock/latest/userguide/claude-messages-adaptive-thinking.html
func (p RequestPayload) convertEnabledThinkingForBedrock() (RequestPayload, error) {
	if gjson.GetBytes(p, messagesReqPathThinkingType).String() != constEnabled {
		return p, nil
	}
	return p.set(messagesReqPathThinking, map[string]string{"type": constAdaptive})
}

// removeBedrockUnsupportedOutputConfigSubFields drops sub-fields of
// output_config that Bedrock rejects even on models where the parent
// output_config object is accepted. Adaptive-only models (Opus 4.7+) accept
// output_config.effort but reject output_config.format (structured outputs)
// with a 400 "Extra inputs are not permitted." The generic field-strip pass
// (removeUnsupportedBedrockFields) operates at top-level granularity only, so
// this targeted pass handles the sub-field case.
func (p RequestPayload) removeBedrockUnsupportedOutputConfigSubFields() (RequestPayload, error) {
	if !gjson.GetBytes(p, messagesReqPathOutputConfigFormat).Exists() {
		return p, nil
	}
	out, err := sjson.DeleteBytes(p, messagesReqPathOutputConfigFormat)
	if err != nil {
		return p, xerrors.Errorf("delete %s: %w", messagesReqPathOutputConfigFormat, err)
	}
	return RequestPayload(out), nil
}

// removeUnsupportedBedrockFields strips top-level fields that Bedrock does not
// support from the payload. Fields that are gated behind a beta flag are only
// removed when the corresponding flag is absent from the Anthropic-Beta header.
// Model-specific beta flags must already be filtered from the header before
// calling this method (see filterBedrockBetaFlags).
//
// Fields exempted by exemptFields are always kept regardless of beta flag
// state. Adaptive-only Bedrock models (Opus 4.7+) require output_config
// without a beta flag, so callers pass the field through this set to bypass
// the effort-2025-11-24 gate.
func (p RequestPayload) removeUnsupportedBedrockFields(headers http.Header, exemptFields ...string) (RequestPayload, error) {
	var payloadMap map[string]any
	if err := json.Unmarshal(p, &payloadMap); err != nil {
		return p, xerrors.Errorf("failed to unmarshal request payload when removing unsupported Bedrock fields: %w", err)
	}

	// Always strip unconditionally unsupported fields.
	for _, field := range bedrockUnsupportedFields {
		delete(payloadMap, field)
	}

	// Strip beta-gated fields only when their beta flag is missing and the
	// caller has not exempted them for the current model.
	betaValues := headers.Values("Anthropic-Beta")
	for field, requiredFlag := range bedrockBetaGatedFields {
		if slices.Contains(exemptFields, field) {
			continue
		}
		if !slices.Contains(betaValues, requiredFlag) {
			delete(payloadMap, field)
		}
	}

	result, err := json.Marshal(payloadMap)
	if err != nil {
		return p, xerrors.Errorf("failed to marshal request payload when removing unsupported Bedrock fields: %w", err)
	}
	return RequestPayload(result), nil
}

func (p RequestPayload) set(path string, value any) (RequestPayload, error) {
	out, err := sjson.SetBytes(p, path, value)
	if err != nil {
		return p, xerrors.Errorf("set %s: %w", path, err)
	}
	return RequestPayload(out), nil
}
