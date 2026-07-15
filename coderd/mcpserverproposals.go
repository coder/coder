package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// The MCP server proposal endpoints are implemented by
// slackd.ProposalsAPI; these wrappers exist so the routes carry API
// documentation annotations and degrade gracefully when the proposals
// API failed to construct.

// @Summary Get MCP server proposal
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getMCPServerProposal(rw http.ResponseWriter, r *http.Request) {
	if !api.mcpProposalsAvailable(rw, r) {
		return
	}
	api.mcpProposals.GetProposal(rw, r)
}

// @Summary Accept MCP server proposal
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) acceptMCPServerProposal(rw http.ResponseWriter, r *http.Request) {
	if !api.mcpProposalsAvailable(rw, r) {
		return
	}
	api.mcpProposals.AcceptProposal(rw, r)
}

// @Summary Reject MCP server proposal
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) rejectMCPServerProposal(rw http.ResponseWriter, r *http.Request) {
	if !api.mcpProposalsAvailable(rw, r) {
		return
	}
	api.mcpProposals.RejectProposal(rw, r)
}

//nolint:revive // Helper writes to ResponseWriter on failure.
func (api *API) mcpProposalsAvailable(rw http.ResponseWriter, r *http.Request) bool {
	if api.mcpProposals != nil {
		return true
	}
	httpapi.Write(r.Context(), rw, http.StatusServiceUnavailable, codersdk.Response{
		Message: "MCP server proposals are not available on this deployment.",
	})
	return false
}
