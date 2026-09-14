package mcp

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/maps"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/tracing"
)

var _ ServerProxier = &StreamableHTTPServerProxy{}

type StreamableHTTPServerProxy struct {
	client  *mcp.Client
	tr      *mcp.StreamableClientTransport
	session *mcp.ClientSession
	logger  slog.Logger
	tracer  trace.Tracer

	allowlistPattern *regexp.Regexp
	denylistPattern  *regexp.Regexp

	serverName string
	serverURL  string
	tools      map[string]*Tool
}

func NewStreamableHTTPServerProxy(serverName, serverURL string, headers map[string]string, allowlist, denylist *regexp.Regexp, logger slog.Logger, tracer trace.Tracer, httpClient *http.Client) (*StreamableHTTPServerProxy, error) {
	if httpClient == nil {
		httpClient = mcpHTTPClient()
	}
	httpClient = withHeaders(httpClient, headers)

	tr := &mcp.StreamableClientTransport{
		Endpoint:   serverURL,
		HTTPClient: httpClient,
	}
	mcpClient := mcp.NewClient(GetClientInfo(), nil)

	return &StreamableHTTPServerProxy{
		serverName:       serverName,
		serverURL:        serverURL,
		client:           mcpClient,
		tr:               tr,
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

	// Init may be called again (e.g. via ServerProxyManager); close
	// the previous session so its transport does not leak.
	if p.session != nil {
		if err := p.session.Close(); err != nil {
			p.logger.Debug(ctx, "failed to close previous MCP session", slog.Error(err))
		}
		p.session = nil
	}

	// The SDK negotiates the protocol version during Connect and
	// fails when no mutually supported version exists.
	session, err := p.client.Connect(ctx, p.tr, nil)
	if err != nil {
		return xerrors.Errorf("init MCP client: %w", err)
	}
	p.session = session

	result := session.InitializeResult()
	p.logger.Debug(ctx, "mcp client initialized", slog.F("name", result.ServerInfo.Name), slog.F("server_version", result.ServerInfo.Version))

	tools, err := p.fetchTools(ctx)
	if err != nil {
		if closeErr := session.Close(); closeErr != nil {
			p.logger.Debug(ctx, "failed to close MCP session after fetch tools error", slog.Error(closeErr))
		}
		p.session = nil
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

	if p.session == nil {
		return nil, xerrors.New("proxy not initialized")
	}

	return p.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      tool.Name,
		Arguments: input,
	})
}

func (p *StreamableHTTPServerProxy) fetchTools(ctx context.Context) (_ map[string]*Tool, outErr error) {
	ctx, span := p.tracer.Start(ctx, "StreamableHTTPServerProxy.Init.fetchTools", trace.WithAttributes(p.traceAttributes()...))
	defer tracing.EndSpanErr(span, &outErr)

	tools, err := p.session.ListTools(ctx, nil)
	if err != nil {
		return nil, xerrors.Errorf("list MCP tools: %w", err)
	}

	out := make(map[string]*Tool, len(tools.Tools))
	for _, tool := range tools.Tools {
		encodedID := EncodeToolID(p.serverName, tool.Name)
		if existing, ok := out[encodedID]; ok {
			p.logger.Warn(ctx,
				"duplicate tool ID after sanitization; previous tool will be unreachable",
				slog.F("tool_id", encodedID),
				slog.F("new_tool", tool.Name),
				slog.F("replaced_tool", existing.Name),
				slog.F("server", p.serverName),
			)
		}
		out[encodedID] = &Tool{
			Client:      p.session,
			ID:          encodedID,
			Name:        tool.Name,
			ServerName:  p.serverName,
			ServerURL:   p.serverURL,
			Description: tool.Description,
			Params:      toolParams(tool.InputSchema),
			Required:    toolRequired(tool.InputSchema),
			Logger:      p.logger,
		}
	}
	span.SetAttributes(append(p.traceAttributes(), attribute.Int(tracing.MCPToolCount, len(out)))...)
	return out, nil
}

func (p *StreamableHTTPServerProxy) Shutdown(_ context.Context) error {
	if p.session == nil {
		return nil
	}

	return p.session.Close()
}

func (p *StreamableHTTPServerProxy) traceAttributes() []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(tracing.MCPProxyName, p.Name()),
		attribute.String(tracing.MCPServerName, p.serverName),
		attribute.String(tracing.MCPServerURL, p.serverURL),
	}
}

func toolParams(schema any) map[string]any {
	m, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	properties, _ := m["properties"].(map[string]any)
	return properties
}

func toolRequired(schema any) []string {
	m, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	rawRequired, ok := m["required"].([]any)
	if !ok {
		return nil
	}
	var required []string
	for _, r := range rawRequired {
		if str, ok := r.(string); ok {
			required = append(required, str)
		}
	}
	return required
}
