package agentselect

import (
	"cmp"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// chatAgentSuffix is a PoC naming convention, not a long-term contract.
// Template authors can name one root coder_agent with this suffix to
// direct chat traffic to that agent. If the PoC succeeds, revisit with
// an explicit field or role-based model.
const chatAgentSuffix = "-coderd-chat"

// SelectChatAgent picks the best workspace agent for a chat session from
// the provided candidates. It applies these rules in order:
//  1. Filter to root agents only (ParentID is null).
//  2. Sort stably and deterministically by DisplayOrder ASC, then
//     Name ASC (case-insensitive), then Name ASC.
//  3. If exactly one root agent name ends with "-coderd-chat"
//     (case-insensitive), return it.
//  4. If zero root agents match the suffix, return the first root agent
//     after sorting (deterministic fallback).
//  5. If more than one root agent matches the suffix, return an error
//     with an actionable message.
//  6. If no root agents exist at all, return an error.
func SelectChatAgent(
	agents []database.WorkspaceAgent,
) (database.WorkspaceAgent, error) {
	rootAgents := make([]database.WorkspaceAgent, 0, len(agents))
	for _, agent := range agents {
		if agent.ParentID.Valid {
			continue
		}
		rootAgents = append(rootAgents, agent)
	}

	if len(rootAgents) == 0 {
		return database.WorkspaceAgent{}, xerrors.New(
			"no eligible workspace agents found",
		)
	}

	slices.SortStableFunc(rootAgents, func(a, b database.WorkspaceAgent) int {
		if order := cmp.Compare(a.DisplayOrder, b.DisplayOrder); order != 0 {
			return order
		}
		if order := cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); order != 0 {
			return order
		}
		return cmp.Compare(a.Name, b.Name)
	})

	matchingAgents := make([]database.WorkspaceAgent, 0, 1)
	for _, agent := range rootAgents {
		if strings.HasSuffix(strings.ToLower(agent.Name), chatAgentSuffix) {
			matchingAgents = append(matchingAgents, agent)
		}
	}

	switch len(matchingAgents) {
	case 0:
		return rootAgents[0], nil
	case 1:
		return matchingAgents[0], nil
	default:
		names := make([]string, 0, len(matchingAgents))
		for _, agent := range matchingAgents {
			names = append(names, agent.Name)
		}
		return database.WorkspaceAgent{}, xerrors.Errorf(
			"multiple agents match the chat suffix %q: %s; only one agent should use this suffix",
			chatAgentSuffix,
			strings.Join(names, ", "),
		)
	}
}
