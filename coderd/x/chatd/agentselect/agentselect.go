package agentselect

import (
	"cmp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// Suffix marks chat-designated agents during the current PoC. This naming
// convention is an implementation detail, not a stable contract.
const Suffix = "-coderd-chat"

// IsChatAgent reports whether name uses the chat-agent suffix convention.
func IsChatAgent(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), Suffix)
}

// FindChatAgent picks the best workspace agent for a chat session from the
// provided candidates. It applies these rules in order:
//  1. Pick a root agent (ParentID is null) with pickAgent.
//  2. If the picked root agent has child agents, such as devcontainer
//     agents, pick one of them with pickAgent instead, so chats run with
//     the devcontainer's tools and environment.
//  3. If no root agents exist at all, return an error.
func FindChatAgent(
	agents []database.WorkspaceAgent,
) (database.WorkspaceAgent, error) {
	rootAgents := make([]database.WorkspaceAgent, 0, len(agents))
	childAgents := make(map[uuid.UUID][]database.WorkspaceAgent)
	for _, agent := range agents {
		if agent.ParentID.Valid {
			childAgents[agent.ParentID.UUID] = append(childAgents[agent.ParentID.UUID], agent)
			continue
		}
		rootAgents = append(rootAgents, agent)
	}

	if len(rootAgents) == 0 {
		return database.WorkspaceAgent{}, xerrors.New(
			"no eligible workspace agents found",
		)
	}

	root, err := pickAgent(rootAgents)
	if err != nil {
		return database.WorkspaceAgent{}, err
	}
	if children := childAgents[root.ID]; len(children) > 0 {
		return pickAgent(children)
	}
	return root, nil
}

// pickAgent picks one agent from a non-empty set of sibling agents:
//  1. Sort stably and deterministically by DisplayOrder ASC, then Name ASC
//     (case-insensitive), then Name ASC, then ID ASC.
//  2. If exactly one agent name ends with Suffix (case-insensitive),
//     return it.
//  3. If zero agents match the suffix, return the first agent after
//     sorting (deterministic fallback).
//  4. If more than one agent matches the suffix, return an error with an
//     actionable message.
func pickAgent(agents []database.WorkspaceAgent) (database.WorkspaceAgent, error) {
	sorted := slices.Clone(agents)
	slices.SortStableFunc(sorted, func(a, b database.WorkspaceAgent) int {
		if order := cmp.Compare(a.DisplayOrder, b.DisplayOrder); order != 0 {
			return order
		}
		if order := cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); order != 0 {
			return order
		}
		if order := cmp.Compare(a.Name, b.Name); order != 0 {
			return order
		}
		return cmp.Compare(a.ID.String(), b.ID.String())
	})

	matchingAgents := make([]database.WorkspaceAgent, 0, 1)
	for _, agent := range sorted {
		if IsChatAgent(agent.Name) {
			matchingAgents = append(matchingAgents, agent)
		}
	}

	switch len(matchingAgents) {
	case 0:
		return sorted[0], nil
	case 1:
		return matchingAgents[0], nil
	default:
		names := make([]string, 0, len(matchingAgents))
		for _, agent := range matchingAgents {
			names = append(names, agent.Name)
		}
		return database.WorkspaceAgent{}, xerrors.Errorf(
			"multiple agents match the chat suffix %q: %s; only one agent should use this suffix",
			Suffix,
			strings.Join(names, ", "),
		)
	}
}
