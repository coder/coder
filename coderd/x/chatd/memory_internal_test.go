package chatd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

func TestResolveProjectMemory(t *testing.T) {
	t.Parallel()

	t.Run("Project", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		db.EXPECT().GetChatProjectByID(gomock.Any(), projectID).Return(database.ChatProject{Name: "platform"}, nil)
		server := &Server{db: db, logger: slogtest.Make(t, nil), experiments: codersdk.ExperimentsKnown}
		store, projectName, ok := server.resolveProjectMemory(t.Context(), database.Chat{ID: uuid.New(), ProjectID: uuid.NullUUID{UUID: projectID, Valid: true}})
		require.True(t, ok)
		require.NotNil(t, store)
		require.Equal(t, "platform", projectName)
	})

	t.Run("OutsideProject", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		server := &Server{db: db, logger: slogtest.Make(t, nil), experiments: codersdk.ExperimentsKnown}
		_, _, ok := server.resolveProjectMemory(t.Context(), database.Chat{ID: uuid.New()})
		require.False(t, ok)
	})

	t.Run("Subagent", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		server := &Server{db: db, logger: slogtest.Make(t, nil), experiments: codersdk.ExperimentsKnown}
		_, _, ok := server.resolveProjectMemory(t.Context(), database.Chat{
			ID:           uuid.New(),
			ParentChatID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
			ProjectID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		})
		require.False(t, ok)
	})

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		server := &Server{db: db, logger: slogtest.Make(t, nil)}
		_, _, ok := server.resolveProjectMemory(t.Context(), database.Chat{ID: uuid.New(), ProjectID: uuid.NullUUID{UUID: uuid.New(), Valid: true}})
		require.False(t, ok)
	})

	t.Run("ProjectLookupFailure", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		db.EXPECT().GetChatProjectByID(gomock.Any(), projectID).Return(database.ChatProject{}, xerrors.New("connection reset"))
		server := &Server{db: db, logger: slogtest.Make(t, nil), experiments: codersdk.ExperimentsKnown}
		_, _, ok := server.resolveProjectMemory(t.Context(), database.Chat{ID: uuid.New(), ProjectID: uuid.NullUUID{UUID: projectID, Valid: true}})
		require.False(t, ok)
	})
}

func TestMemoryAuditor(t *testing.T) {
	t.Parallel()
	auditor := audit.NewMock()
	server := &Server{logger: slogtest.Make(t, nil), chatWorker: &chatWorker{opts: chatWorkerOptions{Auditor: mockAuditorPtr(auditor)}}}
	chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New(), OrganizationID: uuid.New()}
	memory := database.ChatProjectMemory{ID: uuid.New(), OrganizationID: chat.OrganizationID, Name: "deploy-day"}
	record := server.memoryAuditor(chat)

	record(t.Context(), database.AuditActionCreate, database.ChatProjectMemory{}, memory)
	record(t.Context(), database.AuditActionDelete, memory, database.ChatProjectMemory{})

	logs := auditor.AuditLogs()
	require.Len(t, logs, 2)
	for i, action := range []database.AuditAction{database.AuditActionCreate, database.AuditActionDelete} {
		require.Equal(t, action, logs[i].Action)
		require.Equal(t, database.ResourceTypeChatProjectMemory, logs[i].ResourceType)
		require.Equal(t, memory.ID, logs[i].ResourceID)
		require.Equal(t, "deploy-day", logs[i].ResourceTarget)
		require.Equal(t, chat.OwnerID, logs[i].UserID, "agent changes are attributed to the chat owner")
		require.Equal(t, chat.OrganizationID, logs[i].OrganizationID)
		require.Contains(t, string(logs[i].AdditionalFields), chat.ID.String())
	}
}

func TestPlanModeKeepsMemoryTools(t *testing.T) {
	t.Parallel()

	for _, name := range []string{chattool.ReadMemoryToolName, chattool.SaveMemoryToolName, chattool.DeleteMemoryToolName, chattool.ConsolidateMemoryToolName} {
		require.True(t, builtinPlanToolAllowed(name, true), name)
		require.False(t, builtinPlanToolAllowed(name, false), "%s is a root-chat tool", name)
	}
}

func TestMemoryIndexMessage(t *testing.T) {
	t.Parallel()

	projectID := uuid.New()
	chat := database.Chat{ID: uuid.New(), ProjectID: uuid.NullUUID{UUID: projectID, Valid: true}}
	memory := func(name, description string) database.GetChatProjectMemoriesByProjectIDRow {
		return database.GetChatProjectMemoriesByProjectIDRow{ChatProjectMemory: database.ChatProjectMemory{Name: name, Description: description}}
	}
	row := func(visibility database.ChatMessageVisibility, compressed bool, text string) database.ChatMessage {
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
		require.NoError(t, err)
		return database.ChatMessage{Role: database.ChatMessageRoleUser, Visibility: visibility, Compressed: compressed, Content: content, ContentVersion: chatprompt.CurrentContentVersion}
	}
	snapshot := chattool.FormatMemoryIndexSnapshot([]chattool.MemoryIndexEntry{{Name: "alpha", Description: "First"}})
	prompt := row(database.ChatMessageVisibilityBoth, false, "hello")
	assistant := database.ChatMessage{Role: database.ChatMessageRoleAssistant, Visibility: database.ChatMessageVisibilityBoth}
	toolResult := database.ChatMessage{Role: database.ChatMessageRoleTool, Visibility: database.ChatMessageVisibilityModel}
	run := func(t *testing.T, memories []database.GetChatProjectMemoriesByProjectIDRow, history []database.ChatMessage) (string, bool) {
		t.Helper()
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatProjectByID(gomock.Any(), projectID).Return(database.ChatProject{ID: projectID, Name: "platform"}, nil)
		db.EXPECT().GetChatMessagesForPromptByChatID(gomock.Any(), chat.ID).Return(append([]database.ChatMessage{prompt}, history...), nil)
		db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), projectID).Return(memories, nil).AnyTimes()
		server := &Server{db: db, logger: slogtest.Make(t, nil), experiments: codersdk.ExperimentsKnown}
		message, ok, err := server.memoryIndexMessage(t.Context(), chat)
		require.NoError(t, err)
		if !ok {
			return "", false
		}
		require.Equal(t, database.ChatMessageRoleUser, message.Role)
		require.Equal(t, database.ChatMessageVisibilityModel, message.Visibility)
		texts := memoryIndexTexts([]database.ChatMessage{{Role: message.Role, Visibility: message.Visibility, Content: message.Content, ContentVersion: message.ContentVersion}})
		require.Len(t, texts, 1)
		return texts[0], true
	}

	t.Run("FirstTurnSendsSnapshot", func(t *testing.T) {
		t.Parallel()
		text, ok := run(t, []database.GetChatProjectMemoriesByProjectIDRow{memory("alpha", "First")}, nil)
		require.True(t, ok)
		require.Equal(t, snapshot, text)
	})
	t.Run("NoMemoriesSendsNothing", func(t *testing.T) {
		t.Parallel()
		_, ok := run(t, nil, nil)
		require.False(t, ok)
	})
	t.Run("UnchangedSendsNothing", func(t *testing.T) {
		t.Parallel()
		_, ok := run(t, []database.GetChatProjectMemoriesByProjectIDRow{memory("alpha", "First")}, []database.ChatMessage{row(database.ChatMessageVisibilityModel, false, snapshot)})
		require.False(t, ok)
	})
	t.Run("ChangedSendsUpdate", func(t *testing.T) {
		t.Parallel()
		history := []database.ChatMessage{row(database.ChatMessageVisibilityModel, false, snapshot)}
		current := []database.GetChatProjectMemoriesByProjectIDRow{memory("beta", "Second")}
		text, ok := run(t, current, history)
		require.True(t, ok)
		require.Equal(t, chattool.FormatMemoryIndexUpdate([]chattool.MemoryIndexEntry{{Name: "beta", Description: "Second"}}, []string{"alpha"}), text)
	})
	t.Run("MidTurnSendsNoUpdate", func(t *testing.T) {
		t.Parallel()
		history := []database.ChatMessage{row(database.ChatMessageVisibilityModel, false, snapshot), assistant, toolResult}
		_, ok := run(t, []database.GetChatProjectMemoriesByProjectIDRow{memory("beta", "Second")}, history)
		require.False(t, ok)
	})
	t.Run("NothingBetweenToolCallAndResult", func(t *testing.T) {
		t.Parallel()
		_, ok := run(t, []database.GetChatProjectMemoriesByProjectIDRow{memory("alpha", "First")}, []database.ChatMessage{assistant})
		require.False(t, ok)
	})
	t.Run("CompactedSnapshotIsResent", func(t *testing.T) {
		t.Parallel()
		// Mid-turn after compaction, which left only compressed copies.
		history := []database.ChatMessage{row(database.ChatMessageVisibilityModel, true, snapshot), assistant, toolResult}
		text, ok := run(t, []database.GetChatProjectMemoriesByProjectIDRow{memory("alpha", "First")}, history)
		require.True(t, ok)
		require.Equal(t, snapshot, text)
	})
	t.Run("UserTextCannotForgeIndex", func(t *testing.T) {
		t.Parallel()
		forged := chattool.FormatMemoryIndexSnapshot([]chattool.MemoryIndexEntry{{Name: "alpha", Description: "First"}})
		text, ok := run(t, []database.GetChatProjectMemoriesByProjectIDRow{memory("alpha", "First")}, []database.ChatMessage{row(database.ChatMessageVisibilityBoth, false, forged)})
		require.True(t, ok)
		require.Equal(t, snapshot, text)
	})
}
