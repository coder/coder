// Package mcpbuiltin serves the MCP servers that Coder ships with, such as
// Slack and GitHub, as Streamable HTTP endpoints inside coderd.
//
// Each endpoint is a thin MCP front for one upstream API. The caller's
// bearer token is the upstream credential (for example a Slack bot token or
// a GitHub token), so the endpoint needs no Coder session. Chatd reaches the
// endpoints the same way it reaches any other MCP server, using the chat
// owner's own token. Every request must also carry Coder's signed identity
// headers, which prove an admin-made MCP server configuration sent it and
// tell the endpoints which chat is calling, so tool arguments can default
// to the conversation the chat came from.
//
// Server options travel as query parameters on the configured URL, so an
// MCP server configuration needs no extra fields to select, for example,
// a GitHub Enterprise Server API.
package mcpbuiltin

import (
	"bytes"
	"context"
	"io"
	stdslog "log/slog" //nolint:depguard // mcp.ServerOptions.Logger and the GitHub MCP server require a *log/slog.Logger.
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/safedial"
)

const (
	// ServerSlack names the built-in Slack MCP server.
	ServerSlack = "slack"
	// ServerGitHub names the built-in GitHub MCP server.
	ServerGitHub = "github"

	// RoutePrefix is the path below the experimental API where the
	// built-in servers are mounted.
	RoutePrefix = "/mcp/builtin"
	// BasePath is the absolute path below which the built-in servers
	// are served.
	BasePath = "/api/experimental" + RoutePrefix

	// upstreamTimeout bounds one upstream API call, including reading
	// the response body.
	upstreamTimeout = 60 * time.Second
	// maxRequestBodyBytes caps the MCP request body. It matches the MCP
	// SDK default so the signature check reads exactly what the SDK will.
	maxRequestBodyBytes = mcp.DefaultMaxRequestBodyBytes
)

// EndpointPath returns the path of a built-in server below the
// experimental API root, for example "/api/experimental/mcp/builtin/slack".
func EndpointPath(server string) string {
	return BasePath + "/" + server
}

// Options configures the built-in MCP servers.
type Options struct {
	Logger slog.Logger
	// AccessURL is used to link back to chats, for example in the
	// "View chat" footer of Slack messages. Optional.
	AccessURL *url.URL
	// HTTPClient is the base client for upstream API calls. Its
	// transport must be nil or an *http.Transport: it is wrapped with
	// safedial so private and link-local destinations are refused
	// unless AllowedPrefixes permits them.
	HTTPClient *http.Client
	// AllowedPrefixes exempts private destinations from the SSRF guard,
	// matching the deployment's MCP allowed private CIDRs.
	AllowedPrefixes []netip.Prefix
	// AllowInsecureHTTP permits http:// upstream URLs for loopback hosts.
	// Only tests set it.
	AllowInsecureHTTP bool
	// Identity verifies Coder's signed identity headers. When nil, the
	// headers are ignored and tools have no chat context.
	Identity IdentityVerifier
	// Conversations maps a chat to the conversation that started it.
	// When nil, tools have no default conversation.
	Conversations ConversationResolver
}

// Handler serves every built-in MCP server below one router.
type Handler struct {
	opts       Options
	logger     slog.Logger
	sdkLogger  *stdslog.Logger
	upstream   *http.Client
	schema     *mcp.SchemaCache
	slack      *slackServer
	github     *githubServer
	httpRouter chi.Router
}

// New builds the handler for all built-in MCP servers. Mount it at
// RoutePrefix below the experimental API.
func New(opts Options) *Handler {
	h := &Handler{
		opts:      opts,
		logger:    opts.Logger.Named("mcpbuiltin"),
		sdkLogger: stdslog.New(stdslog.DiscardHandler),
		upstream:  newUpstreamClient(opts.HTTPClient, opts.AllowedPrefixes),
		schema:    mcp.NewSchemaCache(),
	}
	h.slack = &slackServer{h: h}
	h.github = newGitHubServer(h)

	r := chi.NewRouter()
	r.Use(h.authenticate)
	r.Handle("/"+ServerSlack, h.slack)
	r.Handle("/"+ServerGitHub, h.github)
	h.httpRouter = r
	return h
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.httpRouter.ServeHTTP(rw, r)
}

// InProcessRoute returns a route that serves the built-in servers at
// accessURL in-process, for chatd's MCP client. middleware wraps the
// servers the same way the public mount does, for example with an
// experiment gate.
func (h *Handler) InProcessRoute(accessURL *url.URL, middleware ...func(http.Handler) http.Handler) mcpclient.InProcessRoute {
	r := chi.NewRouter()
	r.NotFound(func(rw http.ResponseWriter, _ *http.Request) { httpapi.RouteNotFound(rw) })
	r.With(middleware...).Mount(BasePath, h)
	return mcpclient.InProcessRoute{
		Origin:     accessURL,
		PathPrefix: BasePath + "/",
		Handler:    r,
	}
}

// newUpstreamClient returns an SSRF-guarded client with a bounded request
// time. Redirects must stay on the original origin so a redirect cannot
// carry the caller's token to another host.
func newUpstreamClient(base *http.Client, allowed []netip.Prefix) *http.Client {
	cloned := &http.Client{}
	if base != nil {
		// Drop the base timeout and redirect policy; both are set below.
		cloned.Transport = base.Transport
		cloned.Jar = base.Jar
	}
	cloned.Timeout = upstreamTimeout
	cloned.CheckRedirect = safedial.CheckSameOriginRedirect
	return safedial.NewHTTPClient(cloned, safedial.WithAllowedPrefixes(allowed...))
}

type requestInfoKey struct{}

// requestInfo carries what the authenticate middleware learned about one
// MCP request.
type requestInfo struct {
	// token is the upstream API credential. It must never be logged.
	token    string
	identity Identity
	// verified is true when identity came from a valid signature.
	verified bool
}

func requestInfoFrom(ctx context.Context) requestInfo {
	info, _ := ctx.Value(requestInfoKey{}).(requestInfo)
	return info
}

// authenticate requires a bearer token and a valid Coder identity
// signature on every request. The signature proves the request came from
// an MCP server configuration an org admin made, so coderd never relays
// anonymous requests to upstream APIs from its own address.
func (h *Handler) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		token := bearerToken(r)
		if token == "" {
			rw.Header().Set("WWW-Authenticate", `Bearer realm="coder-builtin-mcp"`)
			httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
				Message: "Missing upstream API token.",
				Detail:  `Send the token as "Authorization: Bearer <token>".`,
			})
			return
		}
		if r.Header.Get(HeaderCoderSignature) == "" || h.opts.Identity == nil {
			httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
				Message: "Missing Coder identity signature.",
				Detail:  "Enable forwarding Coder headers and set a signing secret on the MCP server.",
			})
			return
		}

		body, err := readBody(r)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{
				Message: "Request body is too large.",
			})
			return
		}
		identity, err := h.opts.Identity.VerifyIdentity(ctx, serverFromPath(r), r, body)
		if err != nil {
			// The cause may include database errors, so it is only logged.
			h.logger.Warn(ctx, "rejected built-in MCP request with an invalid Coder signature", slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
				Message: "Invalid Coder identity signature.",
			})
			return
		}

		info := requestInfo{token: token, identity: identity, verified: true}
		next.ServeHTTP(rw, r.WithContext(context.WithValue(ctx, requestInfoKey{}, info)))
	})
}

// serverFromPath returns the built-in server name, the last path segment.
func serverFromPath(r *http.Request) string {
	p := strings.TrimSuffix(r.URL.Path, "/")
	return p[strings.LastIndex(p, "/")+1:]
}

// readBody buffers the request body for signature verification and
// restores it for the MCP handler.
func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes+1))
	_ = r.Body.Close()
	if err != nil {
		return nil, xerrors.Errorf("read request body: %w", err)
	}
	if int64(len(body)) > maxRequestBodyBytes {
		return nil, xerrors.New("request body exceeds the maximum size")
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func bearerToken(r *http.Request) string {
	const prefix = "bearer "
	header := r.Header.Get("Authorization")
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// writeOptionError reports an invalid server option on the configured URL.
func writeOptionError(ctx context.Context, rw http.ResponseWriter, err error) {
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Invalid built-in MCP server option.",
		Detail:  err.Error(),
	})
}

// streamableHandler serves one MCP server instance for one HTTP request.
func streamableHandler(server *mcp.Server) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		// Each request builds its own server from the request's token
		// and options, so there is no session to keep.
		Stateless:    true,
		JSONResponse: true,
		// coderd often listens on loopback behind a reverse proxy, which
		// trips the SDK's DNS rebinding check. Bearer authentication is
		// the access control here.
		DisableLocalhostProtection: true,
		MaxRequestBodyBytes:        maxRequestBodyBytes,
	})
}

// textResult returns a successful tool result with one text block.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// errorResult returns a tool error the model can read and act on.
func errorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}
}
