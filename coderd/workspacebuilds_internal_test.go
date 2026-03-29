package coderd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestIsChatAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		agentName string
		want      bool
	}{
		{
			name:      "ExactSuffix",
			agentName: "workspace-coderd-chat",
			want:      true,
		},
		{
			name:      "CaseInsensitive",
			agentName: "workspace-CODERD-CHAT",
			want:      true,
		},
		{
			name:      "NoSuffix",
			agentName: "workspace-agent",
			want:      false,
		},
		{
			name:      "EmptyString",
			agentName: "",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, isChatAgent(tt.agentName))
		})
	}
}

func TestHiddenChatAgentIDsFromAgents(t *testing.T) {
	t.Parallel()

	newAgent := func(name string, parentID uuid.NullUUID) database.WorkspaceAgent {
		return database.WorkspaceAgent{
			ID:       uuid.New(),
			Name:     name,
			ParentID: parentID,
		}
	}

	tests := []struct {
		name  string
		setup func() ([]database.WorkspaceAgent, map[uuid.UUID]struct{})
	}{
		{
			name: "NoChatAgents",
			setup: func() ([]database.WorkspaceAgent, map[uuid.UUID]struct{}) {
				return []database.WorkspaceAgent{newAgent("workspace-agent", uuid.NullUUID{})}, map[uuid.UUID]struct{}{}
			},
		},
		{
			name: "SingleChatRoot",
			setup: func() ([]database.WorkspaceAgent, map[uuid.UUID]struct{}) {
				chatRoot := newAgent("workspace-coderd-chat", uuid.NullUUID{})
				return []database.WorkspaceAgent{chatRoot}, map[uuid.UUID]struct{}{
					chatRoot.ID: {},
				}
			},
		},
		{
			name: "ChatRootWithChildren",
			setup: func() ([]database.WorkspaceAgent, map[uuid.UUID]struct{}) {
				chatRoot := newAgent("workspace-coderd-chat", uuid.NullUUID{})
				child := newAgent("workspace-agent", uuid.NullUUID{UUID: chatRoot.ID, Valid: true})
				grandchild := newAgent("workspace-sidecar", uuid.NullUUID{UUID: child.ID, Valid: true})
				visibleRoot := newAgent("workspace-main", uuid.NullUUID{})
				return []database.WorkspaceAgent{chatRoot, child, grandchild, visibleRoot}, map[uuid.UUID]struct{}{
					chatRoot.ID:   {},
					child.ID:      {},
					grandchild.ID: {},
				}
			},
		},
		{
			name: "NonRootChatAgentIsVisible",
			setup: func() ([]database.WorkspaceAgent, map[uuid.UUID]struct{}) {
				root := newAgent("workspace-main", uuid.NullUUID{})
				childChat := newAgent("workspace-coderd-chat", uuid.NullUUID{UUID: root.ID, Valid: true})
				return []database.WorkspaceAgent{root, childChat}, map[uuid.UUID]struct{}{}
			},
		},
		{
			name: "MultipleChatRoots",
			setup: func() ([]database.WorkspaceAgent, map[uuid.UUID]struct{}) {
				chatRootOne := newAgent("workspace-coderd-chat", uuid.NullUUID{})
				chatChildOne := newAgent("workspace-agent", uuid.NullUUID{UUID: chatRootOne.ID, Valid: true})
				chatRootTwo := newAgent("workspace-CODERD-CHAT", uuid.NullUUID{})
				chatChildTwo := newAgent("workspace-sidecar", uuid.NullUUID{UUID: chatRootTwo.ID, Valid: true})
				visibleRoot := newAgent("workspace-main", uuid.NullUUID{})
				return []database.WorkspaceAgent{chatRootOne, chatChildOne, chatRootTwo, chatChildTwo, visibleRoot}, map[uuid.UUID]struct{}{
					chatRootOne.ID:  {},
					chatChildOne.ID: {},
					chatRootTwo.ID:  {},
					chatChildTwo.ID: {},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			agents, wantHidden := tt.setup()
			require.Equal(t, wantHidden, hiddenChatAgentIDsFromAgents(agents))
		})
	}
}
