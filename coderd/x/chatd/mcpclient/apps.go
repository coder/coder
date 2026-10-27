package mcpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// MCP Apps (SEP-1865, spec 2026-01-26) names.
const (
	// AppsExtension is the client extension that tells a server the
	// host can show views.
	AppsExtension = "io.modelcontextprotocol/ui"
	// AppMIMEType is the MIME type of a view resource.
	AppMIMEType = "text/html;profile=mcp-app"
	// VisibilityModel lets the model call a tool.
	VisibilityModel = "model"
	// VisibilityApp lets a view call a tool.
	VisibilityApp = "app"
)

// Caps for MCP Apps host requests. A view and the results it reads go
// to the browser, so one server must not be able to send unbounded data.
const (
	// maxAppViewBytes caps the HTML of one view.
	maxAppViewBytes = 2 << 20
	// maxAppToolResultBytes caps the result of a tool call that a view
	// or an entry point starts.
	maxAppToolResultBytes = 2 << 20
	// maxPersistedAppResultBytes caps the raw result that a chat
	// message part keeps for a view. A bigger result keeps only the
	// tool input.
	maxPersistedAppResultBytes = 256 << 10
)

// Errors of MCP Apps host requests.
var (
	// ErrAppToolNotFound means the server has no such tool, or the
	// tool allow and deny lists of the server config hide it.
	ErrAppToolNotFound = xerrors.New("tool not found")
	// ErrAppToolNotAllowed means the tool visibility excludes "app".
	ErrAppToolNotAllowed = xerrors.New("tool is not visible to apps")
	// ErrAppViewInvalid means the resource is not a usable view.
	ErrAppViewInvalid = xerrors.New("resource is not an MCP Apps view")
	// ErrAppResultTooLarge means a result is above the size cap.
	ErrAppResultTooLarge = xerrors.New("result is too large")
)

// UIMeta is the MCP Apps part of a tool's _meta, plus the OpenAI thread
// entry point (OpenAI MCP Extensions, "MCP App Entrypoints").
type UIMeta struct {
	// ResourceURI is _meta.ui.resourceUri, or the deprecated flat
	// _meta["ui/resourceUri"]. It is empty unless it is a ui:// URI.
	ResourceURI string
	// Visibility is _meta.ui.visibility. The spec default is
	// ["model", "app"].
	Visibility []string
	// Thread reports a {"type": "thread"} entry in
	// _meta["openai/ui"].entrypoints on a tool with a view.
	Thread bool
}

// ParseUIMeta reads the MCP Apps metadata of a tool.
func ParseUIMeta(meta mcp.Meta) UIMeta {
	out := UIMeta{Visibility: []string{VisibilityModel, VisibilityApp}}
	ui, _ := meta["ui"].(map[string]any)
	uri, _ := ui["resourceUri"].(string)
	if uri == "" {
		uri, _ = meta["ui/resourceUri"].(string)
	}
	if strings.HasPrefix(uri, "ui://") {
		out.ResourceURI = uri
	}
	if raw, ok := ui["visibility"].([]any); ok {
		out.Visibility = make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok {
				out.Visibility = append(out.Visibility, s)
			}
		}
	}
	openaiUI, _ := meta["openai/ui"].(map[string]any)
	entries, _ := openaiUI["entrypoints"].([]any)
	for _, entry := range entries {
		if e, _ := entry.(map[string]any); e["type"] == "thread" {
			out.Thread = out.ResourceURI != ""
		}
	}
	return out
}

// Allows reports whether the visibility includes who.
func (m UIMeta) Allows(who string) bool {
	return slices.Contains(m.Visibility, who)
}

// ToolTitle is the display name of a tool: title, then
// annotations.title, then the name.
func ToolTitle(tool *mcp.Tool) string {
	if tool.Title != "" {
		return tool.Title
	}
	if tool.Annotations != nil && tool.Annotations.Title != "" {
		return tool.Annotations.Title
	}
	return tool.Name
}

// appMetadata returns the tool response metadata that carries the view
// data: {"mcp_app": codersdk.ChatMCPApp}. The chat message part
// decodes it. The model never gets it.
func (t *mcpToolWrapper) appMetadata(args map[string]any, result *mcp.CallToolResult) string {
	app := codersdk.ChatMCPApp{
		MCPServerConfigID: t.configID,
		ToolName:          t.originalName,
		Title:             t.title,
		ResourceURI:       t.ui.ResourceURI,
		ToolInput:         args,
	}
	if app.ToolInput == nil {
		app.ToolInput = map[string]any{}
	}
	if raw, err := json.Marshal(result); err == nil && len(raw) <= maxPersistedAppResultBytes {
		_ = json.Unmarshal(raw, &app.ToolResult)
	}
	encoded, err := json.Marshal(map[string]any{"mcp_app": app})
	if err != nil {
		return ""
	}
	return string(encoded)
}

// AppSession is a short-lived MCP session for the browser requests of
// MCP Apps: read a view, list entry points, and call tools for a view.
// The caller must call Close.
type AppSession struct {
	srv     Server
	session *mcp.ClientSession
}

// ConnectAppSession connects to one org MCP server with the MCP Apps
// extension, as the user. It uses the same auth headers as a chat turn.
func ConnectAppSession(
	ctx context.Context,
	logger slog.Logger,
	srv Server,
	tokens []database.MCPServerUserToken,
	userID uuid.UUID,
	oidcSrc UserOIDCTokenSource,
	coderHeaders map[string]string,
	httpClient *http.Client,
) (*AppSession, error) {
	srv.MCPApps = true
	tokensByConfigID := make(map[uuid.UUID]database.MCPServerUserToken, len(tokens))
	for _, tok := range tokens {
		tokensByConfigID[tok.MCPServerConfigID] = tok
	}
	opts := connectOptions{httpClient: httpClient, timeout: connectTimeout, kind: connectionKindOrg}
	tr, err := serverTransport(ctx, logger, srv, tokensByConfigID, userID, oidcSrc, coderHeaders, opts)
	if err != nil {
		return nil, xerrors.Errorf("create transport: %w", err)
	}
	connectCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	session, err := newMCPClient(srv).Connect(connectCtx, tr, nil)
	if err != nil {
		return nil, xerrors.Errorf("connect: %w", err)
	}
	return &AppSession{srv: srv, session: session}, nil
}

// Close closes the session in the background, because HTTP session
// cleanup can block on a DELETE request.
func (s *AppSession) Close() {
	go func() { _ = s.session.Close() }()
}

// AppView is a view resource of a server.
type AppView struct {
	HTML []byte
	// CSP is the content _meta.ui.csp of the resource.
	CSP AppViewCSP
}

// AppViewCSP is the CSP part of a view resource's _meta.ui (SEP-1865).
type AppViewCSP struct {
	ConnectDomains  []string `json:"connectDomains"`
	ResourceDomains []string `json:"resourceDomains"`
	FrameDomains    []string `json:"frameDomains"`
}

// ReadView reads the ui:// resource uri and checks that it is HTML.
func (s *AppSession) ReadView(ctx context.Context, uri string) (AppView, error) {
	if !strings.HasPrefix(uri, "ui://") {
		return AppView{}, xerrors.Errorf("%w: the URI must start with ui://", ErrAppViewInvalid)
	}
	res, err := s.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		return AppView{}, xerrors.Errorf("read resource: %w", err)
	}
	for _, content := range res.Contents {
		if content == nil || content.URI != uri {
			continue
		}
		mime := strings.ToLower(strings.ReplaceAll(content.MIMEType, " ", ""))
		if !strings.HasPrefix(mime, "text/html") {
			return AppView{}, xerrors.Errorf("%w: MIME type %q", ErrAppViewInvalid, content.MIMEType)
		}
		html := []byte(content.Text)
		if content.Blob != nil {
			html = content.Blob
		}
		if len(html) > maxAppViewBytes {
			return AppView{}, xerrors.Errorf("%w: the view is %d bytes", ErrAppResultTooLarge, len(html))
		}
		if !utf8.Valid(html) {
			return AppView{}, xerrors.Errorf("%w: the view is not UTF-8", ErrAppViewInvalid)
		}
		view := AppView{HTML: html}
		if ui, ok := content.Meta["ui"].(map[string]any); ok {
			if raw, err := json.Marshal(ui["csp"]); err == nil {
				_ = json.Unmarshal(raw, &view.CSP)
			}
		}
		return view, nil
	}
	return AppView{}, xerrors.Errorf("%w: no content for %q", ErrAppViewInvalid, uri)
}

// AppEntrypoint is a tool with an OpenAI thread entry point.
type AppEntrypoint struct {
	ToolName    string
	Title       string
	ResourceURI string
}

// Entrypoints lists the tools with a thread entry point, in server order.
func (s *AppSession) Entrypoints(ctx context.Context) ([]AppEntrypoint, error) {
	tools, err := s.tools(ctx)
	if err != nil {
		return nil, err
	}
	var out []AppEntrypoint
	for _, tool := range tools {
		if ui := ParseUIMeta(tool.Meta); ui.Thread {
			out = append(out, AppEntrypoint{ToolName: tool.Name, Title: ToolTitle(tool), ResourceURI: ui.ResourceURI})
		}
	}
	return out, nil
}

// CallAppTool calls a tool for a view. The tool visibility must include
// "app".
func (s *AppSession) CallAppTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	tool, err := s.tool(ctx, name)
	if err != nil {
		return nil, err
	}
	if !ParseUIMeta(tool.Meta).Allows(VisibilityApp) {
		return nil, ErrAppToolNotAllowed
	}
	return s.call(ctx, name, args)
}

// OpenEntrypoint calls the tool of a thread entry point with {}. The
// tool visibility does not apply, as the OpenAI spec says.
func (s *AppSession) OpenEntrypoint(ctx context.Context, name string) (codersdk.ChatMCPApp, error) {
	tool, err := s.tool(ctx, name)
	if err != nil {
		return codersdk.ChatMCPApp{}, err
	}
	ui := ParseUIMeta(tool.Meta)
	if !ui.Thread {
		return codersdk.ChatMCPApp{}, ErrAppToolNotFound
	}
	result, err := s.call(ctx, name, map[string]any{})
	if err != nil {
		return codersdk.ChatMCPApp{}, err
	}
	app := codersdk.ChatMCPApp{
		MCPServerConfigID: s.srv.ID,
		ToolName:          tool.Name,
		Title:             ToolTitle(tool),
		ResourceURI:       ui.ResourceURI,
		ToolInput:         map[string]any{},
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return codersdk.ChatMCPApp{}, xerrors.Errorf("encode result: %w", err)
	}
	if err := json.Unmarshal(raw, &app.ToolResult); err != nil {
		return codersdk.ChatMCPApp{}, xerrors.Errorf("decode result: %w", err)
	}
	return app, nil
}

func (s *AppSession) call(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, toolCallTimeout)
	defer cancel()
	result, err := s.session.CallTool(callCtx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, xerrors.Errorf("call tool: %w", err)
	}
	if raw, err := json.Marshal(result); err == nil && len(raw) > maxAppToolResultBytes {
		return nil, xerrors.Errorf("%w: %d bytes", ErrAppResultTooLarge, len(raw))
	}
	return result, nil
}

// tool finds a tool by name. The allow and deny lists of the server
// config apply, as they do for the model.
func (s *AppSession) tool(ctx context.Context, name string) (*mcp.Tool, error) {
	tools, err := s.tools(ctx)
	if err != nil {
		return nil, err
	}
	for _, tool := range tools {
		if tool.Name == name {
			return tool, nil
		}
	}
	return nil, ErrAppToolNotFound
}

func (s *AppSession) tools(ctx context.Context) ([]*mcp.Tool, error) {
	var out []*mcp.Tool
	for tool, err := range s.session.Tools(ctx, nil) {
		if err != nil {
			return nil, xerrors.Errorf("list tools: %w", err)
		}
		if tool == nil {
			return nil, xerrors.New("MCP server returned a null tool definition")
		}
		if isToolAllowed(tool.Name, s.srv.ToolAllowList, s.srv.ToolDenyList) {
			out = append(out, tool)
		}
	}
	return out, nil
}
