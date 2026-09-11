package chattool_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

func TestProjectMemoryValidationAndNormalization(t *testing.T) {
	t.Parallel()

	require.NoError(t, chattool.ValidateProjectMemoryName("release_notes-2026"))
	require.Error(t, chattool.ValidateProjectMemoryName("Release Notes"))
	require.Error(t, chattool.ValidateProjectMemoryName("-starts-with-dash"))

	text := chattool.NormalizeProjectMemoryText(" <project-memory>keep</project-memory>\u200b ")
	require.Equal(t, "keep", text)
}

func TestFormatProjectMemoryIndex(t *testing.T) {
	t.Parallel()

	entries := make([]chattool.ProjectMemoryIndexEntry, chattool.MaxProjectMemoryIndexLines+1)
	for i := range entries {
		entries[i] = chattool.ProjectMemoryIndexEntry{
			Name:        "memory-" + strings.Repeat("x", 50) + string(rune('a'+i%26)),
			Type:        database.ChatProjectMemoryTypeProject,
			Description: strings.Repeat("description ", 20),
		}
	}
	index := chattool.FormatProjectMemoryIndex(entries)
	require.Contains(t, index, "<project-memory>")
	require.Contains(t, index, "more memories not shown.")
	require.LessOrEqual(t, len(index), chattool.MaxProjectMemoryIndexBytes)

	// A project with no memories still needs the guidance so the model
	// knows when to save the first one.
	empty := chattool.FormatProjectMemoryIndex(nil)
	require.Contains(t, empty, chattool.ProjectMemoryGuidance)
	require.Contains(t, empty, "No memories saved yet.")
}

func TestSaveProjectMemoryCapAndUpsert(t *testing.T) {
	t.Parallel()

	t.Run("Cap", func(t *testing.T) {
		t.Parallel()
		controller := gomock.NewController(t)
		db := dbmock.NewMockStore(controller)
		projectID := uuid.New()
		db.EXPECT().GetChatProjectMemoryByName(gomock.Any(), gomock.Any()).Return(database.GetChatProjectMemoryByNameRow{}, sql.ErrNoRows)
		db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), projectID).Return(int64(chattool.MaxProjectMemories), nil)

		tool := chattool.SaveProjectMemory(chattool.ProjectMemoryOptions{Store: db, ProjectID: projectID, OrganizationID: uuid.New(), ChatID: uuid.New(), OwnerID: uuid.New()})
		response, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"name":"durable-fact","type":"project","description":"Durable fact","body":"Body"}`})
		require.NoError(t, err)
		require.True(t, response.IsError)
		require.Contains(t, response.Content, "merge or delete")
	})

	t.Run("Upsert", func(t *testing.T) {
		t.Parallel()
		controller := gomock.NewController(t)
		db := dbmock.NewMockStore(controller)
		projectID := uuid.New()
		organizationID := uuid.New()
		chatID := uuid.New()
		ownerID := uuid.New()
		db.EXPECT().GetChatProjectMemoryByName(gomock.Any(), gomock.Any()).Return(database.GetChatProjectMemoryByNameRow{ChatProjectMemory: database.ChatProjectMemory{ID: uuid.New()}}, nil)
		db.EXPECT().UpsertChatProjectMemoryByName(gomock.Any(), gomock.AssignableToTypeOf(database.UpsertChatProjectMemoryByNameParams{})).DoAndReturn(func(_ context.Context, arg database.UpsertChatProjectMemoryByNameParams) (database.ChatProjectMemory, error) {
			require.Equal(t, "durable-fact", arg.Name)
			require.Equal(t, database.ChatProjectMemoryTypeProject, arg.Type)
			require.Equal(t, projectID, arg.ProjectID)
			require.Equal(t, organizationID, arg.OrganizationID)
			require.Equal(t, chatID, arg.SourceChatID.UUID)
			require.Equal(t, ownerID, arg.CreatedBy)
			return database.ChatProjectMemory{ID: uuid.New(), Name: arg.Name}, nil
		})

		tool := chattool.SaveProjectMemory(chattool.ProjectMemoryOptions{Store: db, ProjectID: projectID, OrganizationID: organizationID, ChatID: chatID, OwnerID: ownerID})
		response, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"name":"DURABLE-fact","type":"project","description":"<project-memory>Durable</project-memory>","body":"<project-memory>Body</project-memory>"}`})
		require.NoError(t, err)
		require.False(t, response.IsError)
		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
		require.Equal(t, "durable-fact", result["name"])
	})
}
