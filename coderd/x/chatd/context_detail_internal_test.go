package chatd

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
)

func TestPinnedContextResources(t *testing.T) {
	t.Parallel()

	t.Run("InstructionAndSkillMetadata", func(t *testing.T) {
		t.Parallel()

		resources := []database.ChatContextResource{
			instructionResource(t, "/home/coder/AGENTS.md", "be helpful", database.WorkspaceAgentContextResourceStatusOk),
			skillResource(t, "/home/coder/.coder/skills/deploy", "deploy", "Deploy the app", database.WorkspaceAgentContextResourceStatusOk),
		}
		// instructionResource/skillResource leave SizeBytes zero; set one to
		// confirm it is carried through.
		resources[0].SizeBytes = 10

		out := pinnedContextResources(resources)
		require.Len(t, out, 2)

		require.Equal(t, codersdk.ChatContextResource{
			Source:    "/home/coder/AGENTS.md",
			Kind:      codersdk.ChatContextResourceKindInstructionFile,
			SizeBytes: 10,
			Status:    codersdk.ChatContextResourceStatusOK,
		}, out[0])

		require.Equal(t, codersdk.ChatContextResource{
			Source:           "/home/coder/.coder/skills/deploy",
			Kind:             codersdk.ChatContextResourceKindSkill,
			Status:           codersdk.ChatContextResourceStatusOK,
			SkillName:        "deploy",
			SkillDescription: "Deploy the app",
		}, out[1])
	})

	t.Run("SkipsOKButEmpty", func(t *testing.T) {
		t.Parallel()

		resources := []database.ChatContextResource{
			// OK instruction file with empty content.
			instructionResource(t, "/b/AGENTS.md", "", database.WorkspaceAgentContextResourceStatusOk),
			// OK skill with no name.
			skillResource(t, "/c/skills/x", "", "no name", database.WorkspaceAgentContextResourceStatusOk),
		}
		require.Empty(t, pinnedContextResources(resources))
	})

	t.Run("IncludesNonOKWithError", func(t *testing.T) {
		t.Parallel()

		oversize := instructionResource(t, "/a/AGENTS.md", "ignored", database.WorkspaceAgentContextResourceStatusOversize)
		oversize.SizeBytes = 999
		oversize.Error = "file size exceeds cap"
		invalidSkill := skillResource(t, "/c/skills/moo", "", "", database.WorkspaceAgentContextResourceStatusInvalid)
		invalidSkill.Error = `front-matter name "x" does not match directory "moo"`
		resources := []database.ChatContextResource{oversize, invalidSkill}

		out := pinnedContextResources(resources)
		require.Equal(t, []codersdk.ChatContextResource{
			{
				Source:    "/a/AGENTS.md",
				Kind:      codersdk.ChatContextResourceKindInstructionFile,
				SizeBytes: 999,
				Status:    codersdk.ChatContextResourceStatusOversize,
				Error:     "file size exceeds cap",
			},
			{
				Source: "/c/skills/moo",
				Kind:   codersdk.ChatContextResourceKindSkill,
				Status: codersdk.ChatContextResourceStatusInvalid,
				Error:  `front-matter name "x" does not match directory "moo"`,
			},
		}, out)
	})

	t.Run("IncludesMCPConfigAndServer", func(t *testing.T) {
		t.Parallel()

		resources := []database.ChatContextResource{
			{
				Source:    "/home/coder/.mcp.json",
				BodyKind:  database.WorkspaceAgentContextBodyKindMcpConfig,
				Status:    database.WorkspaceAgentContextResourceStatusOk,
				SizeBytes: 670,
			},
			{
				Source:    "github",
				BodyKind:  database.WorkspaceAgentContextBodyKindMcpServer,
				Status:    database.WorkspaceAgentContextResourceStatusOk,
				SizeBytes: 12,
				// Tool names carry the "<server>__" prefix the agent adds.
				Body: mustMarshalContextBody(t, &agentproto.MCPServerBody{
					ServerName: "github",
					Tools: []*agentproto.MCPTool{
						{Name: "github__create", Description: "Create an issue"},
						{Name: "github__search", Description: "Search code"},
					},
				}),
			},
		}
		out := pinnedContextResources(resources)
		require.Equal(t, []codersdk.ChatContextResource{
			{
				Source:    "/home/coder/.mcp.json",
				Kind:      codersdk.ChatContextResourceKindMCPConfig,
				SizeBytes: 670,
				Status:    codersdk.ChatContextResourceStatusOK,
			},
			{
				Source:    "github",
				Kind:      codersdk.ChatContextResourceKindMCPServer,
				SizeBytes: 12,
				Status:    codersdk.ChatContextResourceStatusOK,
				// Tool names are reported with the "github__" prefix stripped.
				McpTools: []codersdk.ChatContextMCPTool{
					{Name: "create", Description: "Create an issue"},
					{Name: "search", Description: "Search code"},
				},
			},
		}, out)
	})
}

func TestContextResources(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsPinnedResources", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return([]database.ChatContextResource{
				instructionResource(t, "/home/coder/AGENTS.md", "be helpful", database.WorkspaceAgentContextResourceStatusOk),
			}, nil)
		server := newPinServer(t, db)

		resources, err := server.ContextResources(context.Background(), database.Chat{ID: chatID})
		require.NoError(t, err)
		require.Len(t, resources, 1)
		require.Equal(t, "/home/coder/AGENTS.md", resources[0].Source)
		require.Equal(t, codersdk.ChatContextResourceKindInstructionFile, resources[0].Kind)
	})

	t.Run("PinnedListError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return(nil, xerrors.New("boom"))
		server := newPinServer(t, db)

		_, err := server.ContextResources(context.Background(), database.Chat{ID: chatID})
		require.Error(t, err)
	})
}
