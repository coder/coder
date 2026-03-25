package mcpclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
)

// toolNameSep separates the server slug from the original tool
// name in prefixed tool names. Double underscore avoids collisions
// with tool names that may contain single underscores.
//
// TODO: tool names that themselves contain "__" produce ambiguous
// prefixed names (e.g. "srv__my__tool" is indistinguishable from
// slug "srv" + tool "my__tool" vs slug "srv__my" + tool "tool").
// This doesn't affect tool invocation since originalName is used
// directly when calling the remote server.
const toolNameSep = "__"

// connectTimeout bounds how long we wait for a single MCP server
// to start its transport and complete initialization. Servers that
// take longer are skipped so one slow server cannot block the
// entire chat startup.
const connectTimeout = 10 * time.Second

// toolCallTimeout bounds how long a single tool invocation may
// take before being canceled.
const toolCallTimeout = 60 * time.Second

// ConnectAll connects to all configured MCP servers, discovers
// their tools, and returns them as fantasy.AgentTool values. It
// skips servers that fail to connect and logs warnings. The
// returned cleanup function must be called to close all
// connections.
func ConnectAll(
	ctx context.Context,
	logger slog.Logger,
	configs []database.MCPServerConfig,
	tokens []database.MCPServerUserToken,
) ([]fantasy.AgentTool, func()) {
	// Index tokens by server config ID so auth header
	// construction is O(1) per server.
	tokensByConfigID := make(
		map[uuid.UUID]database.MCPServerUserToken, len(tokens),
	)
	for _, tok := range tokens {
		tokensByConfigID[tok.MCPServerConfigID] = tok
	}

	var (
		mu      sync.Mutex
		clients []*client.Client
		tools   []fantasy.AgentTool
	)

	// Build cleanup eagerly so it always closes any clients
	// that connected, even if a later connection fails.
	cleanup := func() {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range clients {
			_ = c.Close()
		}
		clients = nil
	}

	var eg errgroup.Group
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}

		eg.Go(func() error {
			serverTools, mcpClient, connectErr := connectOne(
				ctx, logger, cfg, tokensByConfigID,
			)
			if connectErr != nil {
				logger.Warn(ctx,
					"skipping MCP server due to connection failure",
					slog.F("server_slug", cfg.Slug),
					slog.F("server_url", RedactURL(cfg.Url)),
					slog.F("error", redactErrorURL(connectErr)),
				)
				// Connection failures are not propagated — the
				// LLM simply won't have this server's tools.
				return nil
			}

			mu.Lock()
			clients = append(clients, mcpClient)
			tools = append(tools, serverTools...)
			mu.Unlock()
			return nil
		})
	}

	// All goroutines return nil; error is intentionally
	// discarded.
	_ = eg.Wait()

	return tools, cleanup
}

// connectOne establishes a connection to a single MCP server,
// discovers its tools, and wraps each one as an AgentTool with
// the server slug prefix applied.
func connectOne(
	ctx context.Context,
	logger slog.Logger,
	cfg database.MCPServerConfig,
	tokensByConfigID map[uuid.UUID]database.MCPServerUserToken,
) ([]fantasy.AgentTool, *client.Client, error) {
	headers := buildAuthHeaders(ctx, logger, cfg, tokensByConfigID)

	tr, err := createTransport(cfg, headers)
	if err != nil {
		return nil, nil, xerrors.Errorf(
			"create transport: %w", err,
		)
	}

	mcpClient := client.NewClient(tr)

	// The timeout covers the entire connect+init+list sequence,
	// not each phase individually.
	connectCtx, cancel := context.WithTimeout(
		ctx, connectTimeout,
	)
	defer cancel()

	if err := mcpClient.Start(connectCtx); err != nil {
		_ = mcpClient.Close()
		return nil, nil, xerrors.Errorf(
			"start transport: %w", err,
		)
	}

	_, err = mcpClient.Initialize(
		connectCtx,
		mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo: mcp.Implementation{
					Name:    "coder",
					Version: buildinfo.Version(),
				},
			},
		},
	)
	if err != nil {
		// Best-effort close so we don't leak the transport.
		_ = mcpClient.Close()
		return nil, nil, xerrors.Errorf("initialize: %w", err)
	}

	toolsResult, err := mcpClient.ListTools(
		connectCtx, mcp.ListToolsRequest{},
	)
	if err != nil {
		_ = mcpClient.Close()
		return nil, nil, xerrors.Errorf("list tools: %w", err)
	}

	var tools []fantasy.AgentTool
	for _, mcpTool := range toolsResult.Tools {
		if !isToolAllowed(
			mcpTool.Name,
			cfg.ToolAllowList,
			cfg.ToolDenyList,
		) {
			logger.Debug(ctx, "skipping denied MCP tool",
				slog.F("server_slug", cfg.Slug),
				slog.F("tool_name", mcpTool.Name),
			)
			continue
		}

		tools = append(
			tools, newMCPTool(cfg.ID, cfg.Slug, mcpTool, mcpClient),
		)
	}

	// If no tools passed filtering, close the client early
	// to avoid holding an idle connection.
	if len(tools) == 0 {
		_ = mcpClient.Close()
		return nil, nil, nil
	}

	return tools, mcpClient, nil
}

// createTransport builds the appropriate mcp-go transport based
// on the server's configured transport type.
func createTransport(
	cfg database.MCPServerConfig,
	headers map[string]string,
) (transport.Interface, error) {
	// Each connection gets its own HTTP client with a dedicated
	// transport so that httptest.Server.Close() (which calls
	// CloseIdleConnections on http.DefaultTransport) does not
	// disrupt unrelated connections during parallel tests.
	var httpClient *http.Client
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		httpClient = &http.Client{Transport: dt.Clone()}
	} else {
		httpClient = &http.Client{}
	}

	switch cfg.Transport {
	case "sse":
		return transport.NewSSE(
			cfg.Url,
			transport.WithHeaders(headers),
			transport.WithHTTPClient(httpClient),
		)
	case "", "streamable_http":
		// Default to streamable HTTP, the newer transport.
		return transport.NewStreamableHTTP(
			cfg.Url,
			transport.WithHTTPHeaders(headers),
			transport.WithHTTPBasicClient(httpClient),
		)
	default:
		return nil, xerrors.Errorf(
			"unsupported transport %q", cfg.Transport,
		)
	}
}

// buildAuthHeaders constructs HTTP headers for authenticating
// with the MCP server based on the configured auth type.
func buildAuthHeaders(
	ctx context.Context,
	logger slog.Logger,
	cfg database.MCPServerConfig,
	tokensByConfigID map[uuid.UUID]database.MCPServerUserToken,
) map[string]string {
	// Using map[string]string rather than http.Header because
	// the mcp-go transport options accept map[string]string.
	// MCP servers typically don't require multi-valued headers.
	headers := make(map[string]string)

	switch cfg.AuthType {
	case "oauth2":
		tok, ok := tokensByConfigID[cfg.ID]
		if !ok {
			logger.Warn(ctx,
				"no oauth2 token found for MCP server",
				slog.F("server_slug", cfg.Slug),
			)
			break
		}
		if tok.Expiry.Valid && tok.Expiry.Time.Before(time.Now()) {
			logger.Warn(ctx,
				"oauth2 token for MCP server is expired",
				slog.F("server_slug", cfg.Slug),
				slog.F("expired_at", tok.Expiry.Time),
			)
		}
		if tok.AccessToken == "" {
			logger.Warn(ctx,
				"oauth2 token record has empty access token",
				slog.F("server_slug", cfg.Slug),
			)
			break
		}
		tokenType := tok.TokenType
		if tokenType == "" {
			tokenType = "Bearer"
		}
		// RFC 6750 says the scheme is case-insensitive, but
		// some servers (e.g. Linear) reject lowercase
		// "bearer". Normalize to the canonical form.
		if strings.EqualFold(tokenType, "bearer") {
			tokenType = "Bearer"
		}
		headers["Authorization"] = tokenType + " " + tok.AccessToken
	case "api_key":
		if cfg.APIKeyHeader != "" && cfg.APIKeyValue != "" {
			headers[cfg.APIKeyHeader] = cfg.APIKeyValue
		}
	case "custom_headers":
		if cfg.CustomHeaders != "" {
			var custom map[string]string
			if err := json.Unmarshal(
				[]byte(cfg.CustomHeaders), &custom,
			); err != nil {
				logger.Warn(ctx,
					"failed to parse custom headers JSON",
					slog.F("server_slug", cfg.Slug),
					slog.Error(err),
				)
			} else {
				for k, v := range custom {
					headers[k] = v
				}
			}
		}
	case "none", "":
		// No auth headers needed.
	}

	return headers
}

// isToolAllowed checks a tool name against the allow and deny
// lists. When the allow list is non-empty only tools in it are
// permitted and the deny list is ignored. When the allow list
// is empty and the deny list is non-empty, tools in the deny
// list are rejected. Both lists use exact string matching
// against the original (non-prefixed) tool name.
func isToolAllowed(
	toolName string,
	allowList []string,
	denyList []string,
) bool {
	if len(allowList) > 0 {
		for _, allowed := range allowList {
			if allowed == toolName {
				return true
			}
		}
		// Allow list is set but the tool isn't in it.
		return false
	}

	for _, denied := range denyList {
		if denied == toolName {
			return false
		}
	}

	return true
}

// RedactURL strips userinfo and query parameters from a URL
// to avoid logging embedded credentials. Query params are
// removed because API keys are sometimes passed as
// ?api_key=sk-... in server URLs.
func RedactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// redactErrorURL rewrites URLs in an error string to strip
// credentials. Go's net/http embeds the full request URL in
// *url.Error messages, which can leak userinfo.
func redactErrorURL(err error) string {
	if err == nil {
		return ""
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		urlErr.URL = RedactURL(urlErr.URL)
		return urlErr.Error()
	}
	return err.Error()
}

// MCPToolIdentifier is implemented by tools that originate from
// an MCP server config and can report the config's database ID.
type MCPToolIdentifier interface {
	MCPServerConfigID() uuid.UUID
}

// mcpToolWrapper adapts a single MCP tool into a
// fantasy.AgentTool. It stores the prefixed name for Info() but
// strips the prefix when forwarding calls to the remote server.
type mcpToolWrapper struct {
	configID        uuid.UUID
	prefixedName    string
	originalName    string
	description     string
	parameters      map[string]any
	required        []string
	client          *client.Client
	providerOptions fantasy.ProviderOptions
}

// MCPServerConfigID returns the database ID of the MCP server
// config that this tool originates from.
func (t *mcpToolWrapper) MCPServerConfigID() uuid.UUID {
	return t.configID
}

// newMCPTool creates an mcpToolWrapper from an mcp.Tool
// discovered on a remote server.
func newMCPTool(
	configID uuid.UUID,
	serverSlug string,
	tool mcp.Tool,
	mcpClient *client.Client,
) *mcpToolWrapper {
	return &mcpToolWrapper{
		configID:     configID,
		prefixedName: serverSlug + toolNameSep + tool.Name,
		originalName: tool.Name,
		description:  tool.Description,
		parameters:   tool.InputSchema.Properties,
		required:     tool.InputSchema.Required,
		client:       mcpClient,
	}
}

func (t *mcpToolWrapper) Info() fantasy.ToolInfo {
	// Ensure Required is never nil so that it serializes to [] instead
	// of null. OpenAI rejects null for the JSON Schema "required" field.
	required := t.required
	if required == nil {
		required = []string{}
	}
	return fantasy.ToolInfo{
		Name:        t.prefixedName,
		Description: t.description,
		Parameters:  t.parameters,
		Required:    required,
		Parallel:    true,
	}
}

func (t *mcpToolWrapper) Run(
	ctx context.Context,
	params fantasy.ToolCall,
) (fantasy.ToolResponse, error) {
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

	callCtx, cancel := context.WithTimeout(ctx, toolCallTimeout)
	defer cancel()

	result, err := t.client.CallTool(
		callCtx,
		mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      t.originalName,
				Arguments: args,
			},
		},
	)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	return convertCallResult(result), nil
}

func (t *mcpToolWrapper) ProviderOptions() fantasy.ProviderOptions {
	return t.providerOptions
}

func (t *mcpToolWrapper) SetProviderOptions(
	opts fantasy.ProviderOptions,
) {
	t.providerOptions = opts
}

// convertCallResult translates an MCP CallToolResult into a
// fantasy.ToolResponse. The fantasy response model supports a
// single content type per response, so we prioritize text. All
// text items are collected first. Binary items (image or audio)
// are only returned when no text content is available.
func convertCallResult(
	result *mcp.CallToolResult,
) fantasy.ToolResponse {
	if result == nil {
		return fantasy.NewTextResponse("")
	}

	var (
		textParts    []string
		binaryResult *fantasy.ToolResponse
	)
	for _, item := range result.Content {
		switch c := item.(type) {
		case mcp.TextContent:
			textParts = append(textParts, c.Text)
		case mcp.ImageContent:
			data, err := base64.StdEncoding.DecodeString(
				c.Data,
			)
			if err != nil {
				textParts = append(textParts,
					"[image decode error: "+err.Error()+"]",
				)
				continue
			}
			if binaryResult == nil {
				r := fantasy.ToolResponse{
					Type:      "image",
					Data:      data,
					MediaType: c.MIMEType,
					IsError:   result.IsError,
				}
				binaryResult = &r
			}
		case mcp.AudioContent:
			data, err := base64.StdEncoding.DecodeString(
				c.Data,
			)
			if err != nil {
				textParts = append(textParts,
					"[audio decode error: "+err.Error()+"]",
				)
				continue
			}
			if binaryResult == nil {
				r := fantasy.ToolResponse{
					Type:      "media",
					Data:      data,
					MediaType: c.MIMEType,
					IsError:   result.IsError,
				}
				binaryResult = &r
			}
		default:
			textParts = append(textParts,
				fmt.Sprintf("[unsupported content type: %T]", c),
			)
		}
	}

	// If structured content is present, marshal it to JSON and
	// append as a text part so the data is preserved for the LLM.
	if result.StructuredContent != nil {
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			textParts = append(textParts,
				"[structured content marshal error: "+
					err.Error()+"]",
			)
		} else {
			textParts = append(textParts, string(data))
		}
	}

	// Prefer text content. Only fall back to binary when no
	// text was collected.
	if len(textParts) > 0 {
		resp := fantasy.NewTextResponse(
			strings.Join(textParts, "\n"),
		)
		resp.IsError = result.IsError
		return resp
	}
	if binaryResult != nil {
		return *binaryResult
	}
	return fantasy.NewTextResponse("")
}
