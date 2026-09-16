package mcpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
)

// MCP Apps extension (SEP-1865, revision 2026-01-26) identifiers.
const (
	// UIExtensionID is the extension identifier advertised in the
	// client's initialize capabilities.
	UIExtensionID = "io.modelcontextprotocol/ui"
	// UIResourceMIMEType is the only MIME type the extension defines
	// for UI resources.
	UIResourceMIMEType = "text/html;profile=mcp-app"
	// UIResourceScheme is the URI scheme reserved for UI resources.
	UIResourceScheme = "ui://"
	// MaxAppResultBytes bounds the raw CallToolResult carried on a
	// tool-result message part. Results above this size are omitted
	// from the part and flagged as truncated.
	MaxAppResultBytes = 64 << 10
	// MaxUIResourceURILen bounds a ui:// resource URI accepted from
	// clients and servers.
	MaxUIResourceURILen = 512
)

// IsValidUIResourceURI reports whether uri is a ui:// URI of bounded
// length made only of characters that need no escaping in a URI or an
// attribute value.
func IsValidUIResourceURI(uri string) bool {
	if !strings.HasPrefix(uri, UIResourceScheme) || len(uri) > MaxUIResourceURILen {
		return false
	}
	for _, r := range uri {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("-._~:/?#[]@!$&'()*+,;=%", r):
		default:
			return false
		}
	}
	return true
}

// Tool visibility values from the extension's McpUiToolMeta.
const (
	ToolVisibilityModel = "model"
	ToolVisibilityApp   = "app"
)

// ConnectOptions adjusts how sessions are established.
type ConnectOptions struct {
	// MCPApps advertises the UI extension during initialize, hides
	// app-only tools from the returned tool list, and attaches the
	// raw CallToolResult to responses of tools that declare a UI
	// resource.
	MCPApps bool
}

// ToolUIMeta is the extension metadata a tool may declare under
// `_meta.ui` (or the deprecated flat `_meta["ui/resourceUri"]`).
type ToolUIMeta struct {
	// ResourceURI is the ui:// resource that renders this tool's
	// results. Empty when the tool has no UI.
	ResourceURI string
	// Visibility lists who may call the tool. Nil means the spec
	// default of both model and app.
	Visibility []string
}

// ParseToolUIMeta extracts UI metadata from a tool's `_meta` map.
// Malformed values are ignored and treated as absent.
func ParseToolUIMeta(meta map[string]any) ToolUIMeta {
	var out ToolUIMeta
	if meta == nil {
		return out
	}
	if ui, ok := meta["ui"].(map[string]any); ok {
		if uri, ok := ui["resourceUri"].(string); ok {
			out.ResourceURI = uri
		}
		if raw, ok := ui["visibility"].([]any); ok {
			vis := make([]string, 0, len(raw))
			for _, v := range raw {
				if s, ok := v.(string); ok {
					vis = append(vis, s)
				}
			}
			// An explicit empty list means nobody may call the
			// tool; keep it distinguishable from absent.
			out.Visibility = vis
		}
	}
	if out.ResourceURI == "" {
		if uri, ok := meta["ui/resourceUri"].(string); ok {
			out.ResourceURI = uri
		}
	}
	if out.ResourceURI != "" && !IsValidUIResourceURI(out.ResourceURI) {
		out.ResourceURI = ""
	}
	return out
}

// ModelVisible reports whether the model may see and call the tool.
func (m ToolUIMeta) ModelVisible() bool {
	return m.Visibility == nil || containsString(m.Visibility, ToolVisibilityModel)
}

// AppVisible reports whether an app rendered from this server may
// call the tool.
func (m ToolUIMeta) AppVisible() bool {
	return m.Visibility == nil || containsString(m.Visibility, ToolVisibilityApp)
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// MCPAppToolIdentifier is implemented by MCP tools that declare a UI
// resource.
type MCPAppToolIdentifier interface {
	MCPToolIdentifier
	// MCPAppResourceURI returns the ui:// resource that renders the
	// tool's results, or "" when the tool has no UI.
	MCPAppResourceURI() string
}

// AppResult is the MCP Apps payload carried in a tool response's
// metadata for tools that declare a UI resource.
type AppResult struct {
	// ResourceURI is the ui:// resource that renders the result.
	ResourceURI string `json:"resource_uri"`
	// Result is the raw CallToolResult as returned by the server.
	// Empty when the serialized result exceeded MaxAppResultBytes.
	Result json.RawMessage `json:"result,omitempty"`
	// Truncated is set when Result was omitted for size.
	Truncated bool `json:"truncated,omitempty"`
}

// appResultMetadata is the JSON object envelope stored in
// fantasy.ToolResponse.Metadata. Unrelated keys such as "attachments"
// may coexist in the same object.
type appResultMetadata struct {
	MCPApp *AppResult `json:"mcp_app,omitempty"`
}

// AppResultFromMetadata decodes the MCP Apps payload from a tool
// response's metadata string. ok is false when the metadata is empty
// or carries no payload.
func AppResultFromMetadata(metadata string) (result AppResult, ok bool, err error) {
	if strings.TrimSpace(metadata) == "" {
		return AppResult{}, false, nil
	}
	var decoded appResultMetadata
	if unmarshalErr := json.Unmarshal([]byte(metadata), &decoded); unmarshalErr != nil {
		return AppResult{}, false, xerrors.Errorf("unmarshal mcp app metadata: %w", unmarshalErr)
	}
	if decoded.MCPApp == nil || decoded.MCPApp.ResourceURI == "" {
		return AppResult{}, false, nil
	}
	return *decoded.MCPApp, true, nil
}

// newAppResult serializes a CallToolResult for the metadata envelope,
// dropping the payload when it exceeds MaxAppResultBytes.
func newAppResult(resourceURI string, result *mcp.CallToolResult) *AppResult {
	out := &AppResult{ResourceURI: resourceURI}
	if result == nil {
		return out
	}
	data, err := json.Marshal(result)
	if err != nil || len(data) > MaxAppResultBytes {
		out.Truncated = true
		return out
	}
	out.Result = data
	return out
}

// uiClientCapabilities returns the capabilities advertised during
// initialize when MCP Apps support is enabled. RootsV2 is set
// explicitly because supplying any capabilities replaces the SDK's
// default set.
func uiClientCapabilities() *mcp.ClientCapabilities {
	caps := &mcp.ClientCapabilities{
		RootsV2: &mcp.RootCapabilities{ListChanged: true},
	}
	caps.AddExtension(UIExtensionID, map[string]any{
		"mimeTypes": []string{UIResourceMIMEType},
	})
	return caps
}

// Session is a short-lived connection to one MCP server used for
// requests issued outside a generation turn.
type Session struct {
	session *mcp.ClientSession
	tools   *mcp.ListToolsResult
	cfg     database.MCPServerConfig
}

// Tools returns the tool list fetched when the session was dialed.
func (s *Session) Tools() []*mcp.Tool {
	if s.tools == nil {
		return nil
	}
	return s.tools.Tools
}

// CallTool invokes a tool on the connected server.
func (s *Session) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, toolCallTimeout)
	defer cancel()
	result, err := s.session.CallTool(callCtx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, xerrors.Errorf("call tool: %w", err)
	}
	return result, nil
}

// ReadResource reads a resource from the connected server.
func (s *Session) ReadResource(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, toolCallTimeout)
	defer cancel()
	result, err := s.session.ReadResource(callCtx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		return nil, xerrors.Errorf("read resource: %w", err)
	}
	return result, nil
}

// maxListedResources bounds how many resources ListResources follows
// across pagination cursors.
const maxListedResources = 1000

// ListResources lists the resources the connected server declares,
// following pagination cursors up to maxListedResources entries. A
// list error from the server is returned to the caller.
func (s *Session) ListResources(ctx context.Context) ([]*mcp.Resource, error) {
	callCtx, cancel := context.WithTimeout(ctx, toolCallTimeout)
	defer cancel()
	var out []*mcp.Resource
	var cursor string
	for {
		result, err := s.session.ListResources(callCtx, &mcp.ListResourcesParams{Cursor: cursor})
		if err != nil {
			return nil, xerrors.Errorf("list resources: %w", err)
		}
		out = append(out, result.Resources...)
		if result.NextCursor == "" || len(out) >= maxListedResources {
			return out, nil
		}
		cursor = result.NextCursor
	}
}

// Close releases the connection. It never blocks the caller: the
// SDK's close sends a DELETE on a detached context that can wedge
// on an unresponsive server.
func (s *Session) Close() {
	go func() { _ = s.session.Close() }()
}

// DialSession connects to a single MCP server and lists its tools.
// It reuses the same auth header construction, transport selection,
// and connect budget as the per-turn connect path. The caller must
// Close the returned session.
func DialSession(
	ctx context.Context,
	logger slog.Logger,
	cfg database.MCPServerConfig,
	tokens []database.MCPServerUserToken,
	userID uuid.UUID,
	oidcSrc UserOIDCTokenSource,
	coderHeaders map[string]string,
	httpClient *http.Client,
	opts ConnectOptions,
) (*Session, error) {
	tokensByConfigID := make(map[uuid.UUID]database.MCPServerUserToken, len(tokens))
	for _, tok := range tokens {
		tokensByConfigID[tok.MCPServerConfigID] = tok
	}
	session, tools, err := dialAndListTools(
		ctx, logger, cfg, tokensByConfigID, userID, oidcSrc, coderHeaders,
		httpClient, connectTimeout, opts, connectHooks{},
	)
	if err != nil {
		return nil, err
	}
	return &Session{session: session, tools: tools, cfg: cfg}, nil
}

// dialAndListTools performs the connect and tools/list handshake
// within a single time budget. The goroutine and select guarantee the
// caller gets an answer within the budget even when the SDK's
// transport blocks past the context deadline.
func dialAndListTools(
	ctx context.Context,
	logger slog.Logger,
	cfg database.MCPServerConfig,
	tokensByConfigID map[uuid.UUID]database.MCPServerUserToken,
	userID uuid.UUID,
	oidcSrc UserOIDCTokenSource,
	coderHeaders map[string]string,
	httpClient *http.Client,
	timeout time.Duration,
	opts ConnectOptions,
	hooks connectHooks,
) (*mcp.ClientSession, *mcp.ListToolsResult, error) {
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

	tr, err := createTransport(cfg, headers, httpClient)
	if err != nil {
		return nil, nil, xerrors.Errorf("create transport: %w", err)
	}

	var clientOpts *mcp.ClientOptions
	if opts.MCPApps {
		clientOpts = &mcp.ClientOptions{Capabilities: uiClientCapabilities()}
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "coder",
		Version: buildinfo.Version(),
	}, clientOpts)

	// The timeout covers the entire connect+list sequence, not
	// each phase individually. The SDK negotiates the protocol
	// version during Connect; the session outlives connectCtx.
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
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
			if hooks.reaperDone != nil {
				hooks.reaperDone()
			}
		}()
		return nil, nil, xerrors.Errorf("connect: %w", connectCtx.Err())
	}
	if res.err != nil {
		return nil, nil, res.err
	}
	return res.session, res.tools, nil
}
