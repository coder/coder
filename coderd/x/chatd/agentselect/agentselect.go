package agentselect

import (
	"cmp"
	"slices"
	"strings"

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
//  1. Filter to root agents only (ParentID is null).
//  2. Sort stably and deterministically by DisplayOrder ASC, then Name ASC
//     (case-insensitive), then Name ASC, then ID ASC.
//  3. If exactly one root agent name ends with Suffix (case-insensitive),
//     return it.
//  4. If zero root agents match the suffix, return the first root agent after
//     sorting (deterministic fallback).
//  5. If more than one root agent matches the suffix, return an error with an
//     actionable message.
//  6. If no root agents exist at all, return an error.
func FindChatAgent(
	agents []database.WorkspaceAgent,
) (database.WorkspaceAgent, error) {
	rootAgents := make([]database.WorkspaceAgent, 0, len(agents))
	matchingAgents := make([]database.WorkspaceAgent, 0, 1)
	for _, agent := range agents {
		if agent.ParentID.Valid {
			continue
		}
		rootAgents = append(rootAgents, agent)
		if IsChatAgent(agent.Name) {
			matchingAgents = append(matchingAgents, agent)
		}
	}

	if len(rootAgents) == 0 {
		return database.WorkspaceAgent{}, xerrors.New(
			"no eligible workspace agents found",
		)
	}

	compareAgents := func(a, b database.WorkspaceAgent) int {
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
	}
	slices.SortStableFunc(rootAgents, compareAgents)
	slices.SortStableFunc(matchingAgents, compareAgents)

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
			Suffix,
			strings.Join(names, ", "),
		)
	}
}
