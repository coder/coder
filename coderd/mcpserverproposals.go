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

// getMCPServerProposal returns an MCP server proposal for review.
//
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Get MCP server proposal
// @ID get-mcp-server-proposal
// @Security CoderSessionToken
// @Produce json
// @Tags MCP
// @Param mcpProposal path string true "MCP server proposal ID" format(uuid)
// @Success 200 {object} codersdk.MCPServerProposal
// @Router /api/v2/mcp-server-proposals/{mcpProposal} [get]
// @x-apidocgen {"skip": true}
// nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getMCPServerProposal(rw http.ResponseWriter, r *http.Request) {
	if !api.mcpProposalsAvailable(rw, r) {
		return
	}
	api.mcpProposals.GetProposal(rw, r)
}

// acceptMCPServerProposal accepts an MCP server proposal.
//
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Accept MCP server proposal
// @ID accept-mcp-server-proposal
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags MCP
// @Param mcpProposal path string true "MCP server proposal ID" format(uuid)
// @Param request body codersdk.AcceptMCPServerProposalRequest false "MCP server proposal input values"
// @Success 200 {object} codersdk.AcceptMCPServerProposalResponse
// @Router /api/v2/mcp-server-proposals/{mcpProposal}/accept [post]
// @x-apidocgen {"skip": true}
func (api *API) acceptMCPServerProposal(rw http.ResponseWriter, r *http.Request) {
	if !api.mcpProposalsAvailable(rw, r) {
		return
	}
	api.mcpProposals.AcceptProposal(rw, r)
}

// rejectMCPServerProposal rejects an MCP server proposal.
//
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Reject MCP server proposal
// @ID reject-mcp-server-proposal
// @Security CoderSessionToken
// @Tags MCP
// @Param mcpProposal path string true "MCP server proposal ID" format(uuid)
// @Success 204
// @Router /api/v2/mcp-server-proposals/{mcpProposal}/reject [post]
// @x-apidocgen {"skip": true}
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
