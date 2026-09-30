package chattool_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/testutil"
)

// memoryStore is an in-memory MemoryStore with the same all-or-nothing and
// cap semantics as the project store.
type memoryStore struct {
	memories map[string]chattool.Memory
}

func newMemoryStore(n int) *memoryStore {
	store := &memoryStore{memories: make(map[string]chattool.Memory, n)}
	for i := range n {
		name := fmt.Sprintf("seeded-%03d", i)
		store.memories[name] = chattool.Memory{Name: name}
	}
	return store
}

func (s *memoryStore) Get(_ context.Context, name string) (chattool.Memory, error) {
	memory, ok := s.memories[name]
	if !ok {
		return chattool.Memory{}, chattool.ErrMemoryNotFound
	}
	return memory, nil
}

func (*memoryStore) List(context.Context) ([]chattool.MemoryIndexEntry, error) { return nil, nil }

func (s *memoryStore) Insert(ctx context.Context, input chattool.MemoryInput) (int64, error) {
	return s.Consolidate(ctx, nil, []chattool.MemoryInput{input})
}

func (s *memoryStore) Delete(ctx context.Context, name string) error {
	_, err := s.Consolidate(ctx, []string{name}, nil)
	return err
}

func (s *memoryStore) Consolidate(_ context.Context, deleteNames []string, saves []chattool.MemoryInput) (int64, error) {
	next := make(map[string]chattool.Memory, len(s.memories))
	for name, memory := range s.memories {
		next[name] = memory
	}
	for _, name := range deleteNames {
		if _, ok := next[name]; !ok {
			return 0, chattool.ErrMemoryNotFound
		}
		delete(next, name)
	}
	for _, input := range saves {
		if _, ok := next[input.Name]; ok {
			return 0, chattool.ErrMemoryExists
		}
		next[input.Name] = chattool.Memory{Name: input.Name, Description: input.Description, Body: input.Body}
	}
	if len(next) > chattool.MaxMemories {
		return 0, chattool.ErrMemoryLimit
	}
	s.memories = next
	return int64(len(next)), nil
}

func runMemoryTool(t *testing.T, tool fantasy.AgentTool, input string) (fantasy.ToolResponse, map[string]any) {
	t.Helper()
	response, err := tool.Run(context.Background(), fantasy.ToolCall{Input: input})
	require.NoError(t, err)
	var result map[string]any
	if !response.IsError {
		require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
	}
	return response, result
}

func TestNormalizeMemoryInput(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Release Notes", "-starts-with-dash", ""} {
		_, err := chattool.NormalizeMemoryName(name)
		require.Error(t, err, name)
	}
	name, err := chattool.NormalizeMemoryName(" Release_Notes-2026 ")
	require.NoError(t, err)
	require.Equal(t, "release_notes-2026", name)

	input, err := chattool.NormalizeMemoryInput("fact", "first line\n- forged: index entry\u200b", "body\u200b\r\n")
	require.NoError(t, err)
	require.Equal(t, "first line - forged: index entry", input.Description, "descriptions are one index line")
	require.Equal(t, "body", input.Body)

	_, err = chattool.NormalizeMemoryInput("fact", strings.Repeat("d", chattool.MaxMemoryDescriptionChars+1), "body")
	require.ErrorContains(t, err, "description must be at most")
	_, err = chattool.NormalizeMemoryInput("fact", "description", strings.Repeat("b", chattool.MaxMemoryBodyBytes+1))
	require.ErrorContains(t, err, "body must be at most")
	_, err = chattool.NormalizeMemoryInput("fact", "\u200b", "body")
	require.ErrorContains(t, err, "description is required")
}

func TestFormatMemoryGuidanceAndIndexForTool(t *testing.T) {
	t.Parallel()
	entries := make([]chattool.MemoryIndexEntry, chattool.MaxMemoryIndexLines+1)
	for i := range entries {
		entries[i] = chattool.MemoryIndexEntry{Name: "memory-" + strings.Repeat("x", 50) + string(rune('a'+i%26)), Description: strings.Repeat("description ", 20)}
	}
	guidance := chattool.FormatMemoryGuidance("platform")
	require.Contains(t, guidance, "<memory>")
	require.Contains(t, guidance, `project "platform"`)
	require.Contains(t, guidance, chattool.ConsolidateMemoryToolName)
	require.NotContains(t, guidance, "memory-")
	index := chattool.FormatMemoryIndexForTool(entries)
	require.Contains(t, index, "Available memories:")
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
	tool := chattool.ReadMemory(newMemoryStore(0), []chattool.MemoryIndexEntry{{Name: "release", Description: "Release process"}})
	require.Contains(t, tool.Info().Description, "Read a memory by name.")
	require.Contains(t, tool.Info().Description, "- release: Release process")
}

func TestSaveMemory(t *testing.T) {
	t.Parallel()
	const input = `{"name":"durable-fact","description":"Durable fact","body":"Body"}`
	t.Run("Saves", func(t *testing.T) {
		t.Parallel()
		store := newMemoryStore(0)
		response, result := runMemoryTool(t, chattool.SaveMemory(store, "platform"), `{"name":"DURABLE-fact","description":"Durable","body":"Body"}`)
		require.False(t, response.IsError, response.Content)
		require.Equal(t, "durable-fact", result["saved"])
		require.EqualValues(t, 1, result["count"])
		require.NotContains(t, result, "warning")
		require.Contains(t, store.memories, "durable-fact")
	})
	t.Run("ExistingNameIsNotOverwritten", func(t *testing.T) {
		t.Parallel()
		store := newMemoryStore(0)
		store.memories["durable-fact"] = chattool.Memory{Name: "durable-fact", Body: "original"}
		response, _ := runMemoryTool(t, chattool.SaveMemory(store, "platform"), input)
		require.True(t, response.IsError)
		require.Contains(t, response.Content, "already exists")
		require.Contains(t, response.Content, chattool.ConsolidateMemoryToolName)
		require.Equal(t, "original", store.memories["durable-fact"].Body)
	})
	t.Run("NearCapAsksToConsolidate", func(t *testing.T) {
		t.Parallel()
		store := newMemoryStore(chattool.MemoryConsolidateThreshold - 1)
		response, result := runMemoryTool(t, chattool.SaveMemory(store, "platform"), input)
		require.False(t, response.IsError, response.Content)
		require.EqualValues(t, chattool.MemoryConsolidateThreshold, result["count"])
		require.Equal(t, fmt.Sprintf(
			"memory is %d/%d, approaching the limit. Consolidate it to under %d memories now with %s: merge overlapping memories into one, and drop stale, wrong, or superseded ones.",
			chattool.MemoryConsolidateThreshold, chattool.MaxMemories, chattool.MemoryConsolidateTarget, chattool.ConsolidateMemoryToolName,
		), result["warning"])
	})
	t.Run("FullPointsAtConsolidate", func(t *testing.T) {
		t.Parallel()
		store := newMemoryStore(chattool.MaxMemories)
		response, _ := runMemoryTool(t, chattool.SaveMemory(store, "platform"), input)
		require.True(t, response.IsError)
		require.Contains(t, response.Content, "memory is full (200/200)")
		require.Contains(t, response.Content, chattool.ConsolidateMemoryToolName)
		require.Len(t, store.memories, chattool.MaxMemories)
	})
}

func TestConsolidateMemory(t *testing.T) {
	t.Parallel()
	t.Run("MergesAndMakesRoom", func(t *testing.T) {
		t.Parallel()
		store := newMemoryStore(chattool.MaxMemories)
		response, result := runMemoryTool(t, chattool.ConsolidateMemory(store, "platform"), `{
			"delete": ["seeded-000", "seeded-001", "seeded-002"],
			"save": [
				{"name": "merged", "description": "Merged fact", "body": "Merged body"},
				{"name": "new-fact", "description": "New fact", "body": "New body"}
			]
		}`)
		require.False(t, response.IsError, response.Content)
		require.EqualValues(t, chattool.MaxMemories-1, result["count"])
		require.Contains(t, result, "warning", "still over the threshold")
		require.NotContains(t, store.memories, "seeded-000")
		require.Contains(t, store.memories, "merged")
		require.Contains(t, store.memories, "new-fact")
	})
	t.Run("ReplacesByName", func(t *testing.T) {
		t.Parallel()
		store := newMemoryStore(1)
		response, result := runMemoryTool(t, chattool.ConsolidateMemory(store, "platform"), `{
			"delete": ["seeded-000"],
			"save": [{"name": "seeded-000", "description": "Corrected", "body": "Corrected body"}]
		}`)
		require.False(t, response.IsError, response.Content)
		require.EqualValues(t, 1, result["count"])
		require.NotContains(t, result, "warning")
		require.Equal(t, "Corrected body", store.memories["seeded-000"].Body)
	})
	t.Run("InvalidInputAppliesNothing", func(t *testing.T) {
		t.Parallel()
		for _, input := range []string{
			`{}`,
			`{"delete": ["Not Valid"]}`,
			`{"delete": ["seeded-000"], "save": [{"name": "ok", "description": "", "body": "body"}]}`,
		} {
			store := newMemoryStore(1)
			response, _ := runMemoryTool(t, chattool.ConsolidateMemory(store, "platform"), input)
			require.True(t, response.IsError, input)
			require.Contains(t, store.memories, "seeded-000", input)
		}
	})
}

// TestProjectMemoryStoreConsolidateIsAtomic runs against Postgres because the
// guarantee under test is the transaction rollback.
func TestProjectMemoryStoreConsolidateIsAtomic(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: user.ID})
	store := chattool.NewProjectMemoryStore(db, project.ID, org.ID, user.ID)
	names := func() []string {
		entries, err := store.List(ctx)
		require.NoError(t, err)
		out := make([]string, len(entries))
		for i, entry := range entries {
			out[i] = entry.Name
		}
		return out
	}
	input := func(name string) chattool.MemoryInput {
		return chattool.MemoryInput{Name: name, Description: name + " description", Body: name + " body"}
	}

	for _, name := range []string{"alpha", "beta"} {
		_, err := store.Insert(ctx, input(name))
		require.NoError(t, err)
	}
	_, err := store.Insert(ctx, input("alpha"))
	require.ErrorIs(t, err, chattool.ErrMemoryExists)

	// A missing delete rolls back the deletes and saves before it.
	_, err = store.Consolidate(ctx, []string{"alpha", "missing"}, []chattool.MemoryInput{input("gamma")})
	require.ErrorIs(t, err, chattool.ErrMemoryNotFound)
	require.Equal(t, []string{"alpha", "beta"}, names())

	// A duplicate save rolls back the deletes before it.
	_, err = store.Consolidate(ctx, []string{"alpha"}, []chattool.MemoryInput{input("gamma"), input("gamma")})
	require.ErrorIs(t, err, chattool.ErrMemoryExists)
	require.Equal(t, []string{"alpha", "beta"}, names())

	// Deletes run first, so a save may reuse a deleted name.
	count, err := store.Consolidate(ctx, []string{"alpha", "beta"}, []chattool.MemoryInput{input("alpha"), input("merged")})
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	require.Equal(t, []string{"alpha", "merged"}, names())

	// A result over the cap rolls back.
	saves := make([]chattool.MemoryInput, chattool.MaxMemories-1)
	for i := range saves {
		saves[i] = input(fmt.Sprintf("fill-%03d", i))
	}
	_, err = store.Consolidate(ctx, nil, saves)
	require.ErrorIs(t, err, chattool.ErrMemoryLimit)
	require.Equal(t, []string{"alpha", "merged"}, names())
	count, err = store.Consolidate(ctx, []string{"merged"}, saves)
	require.NoError(t, err)
	require.EqualValues(t, chattool.MaxMemories, count)

	require.NoError(t, store.Delete(ctx, "alpha"))
	require.ErrorIs(t, store.Delete(ctx, "alpha"), chattool.ErrMemoryNotFound)
}
