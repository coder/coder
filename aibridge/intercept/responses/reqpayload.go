package responses

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared/constant"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const (
	reqPathBackground        = "background"
	reqPathCallID            = "call_id"
	reqPathRole              = "role"
	reqPathInput             = "input"
	reqPathParallelToolCalls = "parallel_tool_calls"
	reqPathStream            = "stream"
	reqPathTools             = "tools"
)

var (
	constFunctionCallOutput = string(constant.ValueOf[constant.FunctionCallOutput]())
	constInputText          = string(constant.ValueOf[constant.InputText]())
	constUser               = string(constant.ValueOf[constant.User]())

	reqPathContent = string(constant.ValueOf[constant.Content]())
	reqPathModel   = string(constant.ValueOf[constant.Model]())
	reqPathText    = string(constant.ValueOf[constant.Text]())
	reqPathType    = string(constant.ValueOf[constant.Type]())
)

// RequestPayload is raw JSON bytes of a Responses API request.
// Methods provide package-specific reads and rewrites while preserving the
// original body for upstream pass-through.
// Note: No changes are made on schema error.
type RequestPayload []byte

func NewRequestPayload(raw []byte) (RequestPayload, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, xerrors.New("empty request body")
	}
	if !json.Valid(raw) {
		return nil, xerrors.New("invalid JSON payload")
	}

	return RequestPayload(raw), nil
}

func (p RequestPayload) Stream() bool {
	return gjson.GetBytes(p, reqPathStream).Bool()
}

func (p RequestPayload) model() string {
	return gjson.GetBytes(p, reqPathModel).String()
}

func (p RequestPayload) background() bool {
	return gjson.GetBytes(p, reqPathBackground).Bool()
}

func (p RequestPayload) correlatingToolCallID() *string {
	items := gjson.GetBytes(p, reqPathInput)
	if !items.IsArray() {
		return nil
	}

	arr := items.Array()
	if len(arr) == 0 {
		return nil
	}

	last := arr[len(arr)-1]
	if last.Get(reqPathType).String() != constFunctionCallOutput {
		return nil
	}

	callID := last.Get(reqPathCallID).String()
	if callID == "" {
		return nil
	}

	return &callID
}

// LastUserPrompt returns input text with the "user" role from the last input
// item, or the string input value if present. If no prompt is found, it returns
// empty string, false, nil. Unexpected shapes are treated as unsupported and do
// not fail the request path.
func (p RequestPayload) lastUserPrompt(ctx context.Context, logger slog.Logger) (string, bool, error) {
	inputItems := gjson.GetBytes(p, reqPathInput)
	if !inputItems.Exists() || inputItems.Type == gjson.Null {
		return "", false, nil
	}

	// 'input' can be either a string or an array of input items:
	// https://platform.openai.com/docs/api-reference/responses/create#responses_create-input

	// String variant: treat the whole input as the user prompt.
	if inputItems.Type == gjson.String {
		return inputItems.String(), true, nil
	}

	// Array variant: checking only the last input item
	if !inputItems.IsArray() {
		return "", false, xerrors.Errorf("unexpected input type: %s", inputItems.Type)
	}

	inputItemsArr := inputItems.Array()
	if len(inputItemsArr) == 0 {
		return "", false, nil
	}

	lastItem := inputItemsArr[len(inputItemsArr)-1]
	if lastItem.Get(reqPathRole).Str != constUser {
		// Request was likely not initiated by a prompt but is an iteration of agentic loop.
		return "", false, nil
	}

	// Message content can be either a string or an array of typed content items:
	// https://platform.openai.com/docs/api-reference/responses/create#responses_create-input-input_item_list-input_message-content
	content := lastItem.Get(reqPathContent)
	if !content.Exists() || content.Type == gjson.Null {
		return "", false, nil
	}

	// String variant: use it directly as the prompt.
	if content.Type == gjson.String {
		return content.Str, true, nil
	}

	if !content.IsArray() {
		return "", false, xerrors.Errorf("unexpected input content type: %s", content.Type)
	}

	var sb strings.Builder
	promptExists := false
	for _, c := range content.Array() {
		// Ignore non-text content blocks such as images or files.
		if c.Get(reqPathType).Str != constInputText {
			continue
		}

		text := c.Get(reqPathText)
		if text.Type != gjson.String {
			logger.Warn(ctx, fmt.Sprintf("unexpected input content array element text type: %v", text.Type))
			continue
		}

		if promptExists {
			_ = sb.WriteByte('\n') // strings.Builder.WriteByte never fails
		}
		promptExists = true
		_, _ = sb.WriteString(text.Str) // strings.Builder.WriteString never fails
	}

	if !promptExists {
		return "", false, nil
	}

	return sb.String(), true, nil
}

func (p RequestPayload) injectTools(injected []responses.ToolUnionParam) (RequestPayload, error) {
	if len(injected) == 0 {
		return p, nil
	}

	existing, err := p.toolItems()
	if err != nil {
		return p, xerrors.Errorf("failed to get existing tools: %w", err)
	}

	allTools := make([]any, 0, len(existing)+len(injected))
	for _, item := range existing {
		allTools = append(allTools, item)
	}
	for _, tool := range injected {
		allTools = append(allTools, tool)
	}

	return p.set(reqPathTools, allTools)
}

func (p RequestPayload) disableParallelToolCalls() (RequestPayload, error) {
	return p.set(reqPathParallelToolCalls, false)
}

func (p RequestPayload) appendInputItems(items []responses.ResponseInputItemUnionParam) (RequestPayload, error) {
	if len(items) == 0 {
		return p, nil
	}

	existing, err := p.inputItems()
	if err != nil {
		return p, xerrors.Errorf("failed to get existing 'input' items: %w", err)
	}

	allInput := make([]any, 0, len(existing)+len(items))
	allInput = append(allInput, existing...)
	for _, item := range items {
		allInput = append(allInput, item)
	}

	return p.set(reqPathInput, allInput)
}

func (p RequestPayload) inputItems() ([]any, error) {
	input := gjson.GetBytes(p, reqPathInput)
	if !input.Exists() || input.Type == gjson.Null {
		return []any{}, nil
	}

	if input.Type == gjson.String {
		return []any{responses.ResponseInputItemParamOfMessage(input.String(), responses.EasyInputMessageRoleUser)}, nil
	}

	if !input.IsArray() {
		return nil, xerrors.Errorf("unsupported 'input' type: %s", input.Type)
	}

	items := input.Array()
	existing := make([]any, 0, len(items))
	for _, item := range items {
		existing = append(existing, json.RawMessage(item.Raw))
	}

	return existing, nil
}

func (p RequestPayload) toolItems() ([]json.RawMessage, error) {
	tools := gjson.GetBytes(p, reqPathTools)
	if !tools.Exists() {
		return nil, nil
	}
	if !tools.IsArray() {
		return nil, xerrors.Errorf("unsupported 'tools' type: %s", tools.Type)
	}

	items := tools.Array()
	existing := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		existing = append(existing, json.RawMessage(item.Raw))
	}

	return existing, nil
}

func (p RequestPayload) set(path string, value any) (RequestPayload, error) {
	updated, err := sjson.SetBytes(p, path, value)
	if err != nil {
		return p, xerrors.Errorf("failed to set value at path %s: %w", path, err)
	}
	return updated, nil
}
