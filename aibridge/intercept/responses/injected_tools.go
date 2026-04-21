package responses

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared/constant"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/aibridge/recorder"
)

func (i *responsesInterceptionBase) injectTools() {
	if i.mcpProxy == nil || !i.hasInjectableTools() {
		return
	}

	i.disableParallelToolCalls()

	// Inject tools.
	var injected []responses.ToolUnionParam
	for _, tool := range i.mcpProxy.ListTools() {
		var params map[string]any

		if tool.Params != nil {
			params = map[string]any{
				"type":       "object",
				"properties": tool.Params,
				// "additionalProperties": false, // Only relevant when strict=true.
			}
		}

		// Otherwise the request fails with "None is not of type 'array'" if a nil slice is given.
		if len(tool.Required) > 0 {
			// Must list ALL properties when strict=true.
			params["required"] = tool.Required
		}

		injected = append(injected, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        tool.ID,
				Strict:      openai.Bool(false), // TODO: configurable.
				Description: openai.String(tool.Description),
				Parameters:  params,
			},
		})
	}

	updated, err := i.reqPayload.injectTools(injected)
	if err != nil {
		i.logger.Warn(context.Background(), "failed to inject tools", slog.Error(err))
		return
	}
	i.reqPayload = updated
}

// disableParallelToolCalls disables parallel tool calls, to simplify the inner agentic loop.
// This is best-effort, and failing to set this flag does not fail the request.
// TODO: implement parallel tool calls.
func (i *responsesInterceptionBase) disableParallelToolCalls() {
	updated, err := i.reqPayload.disableParallelToolCalls()
	if err != nil {
		i.logger.Warn(context.Background(), "failed to disable parallel_tool_calls", slog.Error(err))
		return
	}
	i.reqPayload = updated
}

// handleInnerAgenticLoop orchestrates the inner agentic loop whereby injected tools
// are invoked and their results are sent back to the model.
// This is in contrast to regular tool calls which will be handled by the client
// in its own agentic loop.
func (i *responsesInterceptionBase) handleInnerAgenticLoop(ctx context.Context, pending []responses.ResponseFunctionToolCall, response *responses.Response) (bool, error) {
	// Invoke any injected function calls.
	// The Responses API refers to what we call "tools" as "functions", so we keep the terminology
	// consistent in this package.
	// See https://platform.openai.com/docs/guides/function-calling
	results, err := i.handleInjectedToolCalls(ctx, pending, response)
	if err != nil {
		return false, xerrors.Errorf("failed to handle injected tool calls: %w", err)
	}

	// No tool results means no tools were invocable, so the flow is complete.
	if len(results) == 0 {
		return false, nil
	}

	// We'll use the tool results to issue another request to provide the model with.
	err = i.prepareRequestForAgenticLoop(ctx, response, results)

	return true, err
}

// handleInjectedToolCalls checks for function calls that we need to handle in our inner agentic loop.
// These are functions injected by the MCP proxy.
// Returns a list of tool call results.
func (i *responsesInterceptionBase) handleInjectedToolCalls(ctx context.Context, pending []responses.ResponseFunctionToolCall, response *responses.Response) ([]responses.ResponseInputItemUnionParam, error) {
	if response == nil {
		return nil, xerrors.New("empty response")
	}

	// MCP proxy has not been configured; no way to handle injected functions.
	if i.mcpProxy == nil {
		return nil, nil
	}

	var results []responses.ResponseInputItemUnionParam
	for _, fc := range pending {
		results = append(results, i.invokeInjectedTool(ctx, response.ID, fc))
	}

	return results, nil
}

// prepareRequestForAgenticLoop prepares the request by setting the output of the given
// response as input to the next request, in order for the tool call result(s) to make function correctly.
func (i *responsesInterceptionBase) prepareRequestForAgenticLoop(ctx context.Context, response *responses.Response, toolResults []responses.ResponseInputItemUnionParam) error {
	// Collect new items to add: response outputs converted to input format + tool results.
	var newItems []responses.ResponseInputItemUnionParam

	// OutputText is also available, but by definition the trigger for a function call is not a simple
	// text response from the model.
	for _, output := range response.Output {
		if inputItem := i.convertOutputToInput(output); inputItem != nil {
			newItems = append(newItems, *inputItem)
		}
	}
	newItems = append(newItems, toolResults...)

	updated, err := i.reqPayload.appendInputItems(newItems)
	if err != nil {
		i.logger.Error(ctx, "failed to rewrite input in inner agentic loop", slog.Error(err))
		return xerrors.Errorf("failed to rewrite input: %w", err)
	}
	i.reqPayload = updated

	return nil
}

// getPendingInjectedToolCalls extracts function calls from the response that are managed by MCP proxy.
func (i *responsesInterceptionBase) getPendingInjectedToolCalls(response *responses.Response) []responses.ResponseFunctionToolCall {
	var calls []responses.ResponseFunctionToolCall

	for _, item := range response.Output {
		if item.Type != string(constant.ValueOf[constant.FunctionCall]()) {
			continue
		}

		// Injected functions are defined by MCP, and MCP tools have to have a schema
		// for their inputs. The Responses API also supports "Custom Tools":
		// https://platform.openai.com/docs/guides/function-calling#custom-tools
		// These are like regular functions but their inputs are not schematized.
		// As such, custom tools are not considered here.
		fc := item.AsFunctionCall()

		// Check if this is a tool managed by our MCP proxy
		if i.mcpProxy != nil && i.mcpProxy.GetTool(fc.Name) != nil {
			calls = append(calls, fc)
		}
	}

	return calls
}

func (i *responsesInterceptionBase) invokeInjectedTool(ctx context.Context, responseID string, fc responses.ResponseFunctionToolCall) responses.ResponseInputItemUnionParam {
	tool := i.mcpProxy.GetTool(fc.Name)
	if tool == nil {
		return responses.ResponseInputItemParamOfFunctionCallOutput(fc.CallID, fmt.Sprintf("error: unknown injected function %q", fc.ID))
	}

	args := i.parseFunctionCallJSONArgs(ctx, fc.Arguments)
	res, err := tool.Call(ctx, args, i.tracer)
	_ = i.recorder.RecordToolUsage(ctx, &recorder.ToolUsageRecord{
		InterceptionID:  i.ID().String(),
		MsgID:           responseID,
		ToolCallID:      fc.CallID,
		ServerURL:       &tool.ServerURL,
		Tool:            tool.Name,
		Args:            args,
		Injected:        true,
		InvocationError: err,
	})

	var output string
	if err != nil {
		// Results have no fixed structure; if an error occurs, we can just pass back the error.
		// https://platform.openai.com/docs/guides/function-calling?strict-mode=enabled#formatting-results
		output = fmt.Sprintf("invocation error: %q", err.Error())
	} else {
		var out strings.Builder
		if encErr := json.NewEncoder(&out).Encode(res); encErr != nil {
			i.logger.Warn(ctx, "failed to encode tool response", slog.Error(encErr))
			output = fmt.Sprintf("result encode error: %q", encErr.Error())
		} else {
			output = out.String()
		}
	}

	return responses.ResponseInputItemParamOfFunctionCallOutput(fc.CallID, output)
}

// convertOutputToInput converts a response output item to an input item and appends it to the
// request's input list. This is used in agentic loops where we need to feed the model's output
// back as input for the next iteration (e.g., when processing tool call results).
//
// The conversion uses the openai-go library's ToParam() methods where available, which leverage
// param.Override() with raw JSON to preserve all fields. For types without ToParam(), we use
// the ResponseInputItemParamOf* helper functions.
func (i *responsesInterceptionBase) convertOutputToInput(item responses.ResponseOutputItemUnion) *responses.ResponseInputItemUnionParam {
	var inputItem responses.ResponseInputItemUnionParam

	switch item.Type {
	case string(constant.ValueOf[constant.Message]()):
		p := item.AsMessage().ToParam()
		inputItem = responses.ResponseInputItemUnionParam{OfOutputMessage: &p}

	case string(constant.ValueOf[constant.FileSearchCall]()):
		p := item.AsFileSearchCall().ToParam()
		inputItem = responses.ResponseInputItemUnionParam{OfFileSearchCall: &p}

	case string(constant.ValueOf[constant.FunctionCall]()):
		p := item.AsFunctionCall().ToParam()
		inputItem = responses.ResponseInputItemUnionParam{OfFunctionCall: &p}

	case string(constant.ValueOf[constant.WebSearchCall]()):
		p := item.AsWebSearchCall().ToParam()
		inputItem = responses.ResponseInputItemUnionParam{OfWebSearchCall: &p}

	case "computer_call": // No constant.ComputerCall type exists
		p := item.AsComputerCall().ToParam()
		inputItem = responses.ResponseInputItemUnionParam{OfComputerCall: &p}

	case string(constant.ValueOf[constant.Reasoning]()):
		p := item.AsReasoning().ToParam()
		inputItem = responses.ResponseInputItemUnionParam{OfReasoning: &p}

	case string(constant.ValueOf[constant.Compaction]()):
		c := item.AsCompaction()
		inputItem = responses.ResponseInputItemParamOfCompaction(c.EncryptedContent)

	case string(constant.ValueOf[constant.ImageGenerationCall]()):
		c := item.AsImageGenerationCall()
		inputItem = responses.ResponseInputItemParamOfImageGenerationCall(c.ID, c.Result, c.Status)

	case string(constant.ValueOf[constant.CodeInterpreterCall]()):
		p := item.AsCodeInterpreterCall().ToParam()
		inputItem = responses.ResponseInputItemUnionParam{OfCodeInterpreterCall: &p}

	case "custom_tool_call": // No constant.CustomToolCall type exists
		p := item.AsCustomToolCall().ToParam()
		inputItem = responses.ResponseInputItemUnionParam{OfCustomToolCall: &p}

	// Output-only types that don't have direct input equivalents or are handled separately:
	// - local_shell_call, shell_call, shell_call_output: Shell tool outputs
	// - apply_patch_call, apply_patch_call_output: Apply patch outputs
	// - mcp_call, mcp_list_tools, mcp_approval_request: MCP-specific outputs
	default:
		i.logger.Debug(context.Background(), "skipping output item type for input", slog.F("type", item.Type))
		return nil
	}

	return &inputItem
}
