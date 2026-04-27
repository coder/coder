package chattool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// WorkspaceMCPTool wraps a single MCP tool discovered in a
// workspace, proxying calls through the workspace agent
// connection. It implements fantasy.AgentTool so it can be
// registered alongside built-in chat tools.
type WorkspaceMCPTool struct {
	info         fantasy.ToolInfo
	getConn      func(context.Context) (workspacesdk.AgentConn, error)
	providerOpts fantasy.ProviderOptions
}

// NewWorkspaceMCPTool creates a tool wrapper from an MCPToolInfo
// discovered on a workspace agent. Each tool proxies calls back
// through the agent connection.
func NewWorkspaceMCPTool(
	tool workspacesdk.MCPToolInfo,
	getConn func(context.Context) (workspacesdk.AgentConn, error),
) *WorkspaceMCPTool {
	required := tool.Required
	if required == nil {
		required = []string{}
	}
	return &WorkspaceMCPTool{
		info: fantasy.ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Schema,
			Required:    required,
			Parallel:    true,
		},
		getConn: getConn,
	}
}

func (t *WorkspaceMCPTool) Info() fantasy.ToolInfo {
	return t.info
}

func (t *WorkspaceMCPTool) Run(
	ctx context.Context,
	params fantasy.ToolCall,
) (fantasy.ToolResponse, error) {
	conn, err := t.getConn(ctx)
	if err != nil {
		return fantasy.NewTextErrorResponse(
			"workspace connection failed: " + err.Error(),
		), nil
	}

	var args map[string]any
	if params.Input != "" {
		if err := json.Unmarshal(
			[]byte(params.Input), &args,
		); err != nil {
			return fantasy.NewTextErrorResponse(
				"invalid JSON input: " + err.Error(),
			), nil
		}
	}

	resp, err := conn.CallMCPTool(ctx, workspacesdk.CallMCPToolRequest{
		ToolName:  t.info.Name,
		Arguments: args,
	})
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	return convertMCPToolResponse(resp), nil
}

func (t *WorkspaceMCPTool) ProviderOptions() fantasy.ProviderOptions {
	return t.providerOpts
}

func (t *WorkspaceMCPTool) SetProviderOptions(
	opts fantasy.ProviderOptions,
) {
	t.providerOpts = opts
}

// convertMCPToolResponse translates a workspace agent MCP tool
// response into a fantasy.ToolResponse. Text content blocks are
// collected and joined; binary content (image/media) is returned
// only when no text is available, matching the mcpclient
// conversion strategy.
func convertMCPToolResponse(
	resp workspacesdk.CallMCPToolResponse,
) fantasy.ToolResponse {
	var (
		textParts    []string
		binaryResult *fantasy.ToolResponse
	)

	for _, c := range resp.Content {
		switch c.Type {
		case "text":
			textParts = append(textParts, strings.ToValidUTF8(c.Text, "\uFFFD"))
		case "image", "audio":
			if c.Data == "" {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(c.Data)
			if err != nil {
				textParts = append(textParts,
					"[binary decode error: "+err.Error()+"]",
				)
				continue
			}
			if binaryResult == nil {
				r := fantasy.ToolResponse{
					Type:      c.Type,
					Data:      data,
					MediaType: c.MediaType,
					IsError:   resp.IsError,
				}
				binaryResult = &r
			}
		default:
			textParts = append(textParts, strings.ToValidUTF8(c.Text, "\uFFFD"))
		}
	}

	// Prefer text content. Only fall back to binary when no
	// text was collected.
	if len(textParts) > 0 {
		r := fantasy.NewTextResponse(
			strings.Join(textParts, "\n"),
		)
		r.IsError = resp.IsError
		return r
	}
	if binaryResult != nil {
		return *binaryResult
	}
	r := fantasy.NewTextResponse("")
	r.IsError = resp.IsError
	return r
}
