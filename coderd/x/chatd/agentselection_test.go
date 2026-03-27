//nolint:testpackage // Tests the unexported helper without widening its API.
package chatd

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestSelectChatAgent(t *testing.T) {
	t.Parallel()

	newRootAgent := func(name string, displayOrder int32) database.WorkspaceAgent {
		return database.WorkspaceAgent{
			ID:           uuid.New(),
			Name:         name,
			DisplayOrder: displayOrder,
		}
	}

	newChildAgent := func(name string, displayOrder int32) database.WorkspaceAgent {
		agent := newRootAgent(name, displayOrder)
		agent.ParentID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
		return agent
	}

	tests := []struct {
		name            string
		agents          []database.WorkspaceAgent
		wantIndex       int
		wantErrContains []string
	}{
		{
			name: "SingleSuffixMatch",
			agents: []database.WorkspaceAgent{
				newRootAgent("alpha", 0),
				newRootAgent("dev-coderd-chat", 2),
				newRootAgent("zeta", 1),
			},
			wantIndex: 1,
		},
		{
			name: "SuffixMatchCaseInsensitive",
			agents: []database.WorkspaceAgent{
				newRootAgent("alpha", 0),
				newRootAgent("Dev-Coderd-Chat", 2),
				newRootAgent("zeta", 1),
			},
			wantIndex: 1,
		},
		{
			name: "NoSuffixMatchFallbackDeterministic",
			agents: []database.WorkspaceAgent{
				newRootAgent("zeta", 2),
				newRootAgent("bravo", 1),
				newRootAgent("alpha", 1),
			},
			wantIndex: 2,
		},
		{
			name: "NoSuffixMatchFallbackByName",
			agents: []database.WorkspaceAgent{
				newRootAgent("Bravo", 3),
				newRootAgent("alpha", 3),
				newRootAgent("charlie", 3),
			},
			wantIndex: 1,
		},
		{
			name: "MultipleSuffixMatchesError",
			agents: []database.WorkspaceAgent{
				newRootAgent("alpha-coderd-chat", 2),
				newRootAgent("beta-coderd-chat", 1),
				newRootAgent("gamma", 0),
			},
			wantErrContains: []string{
				fmt.Sprintf("multiple agents match the chat suffix %q", chatAgentSuffix),
				"alpha-coderd-chat",
				"beta-coderd-chat",
				"only one agent should use this suffix",
			},
		},
		{
			name: "ChildAgentSuffixIgnored",
			agents: []database.WorkspaceAgent{
				newRootAgent("alpha", 1),
				newChildAgent("child-coderd-chat", 0),
				newRootAgent("bravo", 0),
			},
			wantIndex: 2,
		},
		{
			name: "ChildAgentSuffixIgnoredWithRootMatch",
			agents: []database.WorkspaceAgent{
				newRootAgent("alpha", 0),
				newChildAgent("child-coderd-chat", 1),
				newRootAgent("root-coderd-chat", 2),
			},
			wantIndex: 2,
		},
		{
			name:   "EmptyAgentList",
			agents: []database.WorkspaceAgent{},
			wantErrContains: []string{
				"no eligible workspace agents found",
			},
		},
		{
			name: "OnlyChildAgents",
			agents: []database.WorkspaceAgent{
				newChildAgent("alpha", 0),
				newChildAgent("beta-coderd-chat", 1),
			},
			wantErrContains: []string{
				"no eligible workspace agents found",
			},
		},
		{
			name: "SingleRootAgent",
			agents: []database.WorkspaceAgent{
				newRootAgent("solo", 5),
			},
			wantIndex: 0,
		},
		{
			name: "SuffixAgentWinsRegardlessOfOrder",
			agents: []database.WorkspaceAgent{
				newRootAgent("alpha", 0),
				newRootAgent("zeta", 1),
				newRootAgent("preferred-coderd-chat", 99),
			},
			wantIndex: 2,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := selectChatAgent(tt.agents)
			if len(tt.wantErrContains) > 0 {
				require.Error(t, err)
				for _, wantErr := range tt.wantErrContains {
					require.ErrorContains(t, err, wantErr)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.agents[tt.wantIndex], got)
		})
	}
}
