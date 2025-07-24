package aibridged

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/exp/maps"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

type MCPToolBridge struct {
	name       string
	client     *client.Client
	logger     slog.Logger
	foundTools map[string]*MCPTool
}

type MCPTool struct {
	client      *client.Client
	ID          string
	Name        string
	Description string
	Params      map[string]any
	Required    []string
}

const (
	MCPPrefix    = "__mcp__"
	MCPDelimiter = "_" // TODO: ensure server names CANNOT contain this char.
)

func NewMCPToolBridge(name, serverURL string, headers map[string]string, logger slog.Logger) (*MCPToolBridge, error) {
	// ts := transport.NewMemoryTokenStore()
	// if err := ts.SaveToken(&transport.Token{
	//	AccessToken: token,
	// }); err != nil {
	//	return nil, xerrors.Errorf("save token: %w", err)
	//}

	mcpClient, err := client.NewStreamableHttpClient(serverURL,
		transport.WithHTTPHeaders(headers))
	// transport.WithHTTPOAuth(transport.OAuthConfig{
	//	TokenStore: ts,
	// }))
	if err != nil {
		return nil, xerrors.Errorf("create streamable http client: %w", err)
	}

	return &MCPToolBridge{
		name:   name,
		client: mcpClient,
		logger: logger,
	}, nil
}

func (b *MCPToolBridge) Init(ctx context.Context) error {
	if err := b.client.Start(ctx); err != nil {
		return xerrors.Errorf("start client: %w", err)
	}

	tools, err := b.fetchMCPTools(ctx)
	if err != nil {
		return xerrors.Errorf("fetch tools: %w", err)
	}

	b.foundTools = tools
	return nil
}

func (b *MCPToolBridge) ListTools() []*MCPTool {
	return maps.Values(b.foundTools)
}

func (b *MCPToolBridge) HasTool(name string) bool {
	if b.foundTools == nil {
		return false
	}

	_, ok := b.foundTools[name]
	return ok
}

func (b *MCPToolBridge) CallTool(ctx context.Context, name string, input any) (*mcp.CallToolResult, error) {
	return b.client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: input,
		},
	})
}

func (t *MCPTool) Call(ctx context.Context, input any) (*mcp.CallToolResult, error) {
	return t.client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      t.Name,
			Arguments: input,
		},
	})
}

func (b *MCPToolBridge) fetchMCPTools(ctx context.Context) (map[string]*MCPTool, error) {
	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "coder-ai-bridge",
				Version: "0.0.1",
			},
		},
	}

	result, err := b.client.Initialize(ctx, initReq)
	if err != nil {
		return nil, xerrors.Errorf("init MCP client: %w", err)
	}
	fmt.Printf("mcp(%q)], %+v\n", result.ServerInfo.Name, result) // TODO: remove.

	// Test tool listing
	tools, err := b.client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, xerrors.Errorf("list MCP tools: %w", err)
	}

	out := make(map[string]*MCPTool, len(tools.Tools))
	for _, tool := range tools.Tools {
		out[tool.Name] = &MCPTool{
			client:      b.client,
			ID:          EncodeToolID(b.name, tool.Name),
			Name:        tool.Name,
			Description: tool.Description,
			Params:      tool.InputSchema.Properties,
			Required:    tool.InputSchema.Required,
		}
	}
	return out, nil
}

func EncodeToolID(server, tool string) string {
	return fmt.Sprintf("%s%s%s%s", MCPPrefix, server, MCPDelimiter, tool)
}

func DecodeToolID(id string) (string, string, error) {
	_, name, ok := strings.Cut(id, MCPPrefix)
	if !ok {
		return "", "", xerrors.Errorf("unable to decode %q, prefix %q not found", id, MCPPrefix)
	}

	server, tool, ok := strings.Cut(name, MCPDelimiter)
	if !ok {
		return "", "", xerrors.Errorf("unable to decode %q, delimiter %q not found", id, MCPDelimiter)
	}

	return server, tool, nil
}

func (b *MCPToolBridge) Close() {
	// TODO: atomically close.
}
