package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func mustMarshalContextBody(t *testing.T, msg proto.Message) json.RawMessage {
	t.Helper()
	raw, err := protojson.Marshal(msg)
	require.NoError(t, err)
	return raw
}

func instructionResource(t *testing.T, source, content string, status database.WorkspaceAgentContextResourceStatus) database.ChatContextResource {
	t.Helper()
	return database.ChatContextResource{
		Source:   source,
		BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile,
		Body:     mustMarshalContextBody(t, &agentproto.InstructionFileBody{Content: []byte(content)}),
		Status:   status,
	}
}

func skillResource(t *testing.T, source, name, description string, status database.WorkspaceAgentContextResourceStatus) database.ChatContextResource {
	t.Helper()
	return database.ChatContextResource{
		Source:   source,
		BodyKind: database.WorkspaceAgentContextBodyKindSkill,
		Body: mustMarshalContextBody(t, &agentproto.SkillMetaBody{
			Meta:        []byte("# " + name),
			Name:        name,
			Description: description,
		}),
		Status: status,
	}
}

func mcpServerResource(t *testing.T, source string, body *agentproto.MCPServerBody, status database.WorkspaceAgentContextResourceStatus) database.ChatContextResource {
	t.Helper()
	return database.ChatContextResource{
		Source:   source,
		BodyKind: database.WorkspaceAgentContextBodyKindMcpServer,
		Body:     mustMarshalContextBody(t, body),
		Status:   status,
	}
}

func mustStruct(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	require.NoError(t, err)
	return s
}

func TestContextResourcesToPrompt(t *testing.T) {
	t.Parallel()

	t.Run("InstructionFilesBuildWorkspaceContext", func(t *testing.T) {
		t.Parallel()

		resources := []database.ChatContextResource{
			instructionResource(t, "/home/coder/AGENTS.md", "be helpful", database.WorkspaceAgentContextResourceStatusOk),
		}
		instruction, skills, _ := contextResourcesToPrompt(resources, "linux", "/home/coder")

		require.Empty(t, skills)
		require.Contains(t, instruction, "<workspace-context>")
		require.Contains(t, instruction, "Operating System: linux")
		require.Contains(t, instruction, "Working Directory: /home/coder")
		require.Contains(t, instruction, "Source: /home/coder/AGENTS.md")
		require.Contains(t, instruction, "be helpful")
		require.Contains(t, instruction, "</workspace-context>")
	})

	t.Run("SkillsBuildMeta", func(t *testing.T) {
		t.Parallel()

		resources := []database.ChatContextResource{
			skillResource(t, "/home/coder/.coder/skills/deploy", "deploy", "Deploy the app", database.WorkspaceAgentContextResourceStatusOk),
		}
		instruction, skills, _ := contextResourcesToPrompt(resources, "linux", "/home/coder")

		// Skill-only pins emit no instruction header.
		require.Empty(t, instruction)
		require.Len(t, skills, 1)
		require.Equal(t, "deploy", skills[0].Name)
		require.Equal(t, "Deploy the app", skills[0].Description)
		require.Equal(t, "/home/coder/.coder/skills/deploy", skills[0].Dir)
		// MetaFile is left empty so chattool defaults to SKILL.md.
		require.Empty(t, skills[0].MetaFile)
		// Meta carries the pushed SKILL.md so read_skill serves the body
		// from the pin without dialing the workspace.
		require.Equal(t, []byte("# deploy"), skills[0].Meta)
	})

	t.Run("SkipsNonOKStatus", func(t *testing.T) {
		t.Parallel()

		resources := []database.ChatContextResource{
			instructionResource(t, "/home/coder/AGENTS.md", "be helpful", database.WorkspaceAgentContextResourceStatusInvalid),
			skillResource(t, "/home/coder/.coder/skills/deploy", "deploy", "Deploy the app", database.WorkspaceAgentContextResourceStatusOversize),
		}
		instruction, skills, _ := contextResourcesToPrompt(resources, "linux", "/home/coder")

		require.Empty(t, instruction)
		require.Empty(t, skills)
	})

	t.Run("SkipsUnknownBodyKinds", func(t *testing.T) {
		t.Parallel()

		resources := []database.ChatContextResource{
			{
				Source:   ".mcp.json",
				BodyKind: database.WorkspaceAgentContextBodyKindMcpConfig,
				Body:     mustMarshalContextBody(t, &agentproto.MCPConfigBody{}),
				Status:   database.WorkspaceAgentContextResourceStatusOk,
			},
			{
				Source:   "playwright",
				BodyKind: database.WorkspaceAgentContextBodyKindMcpServer,
				Body:     mustMarshalContextBody(t, &agentproto.MCPServerBody{ServerName: "playwright"}),
				Status:   database.WorkspaceAgentContextResourceStatusOk,
			},
		}
		instruction, skills, _ := contextResourcesToPrompt(resources, "linux", "/home/coder")

		require.Empty(t, instruction)
		require.Empty(t, skills)
	})

	t.Run("SkipsMalformedBody", func(t *testing.T) {
		t.Parallel()

		resources := []database.ChatContextResource{
			{
				Source:   "/home/coder/AGENTS.md",
				BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile,
				Body:     json.RawMessage(`{not valid json`),
				Status:   database.WorkspaceAgentContextResourceStatusOk,
			},
			instructionResource(t, "/home/coder/CLAUDE.md", "good content", database.WorkspaceAgentContextResourceStatusOk),
		}
		instruction, skills, malformed := contextResourcesToPrompt(resources, "linux", "/home/coder")

		require.Empty(t, skills)
		require.Equal(t, 1, malformed)
		require.NotContains(t, instruction, "/home/coder/AGENTS.md")
		require.Contains(t, instruction, "Source: /home/coder/CLAUDE.md")
		require.Contains(t, instruction, "good content")
	})

	t.Run("SkipsMalformedSkillBody", func(t *testing.T) {
		t.Parallel()

		resources := []database.ChatContextResource{
			{
				Source:   "/home/coder/.coder/skills/broken",
				BodyKind: database.WorkspaceAgentContextBodyKindSkill,
				Body:     json.RawMessage(`{not valid json`),
				Status:   database.WorkspaceAgentContextResourceStatusOk,
			},
			skillResource(t, "/home/coder/.coder/skills/deploy", "deploy", "Deploy the app", database.WorkspaceAgentContextResourceStatusOk),
		}
		instruction, skills, malformed := contextResourcesToPrompt(resources, "linux", "/home/coder")

		require.Empty(t, instruction)
		require.Equal(t, 1, malformed)
		require.Len(t, skills, 1)
		require.Equal(t, "deploy", skills[0].Name)
	})

	t.Run("SkipsEmptyNameSkill", func(t *testing.T) {
		t.Parallel()

		// Defensive boundary on the agent's own marshaling: an OK skill with an
		// empty name contributes nothing and is not counted as malformed.
		resources := []database.ChatContextResource{
			skillResource(t, "/home/coder/.coder/skills/nameless", "", "no name", database.WorkspaceAgentContextResourceStatusOk),
		}
		instruction, skills, malformed := contextResourcesToPrompt(resources, "linux", "/home/coder")

		require.Empty(t, instruction)
		require.Empty(t, skills)
		require.Zero(t, malformed)
	})

	t.Run("SkipsEmptyInstructionContent", func(t *testing.T) {
		t.Parallel()

		// Whitespace-only content sanitizes to empty, so the instruction file
		// contributes no context-file part, emits no header, and is not counted
		// as malformed.
		resources := []database.ChatContextResource{
			instructionResource(t, "/home/coder/AGENTS.md", "  \n\t  ", database.WorkspaceAgentContextResourceStatusOk),
		}
		instruction, skills, malformed := contextResourcesToPrompt(resources, "linux", "/home/coder")

		require.Empty(t, instruction)
		require.Empty(t, skills)
		require.Zero(t, malformed)
	})

	t.Run("EmptyInput", func(t *testing.T) {
		t.Parallel()

		instruction, skills, _ := contextResourcesToPrompt(nil, "linux", "/home/coder")
		require.Empty(t, instruction)
		require.Empty(t, skills)
	})

	t.Run("OmitsOSDirWhenAgentUnresolved", func(t *testing.T) {
		t.Parallel()

		resources := []database.ChatContextResource{
			instructionResource(t, "/home/coder/AGENTS.md", "be helpful", database.WorkspaceAgentContextResourceStatusOk),
		}
		instruction, _, _ := contextResourcesToPrompt(resources, "", "")

		require.Contains(t, instruction, "<workspace-context>")
		require.Contains(t, instruction, "Source: /home/coder/AGENTS.md")
		require.Contains(t, instruction, "be helpful")
		require.NotContains(t, instruction, "Operating System:")
		require.NotContains(t, instruction, "Working Directory:")
	})
}

func newPinServer(t *testing.T, db database.Store) *Server {
	t.Helper()
	return &Server{
		db:     db,
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
	}
}

func TestPinnedWorkspaceContext(t *testing.T) {
	t.Parallel()

	t.Run("ListError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return(nil, xerrors.New("boom"))
		server := newPinServer(t, db)

		_, _, err := server.pinnedWorkspaceContext(context.Background(), database.Chat{ID: chatID}, database.WorkspaceAgent{})
		require.Error(t, err)
	})

	t.Run("NoRowsYieldsNothing", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return([]database.ChatContextResource{}, nil)
		server := newPinServer(t, db)

		instruction, skills, err := server.pinnedWorkspaceContext(context.Background(), database.Chat{ID: chatID}, database.WorkspaceAgent{})
		require.NoError(t, err)
		require.Empty(t, instruction)
		require.Empty(t, skills)
	})

	t.Run("RowsPresent", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return([]database.ChatContextResource{
				instructionResource(t, "/home/coder/AGENTS.md", "be helpful", database.WorkspaceAgentContextResourceStatusOk),
				skillResource(t, "/home/coder/.coder/skills/deploy", "deploy", "Deploy the app", database.WorkspaceAgentContextResourceStatusOk),
			}, nil)
		server := newPinServer(t, db)

		agent := database.WorkspaceAgent{OperatingSystem: "linux", ExpandedDirectory: "/home/coder"}
		instruction, skills, err := server.pinnedWorkspaceContext(context.Background(), database.Chat{ID: chatID}, agent)
		require.NoError(t, err)
		require.Contains(t, instruction, "Operating System: linux")
		require.Contains(t, instruction, "Source: /home/coder/AGENTS.md")
		require.Contains(t, instruction, "be helpful")
		require.Len(t, skills, 1)
		require.Equal(t, "deploy", skills[0].Name)
	})

	t.Run("RowsPresentUnresolvedAgent", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return([]database.ChatContextResource{
				instructionResource(t, "/home/coder/AGENTS.md", "be helpful", database.WorkspaceAgentContextResourceStatusOk),
			}, nil)
		server := newPinServer(t, db)

		// Zero-value agent: the pin still resolves, just without the
		// OS/directory header.
		instruction, _, err := server.pinnedWorkspaceContext(context.Background(), database.Chat{ID: chatID}, database.WorkspaceAgent{})
		require.NoError(t, err)
		require.Contains(t, instruction, "Source: /home/coder/AGENTS.md")
		require.NotContains(t, instruction, "Operating System:")
	})
}

// TestPinnedWorkspaceContextFromHydratedPin exercises the resolver end to end
// against a real Postgres pin: an agent's pushed context is hydrated into a
// chat's chat_context_resources, then pinnedWorkspaceContext reads that copy.
func TestPinnedWorkspaceContextFromHydratedPin(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tmpl := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
		CreatedBy:       user.ID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     tmpl.ID,
	})
	pj := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		CompletedAt:    sql.NullTime{Valid: true, Time: dbtime.Now()},
	})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       ws.ID,
		TemplateVersionID: tv.ID,
		JobID:             pj.ID,
		Transition:        database.WorkspaceTransitionStart,
	})
	res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		Transition: database.WorkspaceTransitionStart,
		JobID:      pj.ID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID:      res.ID,
		OperatingSystem: "linux",
		Directory:       "/home/coder/ws",
	})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})

	hash := []byte{0x01, 0x02, 0x03}
	seedAgentContext(ctx, t, db, agent.ID, "/home/coder/ws/AGENTS.md", hash,
		database.WorkspaceAgentContextBodyKindInstructionFile,
		mustMarshalContextBody(t, &agentproto.InstructionFileBody{Content: []byte("follow the rules")}))
	seedAgentContext(ctx, t, db, agent.ID, "/home/coder/ws/.coder/skills/deploy", hash,
		database.WorkspaceAgentContextBodyKindSkill,
		mustMarshalContextBody(t, &agentproto.SkillMetaBody{
			Meta:        []byte("# deploy"),
			Name:        "deploy",
			Description: "Deploy the app",
		}))

	chat := dbgen.Chat(t, db, database.Chat{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		LastModelConfigID: model.ID,
		WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: agent.ID, Valid: true},
		Status:            database.ChatStatusWaiting,
	})
	require.NoError(t, db.HydrateAgentChatsContext(ctx, database.HydrateAgentChatsContextParams{
		AgentID:       agent.ID,
		AggregateHash: hash,
	}))
	rows, err := db.ListChatContextResourcesByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2, "the pin holds the agent's instruction file and skill")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	server := &Server{db: db, logger: logger}

	instruction, skills, err := server.pinnedWorkspaceContext(ctx, chat, agent)
	require.NoError(t, err)
	require.Contains(t, instruction, "Operating System: linux")
	require.Contains(t, instruction, "Working Directory: /home/coder/ws")
	require.Contains(t, instruction, "Source: /home/coder/ws/AGENTS.md")
	require.Contains(t, instruction, "follow the rules")
	require.Len(t, skills, 1)
	require.Equal(t, "deploy", skills[0].Name)
	require.Equal(t, "Deploy the app", skills[0].Description)
	require.Equal(t, "/home/coder/ws/.coder/skills/deploy", skills[0].Dir)

	// A chat created after hydration keeps a NULL pinned hash and no pinned
	// rows, so the pin yields no instruction or skills.
	unpinnedChat := dbgen.Chat(t, db, database.Chat{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		LastModelConfigID: model.ID,
		WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: agent.ID, Valid: true},
		Status:            database.ChatStatusWaiting,
	})
	emptyInstruction, emptySkills, err := server.pinnedWorkspaceContext(ctx, unpinnedChat, agent)
	require.NoError(t, err)
	require.Empty(t, emptyInstruction)
	require.Empty(t, emptySkills)
}

// TestResolveTurnWorkspaceContext covers the dispatch that prepareGeneration
// wires up: the pinned copy when the chat has pinned rows, and nothing for a
// non-workspace chat or a chat without pinned rows.
func TestResolveTurnWorkspaceContext(t *testing.T) {
	t.Parallel()

	workspaceChat := func() database.Chat {
		return database.Chat{ID: uuid.New(), WorkspaceID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
	}

	t.Run("NonWorkspaceChatYieldsNothing", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		server := newPinServer(t, db)

		instruction, skills, err := server.resolveTurnWorkspaceContext(context.Background(), database.Chat{ID: uuid.New()}, database.WorkspaceAgent{})
		require.NoError(t, err)
		require.Empty(t, instruction)
		require.Empty(t, skills)
	})

	t.Run("PinnedPathWins", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := workspaceChat()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chat.ID).
			Return([]database.ChatContextResource{
				instructionResource(t, "/home/coder/AGENTS.md", "pinned content", database.WorkspaceAgentContextResourceStatusOk),
				skillResource(t, "/home/coder/.coder/skills/deploy", "deploy", "Deploy the app", database.WorkspaceAgentContextResourceStatusOk),
			}, nil)
		server := newPinServer(t, db)

		instruction, skills, err := server.resolveTurnWorkspaceContext(context.Background(), chat, database.WorkspaceAgent{OperatingSystem: "linux"})
		require.NoError(t, err)
		require.Contains(t, instruction, "pinned content")
		require.Len(t, skills, 1)
		require.Equal(t, "deploy", skills[0].Name)
	})

	t.Run("NoPinYieldsNothing", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := workspaceChat()
		// No pinned rows: the turn carries no context.
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chat.ID).
			Return([]database.ChatContextResource{}, nil)
		server := newPinServer(t, db)

		instruction, skills, err := server.resolveTurnWorkspaceContext(context.Background(), chat, database.WorkspaceAgent{})
		require.NoError(t, err)
		require.Empty(t, instruction)
		require.Empty(t, skills)
	})

	t.Run("PropagatesPinReadError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := workspaceChat()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chat.ID).
			Return(nil, xerrors.New("boom"))
		server := newPinServer(t, db)

		_, _, err := server.resolveTurnWorkspaceContext(context.Background(), chat, database.WorkspaceAgent{})
		require.Error(t, err)
	})
}

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
				Tools: []codersdk.ChatContextTool{
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

func TestWorkspaceMCPToolInfosFromResources(t *testing.T) {
	t.Parallel()

	t.Run("BuildsPrefixedToolsFromMCPServers", func(t *testing.T) {
		t.Parallel()

		schema := mustStruct(t, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{"type": "string"},
				"body":  map[string]any{"type": "string"},
			},
			"required": []any{"title"},
		})
		resources := []database.ChatContextResource{
			// Skipped: a config resource carries no tools.
			{
				Source:   "/home/coder/.mcp.json",
				BodyKind: database.WorkspaceAgentContextBodyKindMcpConfig,
				Body:     mustMarshalContextBody(t, &agentproto.MCPConfigBody{}),
				Status:   database.WorkspaceAgentContextResourceStatusOk,
			},
			mcpServerResource(t, "github", &agentproto.MCPServerBody{
				ServerName: "github",
				Tools: []*agentproto.MCPTool{
					{Name: "create_issue", Description: "Create an issue", InputSchema: schema},
					// Skipped: a tool with no name cannot be addressed.
					{Name: "", Description: "nameless"},
				},
			}, database.WorkspaceAgentContextResourceStatusOk),
			// Skipped: a server that failed to connect is not OK.
			mcpServerResource(t, "broken", &agentproto.MCPServerBody{ServerName: "broken"},
				database.WorkspaceAgentContextResourceStatusUnreadable),
		}

		infos := workspaceMCPToolInfosFromResources(resources)
		require.Len(t, infos, 1)
		require.Equal(t, "github", infos[0].ServerName)
		// Tool names are re-prefixed with the server name so the workspace
		// agent's MCP proxy routes the call to the owning server.
		require.Equal(t, "github__create_issue", infos[0].Name)
		require.Equal(t, "Create an issue", infos[0].Description)
		require.Equal(t, []string{"title"}, infos[0].Required)
		// Schema is the JSON Schema "properties" sub-map, matching the shape the
		// live discovery path produces; "required" travels separately.
		require.Contains(t, infos[0].Schema, "title")
		require.Contains(t, infos[0].Schema, "body")
		require.NotContains(t, infos[0].Schema, "required")
	})

	t.Run("FallsBackToSourceWhenServerNameEmpty", func(t *testing.T) {
		t.Parallel()

		resources := []database.ChatContextResource{
			mcpServerResource(t, "playwright", &agentproto.MCPServerBody{
				Tools: []*agentproto.MCPTool{{Name: "navigate"}},
			}, database.WorkspaceAgentContextResourceStatusOk),
		}
		infos := workspaceMCPToolInfosFromResources(resources)
		require.Len(t, infos, 1)
		require.Equal(t, "playwright", infos[0].ServerName)
		require.Equal(t, "playwright__navigate", infos[0].Name)
	})

	t.Run("NoMCPServersYieldsNil", func(t *testing.T) {
		t.Parallel()

		resources := []database.ChatContextResource{
			instructionResource(t, "/home/coder/AGENTS.md", "be helpful", database.WorkspaceAgentContextResourceStatusOk),
		}
		require.Empty(t, workspaceMCPToolInfosFromResources(resources))
	})
}

func TestPinnedWorkspaceMCPTools(t *testing.T) {
	t.Parallel()

	// getConn is never dialed by these tests: pinnedWorkspaceMCPTools builds
	// tool definitions from the snapshot and only wires the connection for
	// later execution.
	getConn := func(context.Context) (workspacesdk.AgentConn, error) {
		return nil, xerrors.New("not dialed in this test")
	}

	t.Run("NoRowsYieldsNoTools", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return([]database.ChatContextResource{}, nil)
		server := newPinServer(t, db)

		tools, err := server.pinnedWorkspaceMCPTools(context.Background(), database.Chat{ID: chatID}, getConn)
		require.NoError(t, err)
		require.Empty(t, tools)
	})

	t.Run("ListError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return(nil, xerrors.New("boom"))
		server := newPinServer(t, db)

		_, err := server.pinnedWorkspaceMCPTools(context.Background(), database.Chat{ID: chatID}, getConn)
		require.Error(t, err)
	})

	t.Run("BuildsToolsFromMCPServers", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return([]database.ChatContextResource{
				instructionResource(t, "/home/coder/AGENTS.md", "be helpful", database.WorkspaceAgentContextResourceStatusOk),
				mcpServerResource(t, "github", &agentproto.MCPServerBody{
					ServerName: "github",
					Tools: []*agentproto.MCPTool{
						{Name: "create_issue", Description: "Create an issue"},
						{Name: "search", Description: "Search code"},
					},
				}, database.WorkspaceAgentContextResourceStatusOk),
			}, nil)
		server := newPinServer(t, db)

		tools, err := server.pinnedWorkspaceMCPTools(context.Background(), database.Chat{ID: chatID}, getConn)
		require.NoError(t, err)
		require.Len(t, tools, 2)
		require.Equal(t, "github__create_issue", tools[0].Info().Name)
		require.Equal(t, "github__search", tools[1].Info().Name)
	})

	t.Run("PinWithoutMCPServersIsAuthoritative", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		// The chat is pinned (an instruction file is present) but the agent
		// reported no MCP servers: the pin is authoritative, yielding zero
		// tools without a live pull that could resurrect stale tools.
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return([]database.ChatContextResource{
				instructionResource(t, "/home/coder/AGENTS.md", "be helpful", database.WorkspaceAgentContextResourceStatusOk),
			}, nil)
		server := newPinServer(t, db)

		tools, err := server.pinnedWorkspaceMCPTools(context.Background(), database.Chat{ID: chatID}, getConn)
		require.NoError(t, err)
		require.Empty(t, tools)
	})
}
