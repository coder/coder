package agentselect_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
)

func TestFindChatAgent(t *testing.T) {
	t.Parallel()

	newRootAgentWithID := func(id, name string, displayOrder int32) database.WorkspaceAgent {
		return database.WorkspaceAgent{
			ID:           uuid.MustParse(id),
			Name:         name,
			DisplayOrder: displayOrder,
		}
	}

	newRootAgent := func(name string, displayOrder int32) database.WorkspaceAgent {
		return newRootAgentWithID(uuid.NewString(), name, displayOrder)
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
			name: "CaseOnlyNameTieFallbackDeterministic",
			agents: []database.WorkspaceAgent{
				newRootAgent("Dev", 0),
				newRootAgent("dev", 0),
			},
			wantIndex: 0,
		},
		{
			name: "ExactNameTieFallbackByID",
			agents: []database.WorkspaceAgent{
				newRootAgentWithID("00000000-0000-0000-0000-000000000002", "dev", 0),
				newRootAgentWithID("00000000-0000-0000-0000-000000000001", "dev", 0),
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
				fmt.Sprintf(
					"multiple agents match the chat suffix %q",
					agentselect.Suffix,
				),
				"alpha-coderd-chat",
				"beta-coderd-chat",
				"only one agent should use this suffix",
			},
		},
		{
			name: "SubAgentPreferredOverRoot",
			agents: []database.WorkspaceAgent{
				newRootAgent("host", 0),
				newChildAgent("devcontainer", 1),
			},
			wantIndex: 1,
		},
		{
			name: "SubAgentSuffixMatchWins",
			agents: []database.WorkspaceAgent{
				newRootAgent("alpha", 0),
				newChildAgent("dev-coderd-chat", 1),
			},
			wantIndex: 1,
		},
		{
			name: "RootSuffixMatchWinsOverSubAgent",
			agents: []database.WorkspaceAgent{
				newChildAgent("alpha", 1),
				newRootAgent("dev-coderd-chat", 0),
			},
			wantIndex: 1,
		},
		{
			name: "MultipleSubAgentsSortedDeterministically",
			agents: []database.WorkspaceAgent{
				newChildAgent("zeta", 0),
				newChildAgent("alpha", 0),
			},
			wantIndex: 1,
		},
		{
			name: "OnlySubAgents",
			agents: []database.WorkspaceAgent{
				newChildAgent("zeta", 0),
				newChildAgent("alpha", 0),
			},
			wantIndex: 1,
		},
		{
			name: "MultipleSuffixMatchesAcrossRootAndSubAgentError",
			agents: []database.WorkspaceAgent{
				newRootAgent("alpha-coderd-chat", 0),
				newChildAgent("beta-coderd-chat", 1),
			},
			wantErrContains: []string{
				fmt.Sprintf(
					"multiple agents match the chat suffix %q",
					agentselect.Suffix,
				),
				"alpha-coderd-chat",
				"beta-coderd-chat",
				"only one agent should use this suffix",
			},
		},
		{
			name:   "EmptyAgentList",
			agents: []database.WorkspaceAgent{},
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

			got, err := agentselect.FindChatAgent(tt.agents)
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

func TestIsChatAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "ExactSuffix",
			input: "agent-coderd-chat",
			want:  true,
		},
		{
			name:  "UppercaseSuffix",
			input: "agent-CODERD-CHAT",
			want:  true,
		},
		{
			name:  "MixedCaseSuffix",
			input: "agent-Coderd-Chat",
			want:  true,
		},
		{
			name:  "NoSuffix",
			input: "my-agent",
			want:  false,
		},
		{
			name:  "SuffixOnly",
			input: "-coderd-chat",
			want:  true,
		},
		{
			name:  "PartialSuffix",
			input: "agent-coderd",
			want:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, agentselect.IsChatAgent(tt.input))
		})
	}
}
