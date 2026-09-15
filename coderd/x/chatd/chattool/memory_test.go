package chattool_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

type memoryStore struct {
	memories map[string]chattool.Memory
}

func (s *memoryStore) Get(_ context.Context, name string) (chattool.Memory, error) {
	memory, ok := s.memories[name]
	if !ok {
		return chattool.Memory{}, chattool.ErrMemoryNotFound
	}
	return memory, nil
}
func (*memoryStore) List(context.Context) ([]chattool.MemoryIndexEntry, error) { return nil, nil }
func (s *memoryStore) Count(context.Context) (int64, error)                    { return int64(len(s.memories)), nil }
func (s *memoryStore) Insert(_ context.Context, input chattool.MemoryInput) (chattool.Memory, error) {
	if _, ok := s.memories[input.Name]; ok {
		return chattool.Memory{}, chattool.ErrMemoryExists
	}
	return s.Upsert(context.Background(), input)
}

func (s *memoryStore) Upsert(_ context.Context, input chattool.MemoryInput) (chattool.Memory, error) {
	memory := chattool.Memory{Name: input.Name, Description: input.Description, Body: input.Body, UpdatedAt: time.Now()}
	s.memories[input.Name] = memory
	return memory, nil
}

func (s *memoryStore) Delete(_ context.Context, name string) error {
	delete(s.memories, name)
	return nil
}

func TestMemoryValidationAndNormalization(t *testing.T) {
	t.Parallel()
	require.NoError(t, chattool.ValidateMemoryName("release_notes-2026"))
	require.Error(t, chattool.ValidateMemoryName("Release Notes"))
	require.Error(t, chattool.ValidateMemoryName("-starts-with-dash"))
	require.Equal(t, "keep", chattool.NormalizeMemoryText(" <memory><project-memory>keep</project-memory></memory>\u200b "))
}

func TestFormatMemoryIndex(t *testing.T) {
	t.Parallel()
	entries := make([]chattool.MemoryIndexEntry, chattool.MaxMemoryIndexLines+1)
	for i := range entries {
		entries[i] = chattool.MemoryIndexEntry{Name: "memory-" + strings.Repeat("x", 50) + string(rune('a'+i%26)), Description: strings.Repeat("description ", 20)}
	}
	index := chattool.FormatMemoryIndex(chattool.MemoryScope{Kind: chattool.MemoryScopeProject, Label: "platform"}, entries)
	require.Contains(t, index, "<memory>")
	require.Contains(t, index, `project "platform"`)
	require.Contains(t, index, "more memories not shown.")
	require.LessOrEqual(t, len(index), chattool.MaxMemoryIndexBytes)
	empty := chattool.FormatMemoryIndex(chattool.MemoryScope{Kind: chattool.MemoryScopePersonal}, nil)
	require.Contains(t, empty, "Do not save project details")
	require.NotContains(t, empty, "people on this project")
	require.Contains(t, empty, "No memories saved yet.")
	require.Contains(t, empty, "Memory is personal to you")
	require.Contains(t, index, "people on this project")
	require.NotContains(t, index, "Do not save project details")
}

func TestSaveMemoryCapAndUpsert(t *testing.T) {
	t.Parallel()
	t.Run("Cap", func(t *testing.T) {
		t.Parallel()
		store := &memoryStore{memories: make(map[string]chattool.Memory, chattool.MaxMemories)}
		for i := range chattool.MaxMemories {
			store.memories[string(rune(i))] = chattool.Memory{}
		}
		tool := chattool.SaveMemory(store, chattool.MemoryScope{Kind: chattool.MemoryScopePersonal})
		response, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"name":"durable-fact","description":"Durable fact","body":"Body"}`})
		require.NoError(t, err)
		require.True(t, response.IsError)
		require.Contains(t, response.Content, "merge or delete")
	})
	t.Run("Upsert", func(t *testing.T) {
		t.Parallel()
		store := &memoryStore{memories: map[string]chattool.Memory{"durable-fact": {Name: "durable-fact"}}}
		tool := chattool.SaveMemory(store, chattool.MemoryScope{Kind: chattool.MemoryScopePersonal})
		response, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"name":"DURABLE-fact","description":"<memory>Durable</memory>","body":"<memory>Body</memory>"}`})
		require.NoError(t, err)
		require.False(t, response.IsError)
		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
		require.Equal(t, "durable-fact", result["name"])
	})
}

func TestMemoryStoreAdaptersMapNotFoundAndUpsert(t *testing.T) {
	t.Parallel()
	t.Run("Project", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		projectID, organizationID, chatID, ownerID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		store := chattool.NewProjectMemoryStore(db, projectID, organizationID, chatID, ownerID)
		db.EXPECT().GetChatProjectMemoryByName(gomock.Any(), database.GetChatProjectMemoryByNameParams{ProjectID: projectID, Name: "missing"}).Return(database.GetChatProjectMemoryByNameRow{}, sql.ErrNoRows)
		_, err := store.Get(t.Context(), "missing")
		require.ErrorIs(t, err, chattool.ErrMemoryNotFound)
		db.EXPECT().UpsertChatProjectMemoryByName(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg database.UpsertChatProjectMemoryByNameParams) (database.ChatProjectMemory, error) {
			require.Equal(t, projectID, arg.ProjectID)
			require.Equal(t, organizationID, arg.OrganizationID)
			require.Equal(t, chatID, arg.SourceChatID.UUID)
			require.Equal(t, ownerID, arg.CreatedBy)
			return database.ChatProjectMemory{Name: arg.Name}, nil
		})
		_, err = store.Upsert(t.Context(), chattool.MemoryInput{Name: "fact"})
		require.NoError(t, err)
		db.EXPECT().InsertChatProjectMemory(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg database.InsertChatProjectMemoryParams) (database.ChatProjectMemory, error) {
			require.Equal(t, projectID, arg.ProjectID)
			require.Equal(t, organizationID, arg.OrganizationID)
			require.Equal(t, chatID, arg.SourceChatID.UUID)
			require.Equal(t, ownerID, arg.CreatedBy)
			return database.ChatProjectMemory{Name: arg.Name}, nil
		})
		_, err = store.Insert(t.Context(), chattool.MemoryInput{Name: "new-fact"})
		require.NoError(t, err)
	})
	t.Run("Personal", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		userID, organizationID, chatID := uuid.New(), uuid.New(), uuid.New()
		store := chattool.NewPersonalMemoryStore(db, userID, organizationID, chatID)
		db.EXPECT().GetChatUserMemoryByName(gomock.Any(), database.GetChatUserMemoryByNameParams{UserID: userID, OrganizationID: organizationID, Name: "missing"}).Return(database.GetChatUserMemoryByNameRow{}, sql.ErrNoRows)
		_, err := store.Get(t.Context(), "missing")
		require.ErrorIs(t, err, chattool.ErrMemoryNotFound)
		db.EXPECT().UpsertChatUserMemoryByName(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg database.UpsertChatUserMemoryByNameParams) (database.ChatUserMemory, error) {
			require.Equal(t, userID, arg.UserID)
			require.Equal(t, organizationID, arg.OrganizationID)
			require.Equal(t, chatID, arg.SourceChatID.UUID)
			return database.ChatUserMemory{Name: arg.Name}, nil
		})
		_, err = store.Upsert(t.Context(), chattool.MemoryInput{Name: "fact"})
		require.NoError(t, err)
		db.EXPECT().InsertChatUserMemory(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg database.InsertChatUserMemoryParams) (database.ChatUserMemory, error) {
			require.Equal(t, userID, arg.UserID)
			require.Equal(t, organizationID, arg.OrganizationID)
			require.Equal(t, chatID, arg.SourceChatID.UUID)
			return database.ChatUserMemory{Name: arg.Name}, nil
		})
		_, err = store.Insert(t.Context(), chattool.MemoryInput{Name: "new-fact"})
		require.NoError(t, err)
		db.EXPECT().InsertChatUserMemory(gomock.Any(), gomock.Any()).Return(database.ChatUserMemory{}, &pq.Error{Code: "23505"})
		_, err = store.Insert(t.Context(), chattool.MemoryInput{Name: "existing-fact"})
		require.ErrorIs(t, err, chattool.ErrMemoryExists)
	})
}
