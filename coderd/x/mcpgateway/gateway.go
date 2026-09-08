// Package mcpgateway is a prototype MCP Gateway. It exposes one MCP endpoint
// per organization that aggregates the MCP servers the requesting user may
// read, filters tools by the per-server allow and deny lists, and proxies tool
// calls to the upstream servers with the user's stored credentials.
//
// This is a proof of feasibility. It connects to every upstream on every
// request and keeps no state between requests.
package mcpgateway

import (
	"net/http"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

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
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	})
}
