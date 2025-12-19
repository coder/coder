package aibridged

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/aibridge/mcp"
	"github.com/coder/coder/v2/enterprise/aibridged/proto"
)

var (
	ErrEmptyConfig  = xerrors.New("empty config given")
	ErrCompileRegex = xerrors.New("compile tool regex")
)

const (
	InternalMCPServerID = "coder"
)

type MCPProxyBuilder interface {
	// Build creates a [mcp.ServerProxier] for the given request initiator.
	// At minimum, the Coder MCP server will be proxied.
	// The SessionKey from [Request] is used to authenticate against the Coder MCP server.
	//
	// NOTE: the [mcp.ServerProxier] instance may be proxying one or more MCP servers.
	Build(ctx context.Context, req Request, tracer trace.Tracer) (mcp.ServerProxier, error)
}

var _ MCPProxyBuilder = &MCPProxyFactory{}

type MCPProxyFactory struct {
	logger   slog.Logger
	tracer   trace.Tracer
	clientFn ClientFunc
}

func NewMCPProxyFactory(logger slog.Logger, tracer trace.Tracer, clientFn ClientFunc) *MCPProxyFactory {
	return &MCPProxyFactory{
		logger:   logger,
		tracer:   tracer,
		clientFn: clientFn,
	}
}

func (m *MCPProxyFactory) Build(ctx context.Context, req Request, tracer trace.Tracer) (mcp.ServerProxier, error) {
	proxiers, err := m.retrieveMCPServerConfigs(ctx, req)
	if err != nil {
		return nil, xerrors.Errorf("resolve configs: %w", err)
	}

	return mcp.NewServerProxyManager(proxiers, tracer), nil
}

func (m *MCPProxyFactory) retrieveMCPServerConfigs(ctx context.Context, req Request) (map[string]mcp.ServerProxier, error) {
	client, err := m.clientFn()
	if err != nil {
		return nil, xerrors.Errorf("acquire client: %w", err)
	}

	srvCfgCtx, srvCfgCancel := context.WithTimeout(ctx, time.Second*10)
	defer srvCfgCancel()

	// Fetch MCP server configs.
	mcpSrvCfgs, err := client.GetMCPServerConfigs(srvCfgCtx, &proto.GetMCPServerConfigsRequest{
		UserId: req.InitiatorID.String(),
	})
	if err != nil {
		return nil, xerrors.Errorf("get MCP server configs: %w", err)
	}

	proxiers := make(map[string]mcp.ServerProxier, len(mcpSrvCfgs.GetExternalAuthMcpConfigs())+1) // Extra one for Coder MCP server.

	if mcpSrvCfgs.GetCoderMcpConfig() != nil {
		// Setup the Coder MCP server proxy.
		coderMCPProxy, err := m.newStreamableHTTPServerProxy(mcpSrvCfgs.GetCoderMcpConfig(), req.SessionKey) // The session key is used to auth against our internal MCP server.
		if err != nil {
			m.logger.Warn(ctx, "failed to create MCP server proxy", slog.F("mcp_server_id", mcpSrvCfgs.GetCoderMcpConfig().GetId()), slog.Error(err))
		} else {
			proxiers[InternalMCPServerID] = coderMCPProxy
		}
	}

	if len(mcpSrvCfgs.GetExternalAuthMcpConfigs()) == 0 {
		return proxiers, nil
	}

	serverIDs := make([]string, 0, len(mcpSrvCfgs.GetExternalAuthMcpConfigs()))
	for _, cfg := range mcpSrvCfgs.GetExternalAuthMcpConfigs() {
		serverIDs = append(serverIDs, cfg.GetId())
	}

	accTokCtx, accTokCancel := context.WithTimeout(ctx, time.Second*10)
	defer accTokCancel()

	// Request a batch of access tokens, one per given server ID.
	resp, err := client.GetMCPServerAccessTokensBatch(accTokCtx, &proto.GetMCPServerAccessTokensBatchRequest{
		UserId:             req.InitiatorID.String(),
		McpServerConfigIds: serverIDs,
	})
	if err != nil {
		m.logger.Warn(ctx, "failed to retrieve access token(s)", slog.F("server_ids", serverIDs), slog.Error(err))
	}

	if resp == nil {
		m.logger.Warn(ctx, "nil response given to mcp access tokens call")
		return proxiers, nil
	}
	tokens := resp.GetAccessTokens()
	if len(tokens) == 0 {
		return proxiers, nil
	}

	// Iterate over all External Auth configurations which are configured for MCP and attempt to setup
	// a [mcp.ServerProxier] for it using the access token retrieved above.
	for _, cfg := range mcpSrvCfgs.GetExternalAuthMcpConfigs() {
		if err, ok := resp.GetErrors()[cfg.GetId()]; ok {
			m.logger.Debug(ctx, "failed to get access token", slog.F("mcp_server_id", cfg.GetId()), slog.F("error", err))
			continue
		}

		token, ok := tokens[cfg.GetId()]
		if !ok {
			m.logger.Warn(ctx, "no access token found", slog.F("mcp_server_id", cfg.GetId()))
			continue
		}

		proxy, err := m.newStreamableHTTPServerProxy(cfg, token)
		if err != nil {
			m.logger.Warn(ctx, "failed to create MCP server proxy", slog.F("mcp_server_id", cfg.GetId()), slog.Error(err))
			continue
		}

		proxiers[cfg.Id] = proxy
	}
	return proxiers, nil
}

// newStreamableHTTPServerProxy creates an MCP server capable of proxying requests using the Streamable HTTP transport.
//
// TODO: support SSE transport.
func (m *MCPProxyFactory) newStreamableHTTPServerProxy(cfg *proto.MCPServerConfig, accessToken string) (mcp.ServerProxier, error) {
	if cfg == nil {
		return nil, ErrEmptyConfig
	}

	var (
		allowlist, denylist *regexp.Regexp
		err                 error
	)
	if cfg.GetToolAllowRegex() != "" {
		allowlist, err = regexp.Compile(cfg.GetToolAllowRegex())
		if err != nil {
			return nil, ErrCompileRegex
		}
	}
	if cfg.GetToolDenyRegex() != "" {
		denylist, err = regexp.Compile(cfg.GetToolDenyRegex())
		if err != nil {
			return nil, ErrCompileRegex
		}
	}

	// TODO: future improvement:
	//
	// The access token provided here may expire at any time, or the connection to the MCP server could be severed.
	// Instead of passing through an access token directly, rather provide an interface through which to retrieve
	// an access token imperatively. In the event of a tool call failing, we could Ping() the MCP server to establish
	// whether the connection is still active. If not, this indicates that the access token is probably expired/revoked.
	// (It could also mean the server has a problem, which we should account for.)
	// The proxy could then use its interface to retrieve a new access token and re-establish a connection.
	// For now though, the short TTL of this cache should mostly mask this problem.
	srv, err := mcp.NewStreamableHTTPServerProxy(
		cfg.GetId(),
		cfg.GetUrl(),
		// See https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization#token-requirements.
		map[string]string{
			"Authorization": fmt.Sprintf("Bearer %s", accessToken),
		},
		allowlist,
		denylist,
		m.logger.Named(fmt.Sprintf("mcp-server-proxy-%s", cfg.GetId())),
		m.tracer,
	)
	if err != nil {
		return nil, xerrors.Errorf("create streamable HTTP MCP server proxy: %w", err)
	}

	return srv, nil
}
