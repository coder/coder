package mcp

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/maps"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/tracing"
)

var _ ServerProxier = &StreamableHTTPServerProxy{}

type StreamableHTTPServerProxy struct {
	client *client.Client
	logger slog.Logger
	tracer trace.Tracer

	allowlistPattern *regexp.Regexp
	denylistPattern  *regexp.Regexp

	serverName string
	serverURL  string
	tools      map[string]*Tool
}

func NewStreamableHTTPServerProxy(serverName, serverURL string, headers map[string]string, allowlist, denylist *regexp.Regexp, logger slog.Logger, tracer trace.Tracer, opts ...transport.StreamableHTTPCOption) (*StreamableHTTPServerProxy, error) {
	// nit: headers should be passed in as an option instead of a separate parameter. Not changed as this would be a breaking change.
	if headers != nil {
		opts = append(opts, transport.WithHTTPHeaders(headers))
	}

	mcpClient, err := client.NewStreamableHttpClient(serverURL, opts...)
	if err != nil {
		return nil, xerrors.Errorf("create streamable http client: %w", err)
	}

	return &StreamableHTTPServerProxy{
		serverName:       serverName,
		serverURL:        serverURL,
		client:           mcpClient,
		logger:           logger,
		tracer:           tracer,
		allowlistPattern: allowlist,
		denylistPattern:  denylist,
	}, nil
}

func (p *StreamableHTTPServerProxy) Name() string {
	return p.serverName
}

func (p *StreamableHTTPServerProxy) Init(ctx context.Context) (outErr error) {
	ctx, span := p.tracer.Start(ctx, "StreamableHTTPServerProxy.Init", trace.WithAttributes(p.traceAttributes()...))
	defer tracing.EndSpanErr(span, &outErr)

	if err := p.client.Start(ctx); err != nil {
		return xerrors.Errorf("start client: %w", err)
	}

	version := mcp.LATEST_PROTOCOL_VERSION
	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: version,
			ClientInfo:      GetClientInfo(),
		},
	}

	result, err := p.client.Initialize(ctx, initReq)
	if err != nil {
		return xerrors.Errorf("init MCP client: %w", err)
	}

	if !slices.Contains(mcp.ValidProtocolVersions, result.ProtocolVersion) {
		if err := p.client.Close(); err != nil {
			p.logger.Debug(ctx, "failed to close MCP client on unsuccessful version negotiation", slog.Error(err))
		}
		return xerrors.Errorf("MCP version negotiation failed; requested %q, accepts %q, received %q", version, strings.Join(mcp.ValidProtocolVersions, ","), result.ProtocolVersion)
	}

	p.logger.Debug(ctx, "mcp client initialized", slog.F("name", result.ServerInfo.Name), slog.F("server_version", result.ServerInfo.Version))

	tools, err := p.fetchTools(ctx)
	if err != nil {
		return xerrors.Errorf("fetch tools: %w", err)
	}

	// Only include allowed tools.
	p.tools = FilterAllowedTools(p.logger.Named("tool-filterer"), tools, p.allowlistPattern, p.denylistPattern)
	return nil
}

func (p *StreamableHTTPServerProxy) ListTools() []*Tool {
	tools := maps.Values(p.tools)
	slices.SortStableFunc(tools, func(a, b *Tool) int {
		return strings.Compare(a.ID, b.ID)
	})
	return tools
}

func (p *StreamableHTTPServerProxy) GetTool(name string) *Tool {
	if p.tools == nil {
		return nil
	}

	t, ok := p.tools[name]
	if !ok {
		return nil
	}
	return t
}

func (p *StreamableHTTPServerProxy) CallTool(ctx context.Context, name string, input any) (*mcp.CallToolResult, error) {
	tool := p.GetTool(name)
	if tool == nil {
		return nil, xerrors.Errorf("%q tool not known", name)
	}

	return p.client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      tool.Name,
			Arguments: input,
		},
	})
}

func (p *StreamableHTTPServerProxy) fetchTools(ctx context.Context) (_ map[string]*Tool, outErr error) {
	ctx, span := p.tracer.Start(ctx, "StreamableHTTPServerProxy.Init.fetchTools", trace.WithAttributes(p.traceAttributes()...))
	defer tracing.EndSpanErr(span, &outErr)

	tools, err := p.client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, xerrors.Errorf("list MCP tools: %w", err)
	}

	out := make(map[string]*Tool, len(tools.Tools))
	for _, tool := range tools.Tools {
		encodedID := EncodeToolID(p.serverName, tool.Name)
		out[encodedID] = &Tool{
			Client:      p.client,
			ID:          encodedID,
			Name:        tool.Name,
			ServerName:  p.serverName,
			ServerURL:   p.serverURL,
			Description: tool.Description,
			Params:      tool.InputSchema.Properties,
			Required:    tool.InputSchema.Required,
			Logger:      p.logger,
		}
	}
	span.SetAttributes(append(p.traceAttributes(), attribute.Int(tracing.MCPToolCount, len(out)))...)
	return out, nil
}

func (p *StreamableHTTPServerProxy) Shutdown(_ context.Context) error {
	if p.client == nil {
		return nil
	}

	// NOTE: as of v0.38.0 the lib doesn't allow an outside context to be passed in;
	// it has an internal timeout of 5s, though.
	return p.client.Close()
}

func (p *StreamableHTTPServerProxy) traceAttributes() []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(tracing.MCPProxyName, p.Name()),
		attribute.String(tracing.MCPServerName, p.serverName),
		attribute.String(tracing.MCPServerURL, p.serverURL),
	}
}
