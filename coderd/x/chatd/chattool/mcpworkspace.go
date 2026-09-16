package chattool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// modelToolNameSanitizer matches characters that LLM providers reject in tool
// names. Anthropic and Bedrock require ^[a-zA-Z0-9_-]{1,128}$, and OpenAI
// enforces a 64-character cap over a similar set. A single invalid name would
// otherwise 400 the entire inference request, failing the whole turn.
var modelToolNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// maxModelToolNameLen is the strictest provider tool-name length limit
// (OpenAI allows 64, Bedrock 128); we cap at the lower bound so names are safe
// for every provider.
const maxModelToolNameLen = 64

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
	routingName  string
	getConn      func(context.Context) (workspacesdk.AgentConn, error)
	providerOpts fantasy.ProviderOptions
}

// NewWorkspaceMCPTools builds wrappers for a set of workspace MCP tools.
// Because the model-facing name is sanitized and length-capped, two distinct
// servers or tools can normalize to the same string (for example server keys
// "foo.bar" and "foo_bar" each exposing "echo", or names that share the first
// maxModelToolNameLen bytes). Duplicate names would be sent to the provider,
// which can reject the request, and the model's name-keyed dispatch would make
// one tool unreachable. To keep every tool addressable, colliding model-facing
// names are disambiguated with a numeric suffix while each tool keeps its own
// original routing name. Tools are sorted by routing name first so the suffix
// assignment is stable across turns.
func NewWorkspaceMCPTools(
	infos []workspacesdk.MCPToolInfo,
	getConn func(context.Context) (workspacesdk.AgentConn, error),
) []fantasy.AgentTool {
	sorted := slices.Clone(infos)
	slices.SortFunc(sorted, func(a, b workspacesdk.MCPToolInfo) int {
		return strings.Compare(a.Name, b.Name)
	})
	tools := make([]fantasy.AgentTool, 0, len(sorted))
	seen := make(map[string]struct{}, len(sorted))
	for _, info := range sorted {
		modelName := uniqueModelToolName(sanitizeModelToolName(info.Name), seen)
		tools = append(tools, buildWorkspaceMCPTool(info, modelName, getConn))
	}
	return tools
}

func buildWorkspaceMCPTool(
	tool workspacesdk.MCPToolInfo,
	modelName string,
	getConn func(context.Context) (workspacesdk.AgentConn, error),
) *WorkspaceMCPTool {
	required := tool.Required
	if required == nil {
		required = []string{}
	}
	return &WorkspaceMCPTool{
		info: fantasy.ToolInfo{
			Name:        modelName,
			Description: tool.Description,
			Parameters:  tool.Schema,
			Required:    required,
			Parallel:    true,
		},
		routingName: tool.Name,
		getConn:     getConn,
	}
}

// ServerName returns the originating MCP server name from the unsanitized
// routing name. The model-facing info.Name can lose the "__" separator to
// sanitization or length capping, so it cannot be parsed for the server.
func (t *WorkspaceMCPTool) ServerName() string {
	server, _, ok := strings.Cut(t.routingName, "__")
	if !ok {
		return ""
	}
	return server
}

// sanitizeModelToolName returns the provider-safe form of a workspace MCP tool
// name: characters outside [a-zA-Z0-9_-] become "_" and the result is capped
// at maxModelToolNameLen. The "__" server/tool separator survives because
// underscores are already in the allowed set.
func sanitizeModelToolName(name string) string {
	sanitized := modelToolNameSanitizer.ReplaceAllString(name, "_")
	if len(sanitized) > maxModelToolNameLen {
		sanitized = sanitized[:maxModelToolNameLen]
	}
	return sanitized
}

// uniqueModelToolName returns name when it is unused; otherwise it appends an
// incrementing "_N" suffix (starting at 2), truncating the base so the result
// stays within maxModelToolNameLen, until it finds a name absent from seen.
// The returned name is recorded in seen.
func uniqueModelToolName(name string, seen map[string]struct{}) string {
	if _, ok := seen[name]; !ok {
		seen[name] = struct{}{}
		return name
	}
	for i := 2; ; i++ {
		suffix := "_" + strconv.Itoa(i)
		base := name
		if len(base)+len(suffix) > maxModelToolNameLen {
			cut := maxModelToolNameLen - len(suffix)
			if cut < 0 {
				cut = 0
			}
			base = base[:cut]
		}
		candidate := base + suffix
		if _, ok := seen[candidate]; !ok {
			seen[candidate] = struct{}{}
			return candidate
		}
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
		ToolName:  t.routingName,
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
