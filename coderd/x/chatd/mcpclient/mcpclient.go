package mcpclient

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	aidmcp "github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/safedial"
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

// truncateToolName caps the assembled tool name at MaxToolNameLen so
// it fits within provider limits (e.g. OpenAI 64, Bedrock 128).
func truncateToolName(name string) string {
	if len(name) > aidmcp.MaxToolNameLen {
		return name[:aidmcp.MaxToolNameLen]
	}
	return name
}

// connectTimeout bounds how long we wait for a single MCP server
// to start its transport and complete initialization. Servers that
// take longer are skipped so one slow server cannot block the
// entire chat startup.
const connectTimeout = 10 * time.Second

// toolCallTimeout bounds how long a single tool invocation may
// take before being canceled.
const toolCallTimeout = 60 * time.Second

// slowConnectThreshold is the successful-connect duration above
// which a warning is logged. Connects happen on every generation
// step, so a consistently slow server taxes the whole chat even
// when it stays inside the connect budget.
const slowConnectThreshold = 5 * time.Second

// ConnectOutcome classifies the result of one MCP server connect
// attempt for logs and chat debug runs.
type ConnectOutcome string

const (
	// ConnectOutcomeConnected means tools were discovered and the
	// session is live.
	ConnectOutcomeConnected ConnectOutcome = "connected"
	// ConnectOutcomeTimeout means the connect budget elapsed before
	// the server completed the handshake and tool listing.
	ConnectOutcomeTimeout ConnectOutcome = "timeout"
	// ConnectOutcomeError means the handshake or tool listing
	// failed.
	ConnectOutcomeError ConnectOutcome = "error"
	// ConnectOutcomeNoTools means the server connected but no tools
	// survived allow/deny filtering, so the session was closed.
	ConnectOutcomeNoTools ConnectOutcome = "no_tools"
)

// ConnectSummary describes one MCP server connect attempt. It is
// logged and recorded into chat debug runs so slow or failing
// servers are visible instead of appearing as silent gaps in the
// turn timeline.
type ConnectSummary struct {
	ConfigID   uuid.UUID      `json:"config_id"`
	Slug       string         `json:"slug"`
	Outcome    ConnectOutcome `json:"outcome"`
	DurationMS int64          `json:"duration_ms"`
	ToolCount  int            `json:"tool_count,omitempty"`
	// Error is the redacted, size-bounded connect error, present
	// unless the outcome is connected or no_tools.
	Error string `json:"error,omitempty"`
}

// UserOIDCTokenSource resolves the OIDC access token for the calling
// user. Implementations attempt to refresh tokens that are expired
// or close to expiring and MUST return ("", nil) when the user has
// no OIDC link or a refresh attempt failed for any reason. A
// non-nil error is reserved for unexpected infrastructure failures
// (e.g. database errors) and skips header construction entirely.
// The empty-token-on-refresh-failure behavior matches
// provisionerdserver.ObtainOIDCAccessToken.
type UserOIDCTokenSource interface {
	OIDCAccessToken(ctx context.Context, userID uuid.UUID) (string, error)
}

// ConnectAll connects to all configured MCP servers, discovers
// their tools, and returns them as fantasy.AgentTool values.
// Tools are sorted by their prefixed name so callers
// receive a deterministic order. It skips servers that fail to
// connect and logs warnings; per-server outcomes are returned as
// ConnectSummary values sorted by slug. The returned cleanup
// function must be called to close all connections.
func ConnectAll(
	ctx context.Context,
	logger slog.Logger,
	configs []database.MCPServerConfig,
	tokens []database.MCPServerUserToken,
	userID uuid.UUID,
	oidcSrc UserOIDCTokenSource,
	coderHeaders map[string]string,
	httpClient *http.Client,
) ([]fantasy.AgentTool, []ConnectSummary, func()) {
	return connectAllWithHooks(
		ctx, logger, configs, tokens, userID, oidcSrc, coderHeaders,
		connectOptions{
			httpClient: httpClient,
			timeout:    connectTimeout,
			kind:       connectionKindOrg,
		},
	)
}

// ConnectChatAttached connects to MCP servers that a chat owner attached
// to their own chat. Unlike org-configured servers, these endpoints are
// chosen by an end user, so the connection is hardened: response bodies,
// tool counts, tool definitions, and tool results are size-capped after
// redaction, and sensitiveValues (keyed by config ID) are redacted from
// every string a model or a non-owner chat viewer can see. Chat-attached
// servers have no OAuth tokens or OIDC identity, so those inputs are
// always empty. A nil httpClient falls back to the default guarded client.
func ConnectChatAttached(
	ctx context.Context,
	logger slog.Logger,
	configs []database.MCPServerConfig,
	coderHeaders map[string]string,
	httpClient *http.Client,
	sensitiveValues map[uuid.UUID][]string,
) ([]fantasy.AgentTool, []ConnectSummary, func()) {
	return connectAllWithHooks(
		ctx, logger, configs, nil, uuid.Nil, nil, coderHeaders,
		connectOptions{
			httpClient:      chatAttachedHTTPClient(httpClient),
			timeout:         connectTimeout,
			kind:            connectionKindChatAttached,
			sensitiveValues: sensitiveValues,
		},
	)
}

// connectHooks carries test-only instrumentation for connect
// internals. The zero value is used in production.
type connectHooks struct {
	// reaperDone, when non-nil, is called after an abandoned
	// connect goroutine's late result has been drained and any
	// late session closed.
	reaperDone func()
}

// connectionKind selects the trust level of an MCP server connection.
type connectionKind uint8

const (
	// connectionKindOrg is a server configured by an org admin.
	connectionKindOrg connectionKind = iota
	// connectionKindChatAttached is a server attached to a chat by the
	// chat owner. It is untrusted and subject to caps and redaction.
	connectionKindChatAttached
)

// connectOptions carries the per-connect settings shared by every
// server in one ConnectAll or ConnectChatAttached call.
type connectOptions struct {
	httpClient *http.Client
	timeout    time.Duration
	hooks      connectHooks
	kind       connectionKind
	// sensitiveValues lists, per config ID, the strings to redact from
	// model-visible and viewer-visible text. Chat-attached only.
	sensitiveValues map[uuid.UUID][]string
}

func connectAllWithHooks(
	ctx context.Context,
	logger slog.Logger,
	configs []database.MCPServerConfig,
	tokens []database.MCPServerUserToken,
	userID uuid.UUID,
	oidcSrc UserOIDCTokenSource,
	coderHeaders map[string]string,
	opts connectOptions,
) ([]fantasy.AgentTool, []ConnectSummary, func()) {
	// Index tokens by server config ID so auth header
	// construction is O(1) per server.
	tokensByConfigID := make(
		map[uuid.UUID]database.MCPServerUserToken, len(tokens),
	)
	for _, tok := range tokens {
		tokensByConfigID[tok.MCPServerConfigID] = tok
	}

	var (
		mu        sync.Mutex
		sessions  []*mcp.ClientSession
		tools     []fantasy.AgentTool
		summaries []ConnectSummary
	)

	// Build cleanup eagerly so it always closes any sessions
	// that connected, even if a later connection fails. Each
	// close runs in a detached goroutine: the sessions are
	// discarded either way, and Close on a server that stopped
	// responding mid-turn can block until the transport abandons
	// the connection (the SDK detaches the request context), which
	// must not stall the generation loop at step boundaries.
	cleanup := func() {
		mu.Lock()
		toClose := sessions
		sessions = nil
		mu.Unlock()
		for _, s := range toClose {
			go func() { _ = s.Close() }()
		}
	}

	var eg errgroup.Group
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}

		eg.Go(func() error {
			redactor := newSecretRedactor(opts.sensitiveValues[cfg.ID])
			start := time.Now()
			serverTools, session, connectErr := connectOne(
				ctx, logger, cfg, tokensByConfigID, userID, oidcSrc, coderHeaders,
				opts, redactor,
			)
			duration := time.Since(start)
			summary := ConnectSummary{
				ConfigID:   cfg.ID,
				Slug:       cfg.Slug,
				DurationMS: duration.Milliseconds(),
				ToolCount:  len(serverTools),
			}
			// Redact before truncating so a secret cut at the byte cap
			// cannot leak a prefix into the persisted summary.
			var errText string
			if connectErr != nil {
				errText = redactor.redactString(redactErrorURL(opts.kind, connectErr))
			}
			switch {
			case connectErr != nil && errors.Is(connectErr, context.DeadlineExceeded):
				summary.Outcome = ConnectOutcomeTimeout
				summary.Error = truncateSummaryError(errText)
			case connectErr != nil:
				summary.Outcome = ConnectOutcomeError
				summary.Error = truncateSummaryError(errText)
			case len(serverTools) == 0:
				summary.Outcome = ConnectOutcomeNoTools
			default:
				summary.Outcome = ConnectOutcomeConnected
			}

			if connectErr != nil {
				logger.Warn(ctx,
					"skipping MCP server due to connection failure",
					slog.F("server_slug", cfg.Slug),
					slog.F("server_url", redactServerURL(opts.kind, cfg.Url)),
					slog.F("duration", duration),
					slog.F("error", summary.Error),
				)
			} else if duration >= slowConnectThreshold {
				logger.Warn(ctx,
					"slow MCP server connect",
					slog.F("server_slug", cfg.Slug),
					slog.F("server_url", redactServerURL(opts.kind, cfg.Url)),
					slog.F("duration", duration),
				)
			}

			mu.Lock()
			summaries = append(summaries, summary)
			if connectErr == nil {
				if session != nil {
					sessions = append(sessions, session)
				}
				tools = append(tools, serverTools...)
			}
			mu.Unlock()
			// Connection failures are not propagated; the
			// LLM simply won't have this server's tools.
			return nil
		})
	}

	// All goroutines return nil; error is intentionally
	// discarded.
	_ = eg.Wait()

	// Sort summaries for deterministic ordering regardless of
	// goroutine completion order.
	slices.SortFunc(summaries, func(a, b ConnectSummary) int {
		return cmp.Or(
			cmp.Compare(a.Slug, b.Slug),
			cmp.Compare(a.ConfigID.String(), b.ConfigID.String()),
		)
	})

	// Sort tools by prefixed name for deterministic ordering
	// regardless of goroutine completion order. Ties, possible
	// when the __ separator produces ambiguous prefixed names,
	// are broken by config ID. Stable prompt construction
	// depends on consistent tool ordering.
	slices.SortFunc(tools, func(a, b fantasy.AgentTool) int {
		// All tools in this slice are mcpToolWrapper values
		// created by connectOne above, so these checked
		// assertions should always succeed. The config ID
		// tiebreaker resolves the __ separator ambiguity
		// documented at the top of this file.
		aTool, ok := a.(MCPToolIdentifier)
		if !ok {
			panic(fmt.Sprintf("unexpected tool type %T", a))
		}
		bTool, ok := b.(MCPToolIdentifier)
		if !ok {
			panic(fmt.Sprintf("unexpected tool type %T", b))
		}
		return cmp.Or(
			cmp.Compare(a.Info().Name, b.Info().Name),
			cmp.Compare(aTool.MCPServerConfigID().String(), bTool.MCPServerConfigID().String()),
		)
	})

	// Warn about name collisions that may result from sanitization
	// and truncation. When two tools resolve to the same name, the
	// LLM tool-call dispatch map keeps only one, so the other
	// becomes silently unreachable.
	for i := 1; i < len(tools); i++ {
		if tools[i-1].Info().Name == tools[i].Info().Name {
			prevTool, ok := tools[i-1].(MCPToolIdentifier)
			if !ok {
				continue
			}
			currTool, ok := tools[i].(MCPToolIdentifier)
			if !ok {
				continue
			}
			if prevTool.MCPServerConfigID() != currTool.MCPServerConfigID() {
				logger.Warn(ctx,
					"duplicate tool name after sanitization; one tool will be unreachable",
					slog.F("tool_name", tools[i].Info().Name),
					slog.F("prev_config_id", prevTool.MCPServerConfigID()),
					slog.F("curr_config_id", currTool.MCPServerConfigID()),
				)
			}
		}
	}

	return tools, summaries, cleanup
}

// connectOne establishes a connection to a single MCP server,
// discovers its tools, and wraps each one as an AgentTool with
// the server slug prefix applied.
func connectOne(
	ctx context.Context,
	logger slog.Logger,
	cfg database.MCPServerConfig,
	tokensByConfigID map[uuid.UUID]database.MCPServerUserToken,
	userID uuid.UUID,
	oidcSrc UserOIDCTokenSource,
	coderHeaders map[string]string,
	opts connectOptions,
	redactor secretRedactor,
) ([]fantasy.AgentTool, *mcp.ClientSession, error) {
	headers := buildAuthHeaders(ctx, logger, cfg, tokensByConfigID, userID, oidcSrc)

	// When opted-in, merge Coder identity headers BEFORE the
	// transport is created so any auth header already set above
	// wins on a conflict. Conflict detection uses
	// http.CanonicalHeaderKey because the upstream transport applies
	// http.Header.Set, which canonicalizes keys; without that, an
	// admin-configured header that differs only in case from a Coder
	// identity header would land in the request map twice and the
	// surviving value would be non-deterministic.
	if cfg.ForwardCoderHeaders {
		canonicalAuth := make(map[string]struct{}, len(headers))
		for k := range headers {
			canonicalAuth[http.CanonicalHeaderKey(k)] = struct{}{}
		}
		for k, v := range coderHeaders {
			if _, exists := canonicalAuth[http.CanonicalHeaderKey(k)]; exists {
				continue
			}
			headers[k] = v
		}
	}

	maxResultBytes, maxEventSize := 0, 0
	if opts.kind == connectionKindChatAttached {
		maxResultBytes = maxChatAttachedToolResultBytes
		maxEventSize = maxChatAttachedHTTPResponseBytes
	}
	tr, err := createTransport(cfg, headers, opts.httpClient, maxEventSize)
	if err != nil {
		return nil, nil, xerrors.Errorf(
			"create transport: %w", err,
		)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "coder",
		Version: buildinfo.Version(),
	}, nil)

	// The timeout covers the entire connect+list sequence, not
	// each phase individually. The SDK negotiates the protocol
	// version during Connect; the session outlives connectCtx.
	connectCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	// Run the connect+list sequence in a goroutine and enforce the
	// budget externally. The SDK's streamable transport detaches
	// the context after starting HTTP requests, and its error-path
	// session.Close blocks on those detached requests, so a
	// black-holed server can block Connect far past connectCtx's
	// deadline. The select below guarantees the caller gets an
	// answer within the budget regardless.
	type connectResult struct {
		session *mcp.ClientSession
		tools   *mcp.ListToolsResult
		err     error
	}
	resCh := make(chan connectResult, 1)
	go func() {
		session, err := mcpClient.Connect(connectCtx, tr, nil)
		if err != nil {
			resCh <- connectResult{err: xerrors.Errorf("connect: %w", err)}
			return
		}
		toolsResult, err := session.ListTools(connectCtx, nil)
		if err != nil {
			// Deliver the result before closing: Close sends a
			// DELETE on the SDK's detached context and can wedge,
			// which would otherwise convert a fast ListTools
			// failure into a budget timeout for the caller.
			resCh <- connectResult{err: xerrors.Errorf("list tools: %w", err)}
			_ = session.Close()
			return
		}
		resCh <- connectResult{session: session, tools: toolsResult}
	}()

	var res connectResult
	select {
	case res = <-resCh:
	case <-connectCtx.Done():
		// Abandon the wedged goroutine; it exits once the
		// transport's dial or response-header timeout fires. The
		// reaper drains its late result and closes any session
		// that still materialized so nothing leaks. It must not
		// hold locks or block the caller.
		go func() {
			if late := <-resCh; late.session != nil {
				_ = late.session.Close()
			}
			if opts.hooks.reaperDone != nil {
				opts.hooks.reaperDone()
			}
		}()
		return nil, nil, xerrors.Errorf("connect: %w", connectCtx.Err())
	}
	if res.err != nil {
		return nil, nil, res.err
	}
	session, toolsResult := res.session, res.tools

	var tools []fantasy.AgentTool
	for _, mcpTool := range toolsResult.Tools {
		if mcpTool == nil {
			go func() { _ = session.Close() }()
			return nil, nil, xerrors.New("MCP server returned a null tool definition")
		}
		if !isToolAllowed(
			mcpTool.Name,
			cfg.ToolAllowList,
			cfg.ToolDenyList,
		) {
			logger.Debug(ctx, "skipping denied MCP tool",
				slog.F("server_slug", cfg.Slug),
				slog.F("tool_name", redactor.redactString(mcpTool.Name)),
			)
			continue
		}

		tools = append(
			tools, newMCPTool(cfg.ID, cfg.Slug, mcpTool, session, cfg.ModelIntent, redactor, maxResultBytes),
		)
	}

	if opts.kind == connectionKindChatAttached {
		if err := validateChatAttachedToolDefinitions(tools); err != nil {
			go func() { _ = session.Close() }()
			return nil, nil, err
		}
	}

	if len(tools) == 0 {
		// Close the discarded session asynchronously: Close sends
		// a DELETE on the SDK's detached context, so a server that
		// wedges after a successful connect would otherwise hold
		// the caller far past the connect budget.
		go func() { _ = session.Close() }()
		return nil, nil, nil
	}

	return tools, session, nil
}

// Caps for chat-attached servers. Tool definitions are sent to the model
// on every turn and tool results are persisted into chat messages, so an
// end-user-chosen server must not be able to inflate either unboundedly.
const (
	maxChatAttachedTools                = 64
	maxChatAttachedToolDefinitionBytes  = 64 << 10
	maxChatAttachedToolDefinitionsBytes = 256 << 10
	maxChatAttachedToolResultBytes      = 256 << 10
)

// validateChatAttachedToolDefinitions rejects a tool list that exceeds
// the chat-attached count or size caps. Sizes are measured on the
// redacted definitions the model sees. The whole server is skipped
// rather than truncated so the model never sees a partial tool set.
func validateChatAttachedToolDefinitions(tools []fantasy.AgentTool) error {
	if len(tools) > maxChatAttachedTools {
		return xerrors.Errorf(
			"chat-attached MCP server returned %d tools, maximum is %d",
			len(tools), maxChatAttachedTools,
		)
	}

	totalBytes := 0
	for _, tool := range tools {
		definition, err := json.Marshal(tool.Info())
		if err != nil {
			return xerrors.Errorf("marshal chat-attached MCP tool definition: %w", err)
		}
		if len(definition) > maxChatAttachedToolDefinitionBytes {
			return xerrors.Errorf(
				"chat-attached MCP tool definition exceeds maximum size of %d bytes",
				maxChatAttachedToolDefinitionBytes,
			)
		}
		totalBytes += len(definition)
		if totalBytes > maxChatAttachedToolDefinitionsBytes {
			return xerrors.Errorf(
				"chat-attached MCP tool definitions exceed maximum total size of %d bytes",
				maxChatAttachedToolDefinitionsBytes,
			)
		}
	}
	return nil
}

func createTransport(
	cfg database.MCPServerConfig,
	headers map[string]string,
	baseHTTPClient *http.Client,
	maxEventSize int,
) (mcp.Transport, error) {
	signingSecret := ""
	if cfg.ForwardCoderHeaders {
		signingSecret = cfg.SigningSecret
	}
	httpClient := httpClientWithHeaders(baseHTTPClient, headers, signingSecret)

	switch cfg.Transport {
	case "sse":
		return &mcp.SSEClientTransport{
			Endpoint:     cfg.Url,
			HTTPClient:   httpClient,
			MaxEventSize: maxEventSize,
		}, nil
	case "", "streamable_http":
		// Default to streamable HTTP, the newer transport.
		return &mcp.StreamableClientTransport{
			Endpoint:     cfg.Url,
			HTTPClient:   httpClient,
			MaxEventSize: maxEventSize,
		}, nil
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
	userID uuid.UUID,
	oidcSrc UserOIDCTokenSource,
) map[string]string {
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
		if tok.OauthRefreshFailureReason != "" {
			// The grant is permanently unusable (e.g. revoked
			// upstream) and the user must reconnect. Do not attach
			// any leftover token material.
			logger.Warn(ctx,
				"oauth2 token for MCP server requires reconnect, skipping auth header",
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
	case "user_oidc":
		// Forward the calling user's OIDC access token from
		// user_links as Authorization: Bearer <token>. The token
		// source is responsible for refreshing tokens that are
		// expired or close to expiring before returning them.
		if oidcSrc == nil || userID == uuid.Nil {
			logger.Warn(ctx,
				"user_oidc auth requested but no token source available",
				slog.F("server_slug", cfg.Slug),
			)
			break
		}
		token, err := oidcSrc.OIDCAccessToken(ctx, userID)
		if err != nil {
			logger.Warn(ctx,
				"failed to obtain user OIDC token for MCP server",
				slog.F("server_slug", cfg.Slug),
				slog.Error(err),
			)
			break
		}
		if token == "" {
			// The user has no OIDC link, or a non-fatal refresh
			// failure occurred. Fall through with no header and let
			// the upstream MCP server decide how to respond
			// (typically 401). Logged at debug so password and
			// GitHub users don't generate noise for every chat turn.
			logger.Debug(ctx,
				"no user OIDC token available for MCP server",
				slog.F("server_slug", cfg.Slug),
			)
			break
		}
		headers["Authorization"] = "Bearer " + token
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

// redactServerURL renders a server URL for logs and persisted connect
// errors. Chat-attached URLs are chosen by an end user and commonly
// carry the credential in the path, so only their origin is kept.
func redactServerURL(kind connectionKind, rawURL string) string {
	if kind != connectionKindChatAttached {
		return RedactURL(rawURL)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// redactErrorURL rewrites URLs in an error string to strip
// credentials. Go's net/http embeds the full request URL in
// *url.Error messages, which can leak userinfo.
func redactErrorURL(kind connectionKind, err error) string {
	if err == nil {
		return ""
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		urlErr.URL = redactServerURL(kind, urlErr.URL)
		return urlErr.Error()
	}
	return err.Error()
}

// maxSummaryErrorLen bounds the persisted connect error in bytes.
// Protocol errors can embed arbitrarily large remote-controlled
// response bodies, so without this cap a single retained summary
// could inflate the JSONB row and every debug-runs payload
// regardless of the entry-count cap.
const maxSummaryErrorLen = 512

// summaryError renders a connect error for the persisted summary:
// credential-bearing URLs are redacted and the result is truncated
// to maxSummaryErrorLen bytes on a rune boundary.
func summaryError(err error) string {
	return truncateSummaryError(redactErrorURL(connectionKindOrg, err))
}

// truncateSummaryError caps an already-redacted error message at
// maxSummaryErrorLen bytes on a rune boundary.
func truncateSummaryError(msg string) string {
	if len(msg) <= maxSummaryErrorLen {
		return msg
	}
	cut := maxSummaryErrorLen
	for cut > 0 && !utf8.RuneStart(msg[cut]) {
		cut--
	}
	return msg[:cut] + "... (truncated)"
}

// MCPToolIdentifier is implemented by tools that originate from
// an MCP server config and can report the config's database ID.
type MCPToolIdentifier interface {
	MCPServerConfigID() uuid.UUID
}

// AppendChatAttached appends chat-attached tools to an existing tool
// list. When a chat-attached tool has the same name as an existing
// tool, the existing tool wins and the chat-attached one is dropped
// with a warning: org-configured and built-in tools must never be
// shadowed by an end-user-chosen server.
func AppendChatAttached(
	ctx context.Context,
	logger slog.Logger,
	tools []fantasy.AgentTool,
	chatAttached []fantasy.AgentTool,
) []fantasy.AgentTool {
	if len(chatAttached) == 0 {
		return tools
	}
	existing := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		existing[tool.Info().Name] = struct{}{}
	}
	out := slices.Clone(tools)
	for _, tool := range chatAttached {
		name := tool.Info().Name
		if _, ok := existing[name]; ok {
			var configID uuid.UUID
			if ident, ok := tool.(MCPToolIdentifier); ok {
				configID = ident.MCPServerConfigID()
			}
			logger.Warn(ctx,
				"chat-attached MCP tool name collides with an existing tool; chat-attached tool dropped",
				slog.F("tool_name", name),
				slog.F("config_id", configID),
			)
			continue
		}
		existing[name] = struct{}{}
		out = append(out, tool)
	}
	return out
}

// toolCallIDMetaKey is the _meta key that carries the model's tool
// call ID on every tools/call request. It is a correlation ID for one
// model tool call and is stable across chatd retries of that call, so a
// server MAY use it to deduplicate. No other idempotency guarantee is
// made. The key is reverse-DNS namespaced per the MCP _meta guidance.
const toolCallIDMetaKey = "com.coder/tool_call_id"

// redactedPlaceholder replaces each sensitive value in redacted text.
const redactedPlaceholder = "[REDACTED]"

// secretRedactor replaces a fixed set of sensitive strings with
// redactedPlaceholder. The zero value redacts nothing. Longer values
// are replaced first so a value that contains another value is
// redacted whole. Empty values and substrings of the placeholder are
// dropped: the first would insert the placeholder between every byte,
// the second would mangle placeholders and cannot hide anything.
// Redaction can lengthen its input, so size caps are checked after it.
type secretRedactor struct {
	values []string
}

func newSecretRedactor(values []string) secretRedactor {
	values = slices.Clone(values)
	values = slices.DeleteFunc(values, func(value string) bool {
		return value == "" || strings.Contains(redactedPlaceholder, value)
	})
	slices.SortFunc(values, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(b), len(a)), cmp.Compare(a, b))
	})
	values = slices.Compact(values)
	return secretRedactor{values: values}
}

func (r secretRedactor) redactString(value string) string {
	for _, secret := range r.values {
		value = strings.ReplaceAll(value, secret, redactedPlaceholder)
	}
	return value
}

func (r secretRedactor) redactStrings(values []string) []string {
	if len(values) == 0 || len(r.values) == 0 {
		return values
	}
	redacted := make([]string, len(values))
	for i, item := range values {
		redacted[i] = r.redactString(item)
	}
	return redacted
}

func (r secretRedactor) redactBytes(value []byte) []byte {
	if len(value) == 0 || len(r.values) == 0 {
		return value
	}
	redacted := bytes.Clone(value)
	for _, secret := range r.values {
		redacted = bytes.ReplaceAll(redacted, []byte(secret), []byte(redactedPlaceholder))
	}
	return redacted
}

func (r secretRedactor) redactMap(value map[string]any) map[string]any {
	if len(value) == 0 || len(r.values) == 0 {
		return value
	}
	redacted := make(map[string]any, len(value))
	for key, item := range value {
		redacted[r.redactString(key)] = r.redactValue(item)
	}
	return redacted
}

func (r secretRedactor) redactValue(value any) any {
	if len(r.values) == 0 {
		return value
	}
	switch typed := value.(type) {
	case string:
		return r.redactString(typed)
	case map[string]any:
		return r.redactMap(typed)
	case []any:
		redacted := make([]any, len(typed))
		for i, item := range typed {
			redacted[i] = r.redactValue(item)
		}
		return redacted
	case []string:
		return r.redactStrings(typed)
	default:
		return value
	}
}

func (r secretRedactor) redactResponse(response fantasy.ToolResponse) fantasy.ToolResponse {
	if len(r.values) == 0 {
		return response
	}
	response.Type = r.redactString(response.Type)
	response.Content = r.redactString(response.Content)
	response.Data = r.redactBytes(response.Data)
	response.MediaType = r.redactString(response.MediaType)
	response.Metadata = r.redactString(response.Metadata)
	return response
}

// mcpToolWrapper adapts a single MCP tool into a
// fantasy.AgentTool. It stores the prefixed name for Info() but
// strips the prefix when forwarding calls to the remote server.
type mcpToolWrapper struct {
	configID     uuid.UUID
	prefixedName string
	originalName string
	description  string
	parameters   map[string]any
	required     []string
	modelIntent  bool
	session      *mcp.ClientSession
	redactor     secretRedactor
	// maxResultBytes caps the serialized tool result; 0 means no cap.
	maxResultBytes  int
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
	tool *mcp.Tool,
	session *mcp.ClientSession,
	modelIntent bool,
	redactor secretRedactor,
	maxResultBytes int,
) *mcpToolWrapper {
	properties, required := splitInputSchema(tool.InputSchema)
	// Model-visible fields are redacted once here. originalName is
	// deliberately left as is: it is the name sent in tools/call and the
	// server must recognize it.
	return &mcpToolWrapper{
		configID:       configID,
		prefixedName:   truncateToolName(aidmcp.SanitizeToolName(serverSlug) + toolNameSep + aidmcp.SanitizeToolName(redactor.redactString(tool.Name))),
		originalName:   tool.Name,
		description:    redactor.redactString(tool.Description),
		parameters:     redactor.redactMap(properties),
		required:       redactor.redactStrings(required),
		modelIntent:    modelIntent,
		session:        session,
		redactor:       redactor,
		maxResultBytes: maxResultBytes,
	}
}

func splitInputSchema(schema any) (map[string]any, []string) {
	m, ok := schema.(map[string]any)
	if !ok {
		return nil, nil
	}
	properties, _ := m["properties"].(map[string]any)
	var required []string
	if rawRequired, ok := m["required"].([]any); ok {
		for _, r := range rawRequired {
			if str, ok := r.(string); ok {
				required = append(required, str)
			}
		}
	}
	return properties, required
}

func (t *mcpToolWrapper) Info() fantasy.ToolInfo {
	required := t.required
	if required == nil {
		required = []string{}
	}

	if !t.modelIntent {
		return fantasy.ToolInfo{
			Name:        t.prefixedName,
			Description: t.description,
			Parameters:  t.parameters,
			Required:    required,
			Parallel:    true,
		}
	}

	// Wrap original parameters under "properties" and add
	// "model_intent" so the LLM provides a human-readable
	// description of each tool call.
	wrapped := map[string]any{
		"model_intent": map[string]any{
			"type": "string",
			"description": "A short, natural-language, present-participle " +
				"phrase describing why you are calling this tool. " +
				"This is shown to the user as a status label while " +
				"the tool runs. Use plain English with no underscores " +
				"or technical jargon. Keep it under 100 characters. " +
				"Good examples: \"Reading the authentication module\", " +
				"\"Searching for configuration files\", " +
				"\"Creating a new workspace\".",
		},
		"properties": map[string]any{
			"type":       "object",
			"properties": t.parameters,
			"required":   required,
		},
	}
	return fantasy.ToolInfo{
		Name:        t.prefixedName,
		Description: t.description,
		Parameters:  wrapped,
		Required:    []string{"model_intent", "properties"},
		Parallel:    true,
	}
}

func (t *mcpToolWrapper) Run(
	ctx context.Context,
	params fantasy.ToolCall,
) (fantasy.ToolResponse, error) {
	input := params.Input
	if t.modelIntent {
		input = unwrapModelIntent(input)
	}

	var args map[string]any
	if input != "" {
		if err := json.Unmarshal(
			[]byte(input), &args,
		); err != nil {
			return fantasy.NewTextErrorResponse(
				t.redactor.redactString("invalid JSON input: " + err.Error()),
			), nil
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, toolCallTimeout)
	defer cancel()

	callParams := &mcp.CallToolParams{
		Name:      t.originalName,
		Arguments: args,
	}
	if params.ID != "" {
		callParams.Meta = mcp.Meta{toolCallIDMetaKey: params.ID}
	}
	result, err := t.session.CallTool(callCtx, callParams)
	var resp fantasy.ToolResponse
	if err != nil {
		resp = fantasy.NewTextErrorResponse(t.redactor.redactString(err.Error()))
	} else {
		// Structured content is redacted before convertCallResult encodes it
		// as JSON, where escaping would hide a secret from the string match.
		if result != nil {
			result.StructuredContent = t.redactor.redactValue(result.StructuredContent)
		}
		resp = t.redactor.redactResponse(convertCallResult(result))
	}
	// Measure the result as the model receives it: Data is sent base64-encoded.
	if t.maxResultBytes > 0 && len(resp.Content)+len(resp.MediaType)+base64.StdEncoding.EncodedLen(len(resp.Data)) > t.maxResultBytes {
		return fantasy.NewTextErrorResponse(fmt.Sprintf(
			"tool result exceeded maximum size of %d bytes", t.maxResultBytes,
		)), nil
	}
	return resp, nil
}

func (t *mcpToolWrapper) ProviderOptions() fantasy.ProviderOptions {
	return t.providerOptions
}

func (t *mcpToolWrapper) SetProviderOptions(
	opts fantasy.ProviderOptions,
) {
	t.providerOptions = opts
}

// unwrapModelIntent strips the model_intent wrapper from tool
// call input so the remote MCP server receives only the original
// arguments. It handles three shapes the model may produce:
//
//  1. { model_intent, properties: {...} } — correct format
//  2. { model_intent, key: val, ... } — flat, no properties wrapper
//  3. Anything else — returned as-is
func unwrapModelIntent(input string) string {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(input), &parsed); err != nil {
		return input
	}

	delete(parsed, "model_intent")

	// Case 1: correct { model_intent, properties: {...} } format.
	if props, ok := parsed["properties"]; ok {
		if b, err := json.Marshal(props); err == nil {
			return string(b)
		}
	}

	// Case 2: flat { model_intent, key: val, ... } without wrapper.
	if b, err := json.Marshal(parsed); err == nil {
		return string(b)
	}

	return input
}

// fantasy permits one media payload per response, so only the first eligible
// binary block is kept alongside the text.
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
		case *mcp.TextContent:
			textParts = append(textParts, strings.ToValidUTF8(c.Text, "\uFFFD"))
		case *mcp.ImageContent:
			// The SDK decodes base64 payloads during unmarshal, so
			// Data is raw bytes.
			if binaryResult == nil && len(c.Data) > 0 && c.MIMEType != "" {
				r := fantasy.ToolResponse{
					Type:      "image",
					Data:      c.Data,
					MediaType: c.MIMEType,
					IsError:   result.IsError,
				}
				binaryResult = &r
			}
		case *mcp.AudioContent:
			if binaryResult == nil && len(c.Data) > 0 && c.MIMEType != "" {
				r := fantasy.ToolResponse{
					Type:      "media",
					Data:      c.Data,
					MediaType: c.MIMEType,
					IsError:   result.IsError,
				}
				binaryResult = &r
			}
		case *mcp.EmbeddedResource:
			// Embedded resources wrap either text or blob content
			// from an MCP resource. Exactly one of Text or Blob is
			// set per the spec; a nil Blob means text content.
			switch {
			case c.Resource == nil:
				textParts = append(textParts,
					"[embedded resource with no contents]",
				)
			case c.Resource.Blob != nil:
				if binaryResult == nil && len(c.Resource.Blob) > 0 && c.Resource.MIMEType != "" {
					blobType := "media"
					if strings.HasPrefix(c.Resource.MIMEType, "image/") {
						blobType = "image"
					}
					res := fantasy.ToolResponse{
						Type:      blobType,
						Data:      c.Resource.Blob,
						MediaType: c.Resource.MIMEType,
						IsError:   result.IsError,
					}
					binaryResult = &res
				}
			default:
				textParts = append(textParts, strings.ToValidUTF8(c.Resource.Text, "\uFFFD"))
			}
		case *mcp.ResourceLink:
			// Resource links point to content the LLM can
			// reference by URI. Surface the URI so the model
			// can use it in follow-ups.
			label := c.URI
			if c.Name != "" {
				label = fmt.Sprintf("%s (%s)", c.Name, c.URI)
			}
			if c.Description != "" {
				label += ": " + c.Description
			}
			textParts = append(textParts,
				fmt.Sprintf("[resource: %s]", label),
			)
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

	if binaryResult != nil {
		binaryResult.Content = strings.Join(textParts, "\n")
		return *binaryResult
	}
	resp := fantasy.NewTextResponse(strings.Join(textParts, "\n"))
	resp.IsError = result.IsError
	return resp
}

// RefreshResult contains the outcome of an OAuth2 token refresh
// attempt.
type RefreshResult struct {
	// AccessToken is the new (or unchanged) access token.
	AccessToken string
	// RefreshToken is the new (or preserved original) refresh
	// token. Providers that don't rotate refresh tokens return
	// an empty value; in that case the original is kept.
	RefreshToken string
	// TokenType is the token type (usually "Bearer").
	TokenType string
	// Expiry is the new token expiry. Zero value means no expiry
	// was provided by the provider.
	Expiry time.Time
	// Refreshed is true when the access token actually changed,
	// meaning a refresh occurred. When false the token was still
	// valid and no network call was made.
	Refreshed bool
}

// refreshFailureReasonLimit caps the error text persisted to
// mcp_server_user_tokens.oauth_refresh_failure_reason, matching the
// external auth failure reason limit.
const refreshFailureReasonLimit = 400

// IsPermanentRefreshError reports whether an OAuth2 token refresh
// error means the user's grant is permanently unusable (for example
// the upstream grant was revoked) rather than a transient provider
// failure. Only error codes tied to the grant itself count: client
// or config problems (invalid_client, unauthorized_client, ...)
// affect every user of the server and cannot be fixed by the user
// reconnecting, so they are treated as transient here. See RFC 6749
// section 5.2.
func IsPermanentRefreshError(err error) bool {
	var oauthErr *oauth2.RetrieveError
	if !xerrors.As(err, &oauthErr) {
		return false
	}
	switch oauthErr.ErrorCode {
	case "invalid_grant", // RFC 6749: grant invalid, expired, or revoked
		"bad_refresh_token": // GitHub's equivalent, returned with HTTP 200
		return true
	}
	return false
}

// RefreshFailureReason converts a refresh error into a bounded string
// safe to persist as oauth_refresh_failure_reason. It is stored for
// operator debugging only and is never returned through the API.
func RefreshFailureReason(err error) string {
	reason := err.Error()
	if len(reason) > refreshFailureReasonLimit {
		reason = reason[:refreshFailureReasonLimit]
	}
	return reason
}

// RefreshOAuth2Token checks whether the given MCP user token is
// expired (or within 10 seconds of expiry) and refreshes it using
// the OAuth2 credentials from the server config. If the token is
// still valid, no network call is made and Refreshed is false.
//
// The caller is responsible for persisting the result when
// Refreshed is true.
func RefreshOAuth2Token(
	ctx context.Context,
	httpClient *http.Client,
	cfg database.MCPServerConfig,
	tok database.MCPServerUserToken,
) (RefreshResult, error) {
	oauth2Cfg := &oauth2.Config{
		ClientID:     cfg.OAuth2ClientID,
		ClientSecret: cfg.OAuth2ClientSecret,
		Endpoint: oauth2.Endpoint{
			TokenURL: cfg.OAuth2TokenURL,
		},
	}

	oldToken := &oauth2.Token{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
	}
	if tok.Expiry.Valid {
		oldToken.Expiry = tok.Expiry.Time
	}

	// Cap the refresh HTTP call so a stalled token endpoint
	// cannot block the entire MCP connection phase. The timeout
	// matches connectTimeout used for MCP server connections.
	refreshCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if httpClient == nil {
		httpClient = NewHTTPClient(nil)
	}
	refreshClient := *httpClient
	refreshClient.CheckRedirect = safedial.CheckSameOriginRedirect
	refreshCtx = context.WithValue(refreshCtx, oauth2.HTTPClient, &refreshClient)

	// TokenSource automatically refreshes expired tokens. It
	// uses a 10-second expiry window, so tokens about to expire
	// are also refreshed proactively.
	newToken, err := oauth2Cfg.TokenSource(refreshCtx, oldToken).Token()
	if err != nil {
		return RefreshResult{}, xerrors.Errorf("refresh oauth2 token: %w", err)
	}

	refreshed := newToken.AccessToken != tok.AccessToken

	// Preserve the old refresh token when the provider doesn't
	// rotate (returns empty).
	refreshToken := cmp.Or(newToken.RefreshToken, tok.RefreshToken)

	return RefreshResult{
		AccessToken:  newToken.AccessToken,
		RefreshToken: refreshToken,
		TokenType:    newToken.TokenType,
		Expiry:       newToken.Expiry,
		Refreshed:    refreshed,
	}, nil
}

// RevokeOAuth2Token revokes the user's token at the provider's RFC 7009
// endpoint. It prefers the refresh token, retrying with the access token
// only on unsupported_token_type; other failures do not fall back, since
// an access-token success would hide a possibly live refresh token.
// Returns false without error when there is no revocation endpoint or no
// stored token. Errors carry only the HTTP status because provider
// bodies may echo secrets.
func RevokeOAuth2Token(
	ctx context.Context,
	httpClient *http.Client,
	cfg database.MCPServerConfig,
	tok database.MCPServerUserToken,
) (bool, error) {
	if cfg.OAuth2RevocationURL == "" {
		return false, nil
	}
	if tok.RefreshToken == "" && tok.AccessToken == "" {
		return false, nil
	}
	if err := ValidateRevocationEndpoint(cfg.OAuth2RevocationURL); err != nil {
		return false, err
	}

	if httpClient == nil {
		httpClient = NewHTTPClient(nil)
	}
	// Copy so CheckRedirect does not leak into the shared client.
	redirectSafe := *httpClient
	redirectSafe.CheckRedirect = safedial.CheckSameOriginRedirect
	httpClient = &redirectSafe

	token, hint := tok.AccessToken, "access_token"
	if tok.RefreshToken != "" {
		token, hint = tok.RefreshToken, "refresh_token"
	}
	status, errorCode, err := postTokenRevocation(ctx, httpClient, cfg, token, hint)
	if err != nil {
		return false, err
	}
	if isRevocationSuccessStatus(status) {
		return true, nil
	}

	if hint == "refresh_token" && tok.AccessToken != "" && errorCode == "unsupported_token_type" {
		fbStatus, _, fbErr := postTokenRevocation(ctx, httpClient, cfg, tok.AccessToken, "access_token")
		if fbErr != nil {
			return false, fbErr
		}
		if isRevocationSuccessStatus(fbStatus) {
			return true, nil
		}
		return false, xerrors.Errorf(
			"revocation endpoint returned HTTP %d for the refresh token and HTTP %d for the access token",
			status, fbStatus,
		)
	}
	return false, xerrors.Errorf(
		"revocation endpoint returned HTTP %d", status,
	)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ValidateRevocationEndpoint enforces the RFC 7009 HTTPS requirement;
// the request carries token material and the client secret. Plain HTTP
// is allowed only for loopback hosts.
func ValidateRevocationEndpoint(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return xerrors.Errorf("parse revocation URL: %w", err)
	}
	// url.Parse accepts hostless forms like "https:/revoke" that can
	// never be POSTed to.
	if parsed.Hostname() == "" {
		return xerrors.Errorf(
			"revocation endpoint %q has no host", parsed.Redacted(),
		)
	}
	if !isAllowedRevocationScheme(parsed) {
		return xerrors.Errorf(
			"revocation endpoint %q must use https", parsed.Redacted(),
		)
	}
	return nil
}

func isAllowedRevocationScheme(u *url.URL) bool {
	if u.Scheme == "https" {
		return true
	}
	return u.Scheme == "http" && isLoopbackHost(u.Hostname())
}

func isRevocationSuccessStatus(status int) bool {
	return status == http.StatusOK || status == http.StatusNoContent
}

// postTokenRevocation returns the HTTP status and the RFC 6749 error
// code from the body; the raw body never propagates.
func postTokenRevocation(
	ctx context.Context,
	httpClient *http.Client,
	cfg database.MCPServerConfig,
	token, tokenTypeHint string,
) (int, string, error) {
	form := url.Values{}
	form.Set("token", token)
	form.Set("token_type_hint", tokenTypeHint)
	// Only public clients send client_id in the body; mixing it with
	// Basic auth is malformed per RFC 6749 section 2.3.1.
	if cfg.OAuth2ClientSecret == "" {
		form.Set("client_id", cfg.OAuth2ClientID)
	}

	revokeCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		revokeCtx, http.MethodPost,
		cfg.OAuth2RevocationURL, strings.NewReader(form.Encode()),
	)
	if err != nil {
		return 0, "", xerrors.Errorf("create revocation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Credentials are form-encoded per RFC 6749 section 2.3.1
	// (mirrors x/oauth2).
	if cfg.OAuth2ClientSecret != "" {
		req.SetBasicAuth(url.QueryEscape(cfg.OAuth2ClientID), url.QueryEscape(cfg.OAuth2ClientSecret))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return 0, "", xerrors.Errorf("revoke oauth2 token: %w", err)
	}
	defer resp.Body.Close()

	if isRevocationSuccessStatus(resp.StatusCode) {
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, "", nil
	}
	var errBody struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&errBody)
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, errBody.Error, nil
}
