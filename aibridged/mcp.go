package aibridged

import (
	"context"
	"fmt"

	"cdr.dev/slog"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/xerrors"
)

type BridgeMCPProxy struct {
	name       string
	client     *client.Client
	logger     slog.Logger
	foundTools []anthropic.BetaToolUnionParam
}

const MCPProxyDelimiter = "_"

func NewBridgeMCPProxy(name, serverURL, token string, logger slog.Logger) (*BridgeMCPProxy, error) {
	mcpClient, err := client.NewStreamableHttpClient(serverURL,
		transport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer " + token,
		}))
	if err != nil {
		return nil, xerrors.Errorf("create streamable http client: %w", err)
	}

	return &BridgeMCPProxy{
		name:   name,
		client: mcpClient,
		logger: logger,
	}, nil
}

func (b *BridgeMCPProxy) Init(ctx context.Context) error {
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

func (b *BridgeMCPProxy) ListTools() []anthropic.BetaToolUnionParam {
	return b.foundTools
}

func (b *BridgeMCPProxy) CallTool(ctx context.Context, name string, input any) (*mcp.CallToolResult, error) {
	return b.client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: input,
		},
	})
}

func (b *BridgeMCPProxy) fetchMCPTools(ctx context.Context) ([]anthropic.BetaToolUnionParam, error) {
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
	fmt.Println(result.ProtocolVersion) // TODO: remove.

	// Test tool listing
	tools, err := b.client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, xerrors.Errorf("list MCP tools: %w", err)
	}

	out := make([]anthropic.BetaToolUnionParam, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		out = append(out, anthropic.BetaToolUnionParam{
			OfTool: &anthropic.BetaToolParam{
				InputSchema: anthropic.BetaToolInputSchemaParam{
					Properties: tool.InputSchema.Properties,
					Required:   tool.InputSchema.Required,
				},
				Name:        fmt.Sprintf("%s%s%s", b.name, MCPProxyDelimiter, tool.Name),
				Description: anthropic.String(tool.Description),
				Type:        anthropic.BetaToolTypeCustom,
			},
		})
	}

	return out, nil
}

func (b *BridgeMCPProxy) Close() {
	// TODO: atomically close.
}
