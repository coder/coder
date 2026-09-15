// Package mcpgateway is a prototype MCP Gateway. It exposes one MCP endpoint
// per organization that aggregates the MCP servers the requesting user may
// read, filters tools by the per-server allow and deny lists, and proxies tool
// calls to the upstream servers with the user's stored credentials.
//
// This is a proof of feasibility. It connects to every upstream on every
// request and keeps no state between requests.
package mcpgateway

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
)

// connectTimeout bounds the connect and list sequence for one upstream.
const connectTimeout = 10 * time.Second

// Options configures a Gateway.
type Options struct {
	Logger slog.Logger
	// Database must be dbauthz-wrapped. Config reads are post-filtered by
	// the actor in the request context, which is how server-level ACLs are
	// enforced.
	Database database.Store
	// HTTPClient is the base client for upstream MCP connections.
	HTTPClient *http.Client
	// OIDCTokenSource resolves user_oidc upstream credentials. May be nil.
	OIDCTokenSource mcpclient.UserOIDCTokenSource
}

// Gateway serves an aggregated MCP endpoint.
type Gateway struct {
	opts Options
}

// New returns a Gateway.
func New(opts Options) *Gateway {
	return &Gateway{opts: opts}
}

// Handler returns the streamable HTTP handler. The request context must carry
// a dbauthz actor for the requesting user. The organization comes from
// httpmw.OrganizationParam.
func (g *Gateway) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		org := httpmw.OrganizationParam(r)

		userID, err := requestUserID(r)
		if err != nil {
			httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
				Message: "No user in request context.",
				Detail:  err.Error(),
			})
			return
		}

		server, closeUpstreams, err := g.buildServer(ctx, org.ID, userID)
		if err != nil {
			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to build MCP gateway server.",
				Detail:  err.Error(),
			})
			return
		}
		defer closeUpstreams()

		mcp.NewStreamableHTTPHandler(
			func(*http.Request) *mcp.Server { return server },
			&mcp.StreamableHTTPOptions{Stateless: true},
		).ServeHTTP(w, r)
	})
}

// requestUserID resolves the calling user. HTTP callers come through
// apiKeyMiddleware; in-process callers only have a dbauthz actor.
func requestUserID(r *http.Request) (uuid.UUID, error) {
	if key, ok := httpmw.APIKeyOptional(r); ok {
		return key.UserID, nil
	}
	actor, ok := dbauthz.ActorFromContext(r.Context())
	if !ok {
		return uuid.Nil, xerrors.New("no actor in request context")
	}
	userID, err := uuid.Parse(actor.ID)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("parse actor id %q: %w", actor.ID, err)
	}
	return userID, nil
}

// buildServer connects to every MCP server the actor may read and registers
// their allowed tools on a fresh server. An upstream that fails is logged and
// skipped so one bad server cannot break the whole request.
func (g *Gateway) buildServer(
	ctx context.Context,
	orgID uuid.UUID,
	userID uuid.UUID,
) (*mcp.Server, func(), error) {
	configs, err := g.opts.Database.GetEnabledMCPServerConfigsByOrganization(ctx, orgID)
	if err != nil {
		return nil, nil, xerrors.Errorf("get MCP server configs: %w", err)
	}
	tokens, err := g.opts.Database.GetMCPServerUserTokensByUserID(ctx, userID)
	if err != nil {
		return nil, nil, xerrors.Errorf("get MCP server user tokens: %w", err)
	}
	tokensByConfigID := make(map[uuid.UUID]database.MCPServerUserToken, len(tokens))
	for _, token := range tokens {
		tokensByConfigID[token.MCPServerConfigID] = token
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "coder-mcp-gateway",
		Version: buildinfo.Version(),
	}, nil)

	var sessions []*mcp.ClientSession
	for _, cfg := range configs {
		session, tools, err := g.connect(ctx, cfg, tokensByConfigID, userID)
		if err != nil {
			g.opts.Logger.Warn(ctx, "skipping unreachable MCP server",
				slog.F("server_slug", cfg.Slug),
				slog.F("server_url", mcpclient.RedactURL(cfg.Url)),
				slog.Error(err),
			)
			continue
		}
		sessions = append(sessions, session)

		for _, tool := range tools {
			if !mcpclient.IsToolAllowed(tool.Name, cfg.ToolAllowList, cfg.ToolDenyList) {
				continue
			}
			gatewayTool := *tool
			gatewayTool.Name = cfg.Slug + "__" + tool.Name
			server.AddTool(&gatewayTool, forwardTool(session, tool.Name))
		}
	}

	return server, func() {
		for _, session := range sessions {
			_ = session.Close()
		}
	}, nil
}

// connect opens a session to one upstream and lists its tools.
func (g *Gateway) connect(
	ctx context.Context,
	cfg database.MCPServerConfig,
	tokensByConfigID map[uuid.UUID]database.MCPServerUserToken,
	userID uuid.UUID,
) (*mcp.ClientSession, []*mcp.Tool, error) {
	headers := mcpclient.BuildAuthHeaders(
		ctx, g.opts.Logger, cfg, tokensByConfigID, userID, g.opts.OIDCTokenSource,
	)
	transport, err := mcpclient.CreateTransport(cfg, headers, g.opts.HTTPClient)
	if err != nil {
		return nil, nil, xerrors.Errorf("create transport: %w", err)
	}

	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "coder-mcp-gateway",
		Version: buildinfo.Version(),
	}, nil)
	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		return nil, nil, xerrors.Errorf("connect: %w", err)
	}
	result, err := session.ListTools(connectCtx, nil)
	if err != nil {
		_ = session.Close()
		return nil, nil, xerrors.Errorf("list tools: %w", err)
	}
	return session, result.Tools, nil
}

// forwardTool proxies a call to the upstream session under its original name.
func forwardTool(session *mcp.ClientSession, upstreamName string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return session.CallTool(ctx, &mcp.CallToolParams{
			Name:      upstreamName,
			Arguments: req.Params.Arguments,
		})
	}
}
