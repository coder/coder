package chatd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// workspaceMCPView is the single projection of a chat's workspace MCP
// state: discovery completeness from the bound agent's latest snapshot,
// freshness from the agent run ids, and per-source outcomes plus
// eligible tools from the chat's pinned MCP rows. The generation path,
// the chat GET response, and find_tools all read this one view so the
// model and the user see the same inventory.
type workspaceMCPView struct {
	phase codersdk.ChatContextMCPDiscoveryPhase
	// stale means the pinned MCP rows were published by a previous agent
	// process. Both run ids must be non-empty to prove it; when either is
	// empty nothing can be proven and nothing is withheld.
	stale bool
	// tools is the eligible tool set: every tool on an OK mcp_server row
	// unless the view is stale.
	tools   []workspacesdk.MCPToolInfo
	servers []workspaceMCPServerOutcome
	configs []workspaceMCPConfigOutcome
}

// workspaceMCPServerOutcome is one mcp_server row's discovery result.
type workspaceMCPServerOutcome struct {
	name      string
	ok        bool
	toolCount int
	// diagnostic is the error for a failed server or the warning on a
	// usable one.
	diagnostic string
}

// workspaceMCPConfigOutcome is one non-OK mcp_config row.
type workspaceMCPConfigOutcome struct {
	path       string
	diagnostic string
}

// buildWorkspaceMCPView combines the agent row (zero when the chat has no
// bound agent), its latest snapshot (nil when it has not pushed yet), and
// the chat's pinned rows. Without an agent the phase is unknown and nothing
// can be proven stale, so the pinned rows are projected as they are.
func buildWorkspaceMCPView(agent database.WorkspaceAgent, snapshot *database.WorkspaceAgentContextSnapshot, pinned []database.ChatContextResource) workspaceMCPView {
	view := workspaceMCPView{phase: codersdk.ChatContextMCPDiscoveryPhaseUnknown}
	switch {
	case agent.ID == uuid.Nil:
	case snapshot == nil:
		// A current agent that has not pushed yet is still discovering; a
		// legacy agent (no run id) gives no guarantee either way.
		if agent.AgentRunID != "" {
			view.phase = codersdk.ChatContextMCPDiscoveryPhasePending
		}
	case snapshot.McpDiscoveryPhase == database.WorkspaceAgentMcpDiscoveryPhasePending:
		view.phase = codersdk.ChatContextMCPDiscoveryPhasePending
	case snapshot.McpDiscoveryPhase == database.WorkspaceAgentMcpDiscoveryPhaseComplete:
		view.phase = codersdk.ChatContextMCPDiscoveryPhaseComplete
	}
	if snapshot != nil {
		view.stale = workspaceMCPSnapshotStale(agent.AgentRunID, snapshot.AgentRunID)
	}
	view.projectPinned(pinned)
	return view
}

// buildReplacedAgentMCPView projects rows pinned from an agent that a later
// workspace build replaced. Its snapshot is gone, so the phase is unknown,
// and the rows describe a previous agent process, so they are stale and
// their tools are withheld until the next turn rebinds the chat.
func buildReplacedAgentMCPView(pinned []database.ChatContextResource) workspaceMCPView {
	view := workspaceMCPView{phase: codersdk.ChatContextMCPDiscoveryPhaseUnknown, stale: true}
	view.projectPinned(pinned)
	return view
}

// projectPinned fills the per-source outcomes and, unless the view is
// stale, the eligible tools from the chat's pinned MCP rows.
func (v *workspaceMCPView) projectPinned(pinned []database.ChatContextResource) {
	for _, r := range pinned {
		switch r.BodyKind {
		case database.WorkspaceAgentContextBodyKindMcpServer:
			outcome := workspaceMCPServerOutcome{
				name:       r.Source,
				ok:         r.Status == database.WorkspaceAgentContextResourceStatusOk,
				diagnostic: r.Error,
			}
			if outcome.ok {
				infos := workspaceMCPToolInfosFromResources([]database.ChatContextResource{r})
				outcome.toolCount = len(infos)
				if !v.stale {
					v.tools = append(v.tools, infos...)
				}
			}
			v.servers = append(v.servers, outcome)
		case database.WorkspaceAgentContextBodyKindMcpConfig:
			if r.Status != database.WorkspaceAgentContextResourceStatusOk {
				v.configs = append(v.configs, workspaceMCPConfigOutcome{path: r.Source, diagnostic: r.Error})
			}
		default:
		}
	}
}

// workspaceMCPSnapshotStale is the freshness rule: stale only when both
// run ids are known and differ. Timestamps are deliberately not compared
// because the snapshot and agent clocks are different machines.
func workspaceMCPSnapshotStale(agentRunID, snapshotRunID string) bool {
	return agentRunID != "" && snapshotRunID != "" && agentRunID != snapshotRunID
}

// loadWorkspaceMCPView lists the chat's pinned rows and builds the view.
func (p *Server) loadWorkspaceMCPView(ctx context.Context, chat database.Chat) (workspaceMCPView, error) {
	pinned, err := p.db.ListChatContextResourcesByChatID(ctx, chat.ID)
	if err != nil {
		return workspaceMCPView{}, xerrors.Errorf("list chat context resources: %w", err)
	}
	return p.workspaceMCPViewForPinned(ctx, chat, pinned)
}

// workspaceMCPViewForPinned reads the bound agent and its latest snapshot
// and combines them with rows the caller already listed.
func (p *Server) workspaceMCPViewForPinned(ctx context.Context, chat database.Chat, pinned []database.ChatContextResource) (workspaceMCPView, error) {
	if !chat.AgentID.Valid {
		return buildWorkspaceMCPView(database.WorkspaceAgent{}, nil, pinned), nil
	}
	agent, err := p.db.GetWorkspaceAgentByID(ctx, chat.AgentID.UUID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// A bound agent that no longer resolves was soft-deleted by a later
		// workspace build (SoftDeletePriorWorkspaceAgents also purges its
		// snapshot). The chat stays bound until its next turn rebinds it, so
		// project the pinned rows as stale instead of hiding the whole context.
		return buildReplacedAgentMCPView(pinned), nil
	case err != nil:
		return workspaceMCPView{}, xerrors.Errorf("get workspace agent: %w", err)
	}
	var snapshot *database.WorkspaceAgentContextSnapshot
	latest, err := p.db.GetLatestWorkspaceAgentContextSnapshot(ctx, agent.ID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return workspaceMCPView{}, xerrors.Errorf("get latest snapshot: %w", err)
	default:
		snapshot = &latest
	}
	return buildWorkspaceMCPView(agent, snapshot, pinned), nil
}

// Tools returns the eligible workspace MCP tool definitions: empty when
// the view is stale, because those definitions belong to a previous agent
// process.
func (v workspaceMCPView) Tools() []workspacesdk.MCPToolInfo {
	return v.tools
}

// Discovery is the SDK projection of the view's completeness and
// freshness.
func (v workspaceMCPView) Discovery() *codersdk.ChatContextMCPDiscovery {
	return &codersdk.ChatContextMCPDiscovery{Phase: v.phase, Stale: v.stale}
}

// Incomplete reports whether the model should be told about discovery:
// discovery is pending, the rows are stale, or a declared source failed.
func (v workspaceMCPView) Incomplete() bool {
	if v.phase == codersdk.ChatContextMCPDiscoveryPhasePending || v.stale || len(v.configs) > 0 {
		return true
	}
	for _, s := range v.servers {
		if !s.ok {
			return true
		}
	}
	return false
}

// Summary renders the view as one model-facing line. It describes
// discovery outcomes only and never claims a server is healthy.
func (v workspaceMCPView) Summary() string {
	var sentences []string
	switch {
	case v.stale:
		sentences = append(sentences, "Workspace MCP tools were published by a previous agent process and are withheld until the current process publishes its discovery.")
	case v.phase == codersdk.ChatContextMCPDiscoveryPhasePending:
		sentences = append(sentences, "Workspace MCP discovery is still initializing; more servers may appear on a later turn.")
	case v.phase == codersdk.ChatContextMCPDiscoveryPhaseComplete:
		sentences = append(sentences, "Workspace MCP discovery is complete.")
	default:
		sentences = append(sentences, "Workspace MCP discovery state is unknown.")
	}
	var usable, failed []string
	for _, s := range v.servers {
		switch {
		case s.ok && s.diagnostic != "":
			usable = append(usable, fmt.Sprintf("%s (%d tools, warning: %s)", s.name, s.toolCount, s.diagnostic))
		case s.ok:
			usable = append(usable, fmt.Sprintf("%s (%d tools)", s.name, s.toolCount))
		default:
			failed = append(failed, fmt.Sprintf("%s (%s)", s.name, s.diagnostic))
		}
	}
	if len(usable) > 0 {
		sentences = append(sentences, fmt.Sprintf("Discovered servers: %s.", strings.Join(usable, "; ")))
	}
	if len(failed) > 0 {
		sentences = append(sentences, fmt.Sprintf("Failed servers: %s.", strings.Join(failed, "; ")))
	}
	if len(v.configs) > 0 {
		parts := make([]string, 0, len(v.configs))
		for _, c := range v.configs {
			parts = append(parts, fmt.Sprintf("%s (%s)", c.path, c.diagnostic))
		}
		sentences = append(sentences, fmt.Sprintf("Invalid MCP config files: %s.", strings.Join(parts, "; ")))
	}
	return strings.Join(sentences, " ")
}
