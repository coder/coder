package chattool

import (
	"context"
	"encoding/json"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/toolsdk"
)

// FromToolSDK adapts a toolsdk.GenericTool into a fantasy.AgentTool.
// This allows existing Coder SDK tools to be used directly in the
// fantasy-based chat agent without rewriting their handlers. The
// depsFunc callback is invoked on each Run call to resolve
// dependencies lazily.
func FromToolSDK(tool toolsdk.GenericTool, depsFunc func() (toolsdk.Deps, error)) fantasy.AgentTool {
	return &toolsdkAdapter{
		tool:     tool,
		depsFunc: depsFunc,
	}
}

// toolsdkAdapter wraps a toolsdk.GenericTool to satisfy
// fantasy.AgentTool.
type toolsdkAdapter struct {
	tool            toolsdk.GenericTool
	depsFunc        func() (toolsdk.Deps, error)
	providerOptions fantasy.ProviderOptions
}

func (a *toolsdkAdapter) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name:        a.tool.Name,
		Description: a.tool.Description,
		Parameters:  a.tool.Schema.Properties,
		Required:    a.tool.Schema.Required,
	}
}

func (a *toolsdkAdapter) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	deps, err := a.depsFunc()
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	result, err := a.tool.Handler(ctx, deps, json.RawMessage(call.Input))
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	return fantasy.NewTextResponse(string(result)), nil
}

func (a *toolsdkAdapter) ProviderOptions() fantasy.ProviderOptions {
	return a.providerOptions
}

func (a *toolsdkAdapter) SetProviderOptions(opts fantasy.ProviderOptions) {
	a.providerOptions = opts
}
