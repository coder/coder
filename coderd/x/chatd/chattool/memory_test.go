package chattool_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	if _, ok := s.memories[input.Name]; !ok && len(s.memories) >= chattool.MaxMemories {
		return chattool.Memory{}, chattool.ErrMemoryLimit
	}
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

func TestFormatMemoryGuidanceAndIndexForTool(t *testing.T) {
	t.Parallel()
	entries := make([]chattool.MemoryIndexEntry, chattool.MaxMemoryIndexLines+1)
	for i := range entries {
		entries[i] = chattool.MemoryIndexEntry{Name: "memory-" + strings.Repeat("x", 50) + string(rune('a'+i%26)), Description: strings.Repeat("description ", 20)}
	}
	guidance := chattool.FormatMemoryGuidance(chattool.MemoryScope{Label: "platform"})
	require.Contains(t, guidance, "<memory>")
	require.Contains(t, guidance, `project "platform"`)
	require.NotContains(t, guidance, "memory-")
	index := chattool.FormatMemoryIndexForTool(entries)
	require.Contains(t, index, "Available memories (newest first):")
	require.Contains(t, index, "more memories not shown.")
	require.LessOrEqual(t, len(index), chattool.MaxMemoryIndexBytes)

	// A project at the cap with the longest allowed names and descriptions
	// still lists every memory, so nothing becomes unreachable.
	full := make([]chattool.MemoryIndexEntry, chattool.MaxMemories)
	for i := range full {
		full[i] = chattool.MemoryIndexEntry{Name: fmt.Sprintf("%03d-%s", i, strings.Repeat("n", 60)), Description: strings.Repeat("d", chattool.MaxMemoryDescriptionChars)}
	}
	fullIndex := chattool.FormatMemoryIndexForTool(full)
	require.NotContains(t, fullIndex, "not shown")
	require.Contains(t, fullIndex, full[len(full)-1].Name)
	require.Contains(t, guidance, "people on this project")
	require.Equal(t, "No memories saved yet.", chattool.FormatMemoryIndexForTool(nil))
}

func TestReadMemoryDescriptionIncludesIndex(t *testing.T) {
	t.Parallel()
	tool := chattool.ReadMemory(&memoryStore{memories: map[string]chattool.Memory{}}, chattool.MemoryScope{Label: "platform"}, []chattool.MemoryIndexEntry{{Name: "release", Description: "Release process"}})
	require.Contains(t, tool.Info().Description, "Read a memory by name.")
	require.Contains(t, tool.Info().Description, "- release: Release process")
}

func TestSaveMemoryCapAndUpsert(t *testing.T) {
	t.Parallel()
	t.Run("Cap", func(t *testing.T) {
		t.Parallel()
		store := &memoryStore{memories: make(map[string]chattool.Memory, chattool.MaxMemories)}
		for i := range chattool.MaxMemories {
			store.memories[string(rune(i))] = chattool.Memory{}
		}
		tool := chattool.SaveMemory(store, chattool.MemoryScope{Label: "platform"})
		response, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"name":"durable-fact","description":"Durable fact","body":"Body"}`})
		require.NoError(t, err)
		require.True(t, response.IsError)
		require.Contains(t, response.Content, "memory is full (200/200)")
		require.Contains(t, response.Content, "delete_memory")
	})
	t.Run("NearCapWarns", func(t *testing.T) {
		t.Parallel()
		store := &memoryStore{memories: make(map[string]chattool.Memory, chattool.MemoryNearCapWarning)}
		for i := range chattool.MemoryNearCapWarning - 1 {
			store.memories[string(rune(i))] = chattool.Memory{}
		}
		tool := chattool.SaveMemory(store, chattool.MemoryScope{Label: "platform"})
		response, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"name":"durable-fact","description":"Durable fact","body":"Body"}`})
		require.NoError(t, err)
		require.False(t, response.IsError)
		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
		require.Equal(t, "memory is 180/200; merge or delete stale entries soon", result["warning"])
	})
	t.Run("Upsert", func(t *testing.T) {
		t.Parallel()
		store := &memoryStore{memories: map[string]chattool.Memory{"durable-fact": {Name: "durable-fact"}}}
		tool := chattool.SaveMemory(store, chattool.MemoryScope{Label: "platform"})
		response, err := tool.Run(context.Background(), fantasy.ToolCall{Input: `{"name":"DURABLE-fact","description":"<memory>Durable</memory>","body":"<memory>Body</memory>"}`})
		require.NoError(t, err)
		require.False(t, response.IsError)
		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
		require.Equal(t, "durable-fact", result["name"])
		require.NotContains(t, result, "warning")
	})
}

// expectMemoryTx runs the cap-checking transaction against the same mock.
func expectMemoryTx(db *dbmock.MockStore) {
	db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(func(fn func(database.Store) error, _ *database.TxOptions) error {
		return fn(db)
	}).AnyTimes()
	db.EXPECT().AcquireLock(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
}

func TestMemoryStoreAdaptersMapNotFoundAndUpsert(t *testing.T) {
	t.Parallel()
	t.Run("Project", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		projectID, organizationID, chatID, ownerID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		store := chattool.NewProjectMemoryStore(db, projectID, organizationID, chatID, ownerID)
		expectMemoryTx(db)
		db.EXPECT().GetChatProjectMemoryByName(gomock.Any(), database.GetChatProjectMemoryByNameParams{ProjectID: projectID, Name: "missing"}).Return(database.GetChatProjectMemoryByNameRow{}, sql.ErrNoRows)
		_, err := store.Get(t.Context(), "missing")
		require.ErrorIs(t, err, chattool.ErrMemoryNotFound)
		// Upsert of an existing name skips the cap count.
		db.EXPECT().GetChatProjectMemoryByName(gomock.Any(), database.GetChatProjectMemoryByNameParams{ProjectID: projectID, Name: "fact"}).Return(database.GetChatProjectMemoryByNameRow{}, nil)
		db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), projectID).Return(int64(1), nil).AnyTimes()
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
		db.EXPECT().InsertChatProjectMemory(gomock.Any(), gomock.Any()).Return(database.ChatProjectMemory{}, &pq.Error{Code: "23505"})
		_, err = store.Insert(t.Context(), chattool.MemoryInput{Name: "existing-fact"})
		require.ErrorIs(t, err, chattool.ErrMemoryExists)
	})
}
