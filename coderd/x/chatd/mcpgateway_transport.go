package chatd

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
)

// mcpGatewayHost is a placeholder host for in-process gateway requests.
// It is never dialed: mcpGatewayRoundTripper serves the request.
const mcpGatewayHost = "http://mcp-gateway.internal"

// mcpGatewayConfig returns the synthetic MCP server config that makes
// the gateway look like an ordinary upstream to mcpclient.ConnectAll,
// so the tools it aggregates arrive as gw__<slug>__<tool>.
func mcpGatewayConfig(orgID uuid.UUID) database.MCPServerConfig {
	return database.MCPServerConfig{
		Slug:      "gw",
		Url:       mcpGatewayHost + "/organizations/" + orgID.String() + "/mcp-gateway",
		Transport: "streamable_http",
		AuthType:  "none",
		Enabled:   true,
	}
}

// mcpGatewayClient returns an HTTP client that serves every request
// with the gateway handler in-process under the chat owner's actor.
// chatd holds no user credential, so a loopback HTTP call would have
// nothing to authenticate with.
func (server *Server) mcpGatewayClient(ctx context.Context, ownerID uuid.UUID) (*http.Client, error) {
	//nolint:gocritic // Resolving a user's RBAC subject requires system access.
	owner, _, err := httpmw.UserRBACSubject(dbauthz.AsSystemRestricted(ctx), server.db, ownerID, rbac.ScopeAll)
	if err != nil {
		return nil, xerrors.Errorf("resolve chat owner RBAC subject: %w", err)
	}

	router := chi.NewRouter()
	router.Route("/organizations/{organization}", func(r chi.Router) {
		r.Use(httpmw.ExtractOrganizationParam(server.db))
		r.Mount("/mcp-gateway", server.mcpGateway)
	})
	return &http.Client{Transport: &mcpGatewayRoundTripper{
		handler: router,
		actor:   owner,
	}}, nil
}

// mcpGatewayRoundTripper serves requests with an http.Handler instead
// of dialing. The actor is injected here because the gateway and the
// organization middleware both authorize from the request context.
type mcpGatewayRoundTripper struct {
	handler http.Handler
	actor   rbac.Subject
}

func (rt *mcpGatewayRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Stateless gateway responses are one JSON body or a short SSE
	// stream that ends with the response, so buffering is enough.
	recorder := httptest.NewRecorder()
	rt.handler.ServeHTTP(recorder, req.WithContext(dbauthz.As(req.Context(), rt.actor)))
	res := recorder.Result()
	res.Request = req
	return res, nil
}
