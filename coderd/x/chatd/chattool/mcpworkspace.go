package chattool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"charm.land/fantasy"

	aidmcp "github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// WorkspaceMCPTool wraps a single MCP tool discovered in a
// workspace, proxying calls through the workspace agent
// connection. It implements fantasy.AgentTool so it can be
// registered alongside built-in chat tools.
type WorkspaceMCPTool struct {
	info fantasy.ToolInfo
	// routingName is the unsanitized "serverName__toolName" form the
	// workspace agent expects: it splits on "__" to locate the server and
	// calls the original tool name. info.Name is the sanitized, provider-safe
	// name shown to the model, so the two can differ when the server or tool
	// name contains characters outside the provider's allowed set.
	routingName     string
	getConn         func(context.Context) (workspacesdk.AgentConn, error)
	providerOpts    fantasy.ProviderOptions
	invalidateCache func()
}

// NewWorkspaceMCPTool creates a tool wrapper from an MCPToolInfo
// discovered on a workspace agent. Each tool proxies calls back
// through the agent connection. The optional invalidateCache
// callback is invoked when CallMCPTool returns a 404 error,
// indicating that the server was removed and the chat's cached
// tool list should be dropped.
//
// The model-facing name is sanitized to the provider-safe character set and
// length so a server or tool name containing characters such as "@" cannot
// produce an invalid tool name that the provider rejects (Anthropic and
// Bedrock require ^[a-zA-Z0-9_-]{1,128}$). The unsanitized name is retained
// as routingName so the workspace agent can still route the call to the
// original server and tool.
func NewWorkspaceMCPTool(
	tool workspacesdk.MCPToolInfo,
	getConn func(context.Context) (workspacesdk.AgentConn, error),
	invalidateCache func(),
) *WorkspaceMCPTool {
	required := tool.Required
	if required == nil {
		required = []string{}
	}
	return &WorkspaceMCPTool{
		info: fantasy.ToolInfo{
			Name:        sanitizeModelToolName(tool.Name),
			Description: tool.Description,
			Parameters:  tool.Schema,
			Required:    required,
			Parallel:    true,
		},
		routingName:     tool.Name,
		getConn:         getConn,
		invalidateCache: invalidateCache,
	}
}

// sanitizeModelToolName returns the provider-safe form of a workspace MCP
// tool name. Characters outside [a-zA-Z0-9_-] are replaced with "_" and the
// result is capped at the strictest provider tool-name limit, matching the
// remote MCP path in mcpclient. The "__" server/tool separator survives
// because underscores are already in the allowed set.
func sanitizeModelToolName(name string) string {
	sanitized := aidmcp.SanitizeToolName(name)
	if len(sanitized) > aidmcp.MaxToolNameLen {
		sanitized = sanitized[:aidmcp.MaxToolNameLen]
	}
	return sanitized
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
		ToolName:  t.routingName,
		Arguments: args,
	})
	if err != nil {
		// If the agent returns a 404 (ErrUnknownServer), the
		// server was removed or renamed. Invalidate the chat's
		// cached tool list so the next turn refetches.
		var coderErr *codersdk.Error
		if errors.As(err, &coderErr) && coderErr.StatusCode() == http.StatusNotFound {
			if t.invalidateCache != nil {
				t.invalidateCache()
			}
		}
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
