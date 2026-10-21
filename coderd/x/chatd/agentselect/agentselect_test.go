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

func TestFirstChatAgent(t *testing.T) {
	t.Parallel()

	child := database.WorkspaceAgent{ID: uuid.New(), Name: "child" + agentselect.Suffix, ParentID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
	plain := database.WorkspaceAgent{ID: uuid.New(), Name: "aaa"}
	late := database.WorkspaceAgent{ID: uuid.New(), Name: "alpha" + agentselect.Suffix, DisplayOrder: 1}
	beta := database.WorkspaceAgent{ID: uuid.New(), Name: "beta" + agentselect.Suffix}
	gamma := database.WorkspaceAgent{ID: uuid.New(), Name: "Gamma" + agentselect.Suffix}

	agent, ok := agentselect.FirstChatAgent([]database.WorkspaceAgent{child, plain, late, gamma, beta})
	require.True(t, ok)
	require.Equal(t, beta, agent)

	_, ok = agentselect.FirstChatAgent([]database.WorkspaceAgent{child, plain})
	require.False(t, ok)
}

func TestForChat(t *testing.T) {
	t.Parallel()

	main := database.WorkspaceAgent{ID: uuid.New(), Name: "main", DisplayOrder: 0}
	chatAgent := database.WorkspaceAgent{ID: uuid.New(), Name: "dev" + agentselect.Suffix, DisplayOrder: 1}
	child := database.WorkspaceAgent{
		ID:       uuid.New(),
		Name:     "child" + agentselect.Suffix,
		ParentID: uuid.NullUUID{UUID: main.ID, Valid: true},
	}
	otherChatAgent := database.WorkspaceAgent{ID: uuid.New(), Name: "other" + agentselect.Suffix, DisplayOrder: 2}
	bound := func(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: true} }

	tests := []struct {
		name    string
		agentID uuid.NullUUID
		agents  []database.WorkspaceAgent
		want    uuid.UUID
		wantErr error
	}{
		{name: "UnboundPicksSuffixedRootOverMain", agents: []database.WorkspaceAgent{main, chatAgent, child}, want: chatAgent.ID},
		{name: "BoundAgentPreferred", agentID: bound(main.ID), agents: []database.WorkspaceAgent{main, chatAgent}, want: main.ID},
		{name: "BoundChildAgentPreferred", agentID: bound(child.ID), agents: []database.WorkspaceAgent{main, chatAgent, child}, want: child.ID},
		{name: "StaleBindingFallsBack", agentID: bound(uuid.New()), agents: []database.WorkspaceAgent{main, chatAgent}, want: chatAgent.ID},
		{name: "StoppedWorkspaceHasNoAgent", agentID: bound(main.ID), wantErr: agentselect.ErrNoEligibleAgents},
		{name: "AmbiguousSuffixedRoots", agents: []database.WorkspaceAgent{main, chatAgent, otherChatAgent}, wantErr: agentselect.ErrMultipleChatAgents},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := agentselect.ForChat(database.Chat{AgentID: tt.agentID}, tt.agents)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got.ID)
		})
	}
}
