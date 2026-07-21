package database_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func insertAgentMemory(t *testing.T, db database.Store, userID uuid.UUID, path, content string) database.AgentMemory {
	t.Helper()
	memory, err := db.InsertAgentMemory(context.Background(), database.InsertAgentMemoryParams{
		ID:      uuid.New(),
		UserID:  userID,
		Path:    path,
		Content: content,
	})
	require.NoError(t, err)
	return memory
}

func TestAgentMemoriesSchema(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	ctx := testutil.Context(t, testutil.WaitMedium)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	user := dbgen.User(t, db, database.User{})
	other := dbgen.User(t, db, database.User{})

	first := insertAgentMemory(t, db, user.ID, "/same.md", "first")
	second := insertAgentMemory(t, db, other.ID, "/same.md", "second")
	require.Equal(t, first.Path, second.Path)

	_, err := db.InsertAgentMemory(ctx, database.InsertAgentMemoryParams{
		ID: uuid.New(), UserID: user.ID, Path: first.Path, Content: "duplicate",
	})
	require.True(t, database.IsUniqueViolation(err))

	_, err = db.InsertAgentMemory(ctx, database.InsertAgentMemoryParams{
		ID: uuid.New(), UserID: user.ID, Path: strings.Repeat("a", 1025), Content: "content",
	})
	require.True(t, database.IsCheckViolation(err, database.CheckAgentMemoriesPathSize))

	_, err = db.InsertAgentMemory(ctx, database.InsertAgentMemoryParams{
		ID: uuid.New(), UserID: user.ID, Path: "/large.md", Content: strings.Repeat("x", 65537),
	})
	require.True(t, database.IsCheckViolation(err, database.CheckAgentMemoriesContentSize))

	var nullable string
	err = sqlDB.QueryRowContext(ctx, `
		SELECT is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'agent_memories'
		  AND column_name = 'user_id'
	`).Scan(&nullable)
	require.NoError(t, err)
	require.Equal(t, "NO", nullable)

	var apiKeyScope string
	err = sqlDB.QueryRowContext(ctx, `SELECT 'agent_memory:*'::api_key_scope::text`).Scan(&apiKeyScope)
	require.NoError(t, err)
	require.Equal(t, "agent_memory:*", apiKeyScope)

	_, err = sqlDB.ExecContext(ctx, `DELETE FROM user_status_changes WHERE user_id = $1`, other.ID)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, other.ID)
	require.NoError(t, err)
	_, err = db.GetAgentMemoryByUserIDAndPath(ctx, database.GetAgentMemoryByUserIDAndPathParams{
		UserID: other.ID,
		Path:   "/same.md",
	})
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestAgentMemoriesUserIsolationAndSoftDelete(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	other := dbgen.User(t, db, database.User{})
	insertAgentMemory(t, db, user.ID, "/private.md", "private")
	insertAgentMemory(t, db, other.ID, "/private.md", "other")

	memory, err := db.GetAgentMemoryByUserIDAndPath(ctx, database.GetAgentMemoryByUserIDAndPathParams{
		UserID: user.ID,
		Path:   "/private.md",
	})
	require.NoError(t, err)
	require.Equal(t, "private", memory.Content)

	err = db.UpdateUserDeletedByID(ctx, user.ID)
	require.NoError(t, err)
	_, err = db.GetAgentMemoryByUserIDAndPath(ctx, database.GetAgentMemoryByUserIDAndPathParams{
		UserID: user.ID,
		Path:   "/private.md",
	})
	require.ErrorIs(t, err, sql.ErrNoRows)

	_, err = db.InsertAgentMemory(ctx, database.InsertAgentMemoryParams{
		ID: uuid.New(), UserID: user.ID, Path: "/after-delete.md", Content: "denied",
	})
	require.True(t, database.IsCheckViolation(err))

	otherMemory, err := db.GetAgentMemoryByUserIDAndPath(ctx, database.GetAgentMemoryByUserIDAndPathParams{
		UserID: other.ID,
		Path:   "/private.md",
	})
	require.NoError(t, err)
	require.Equal(t, "other", otherMemory.Content)
}

func TestSearchAgentMemories(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	other := dbgen.User(t, db, database.User{})

	insertAgentMemory(t, db, user.ID, "/projects/alpha.md", "PostgreSQL keeps durable memory.")
	insertAgentMemory(t, db, user.ID, "/projects/beta.md", "PostgreSQL keeps searchable memory.")
	insertAgentMemory(t, db, user.ID, "/notes/exact.md", "The exact phrase appears here.")
	insertAgentMemory(t, db, user.ID, "/path-keyword.md", "first line")
	insertAgentMemory(t, db, other.ID, "/projects/private.md", "PostgreSQL belongs to another user.")

	rows, err := db.SearchAgentMemories(ctx, database.SearchAgentMemoriesParams{
		UserID: user.ID, Keywords: `"exact phrase"`, PathRegexes: []string{}, OffsetValue: 0,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "/notes/exact.md", rows[0].Path)
	require.Contains(t, rows[0].Headline, "<memory-hit>exact</memory-hit>")

	rows, err = db.SearchAgentMemories(ctx, database.SearchAgentMemoriesParams{
		UserID: user.ID, Keywords: "postgresql -durable", PathRegexes: []string{`^/projects/[^/]+\.md$`}, OffsetValue: 0,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "/projects/beta.md", rows[0].Path)

	rows, err = db.SearchAgentMemories(ctx, database.SearchAgentMemoriesParams{
		UserID: user.ID, Keywords: "keyword", PathRegexes: []string{}, OffsetValue: 0,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "/path-keyword.md", rows[0].Path)
	require.NotContains(t, rows[0].Headline, "<memory-hit>")

	for i := 0; i < 30; i++ {
		insertAgentMemory(t, db, user.ID, fmt.Sprintf("/page/%02d.md", i), "pageable")
	}
	rows, err = db.SearchAgentMemories(ctx, database.SearchAgentMemoriesParams{
		UserID: user.ID, Keywords: "pageable", PathRegexes: []string{}, OffsetValue: 0,
	})
	require.NoError(t, err)
	require.Len(t, rows, 26)
	require.Equal(t, "/page/00.md", rows[0].Path)
	require.Equal(t, "/page/25.md", rows[25].Path)

	rows, err = db.SearchAgentMemories(ctx, database.SearchAgentMemoriesParams{
		UserID: user.ID, Keywords: "pageable", PathRegexes: []string{}, OffsetValue: 25,
	})
	require.NoError(t, err)
	require.Len(t, rows, 5)
	require.Equal(t, "/page/25.md", rows[0].Path)
	require.Equal(t, "/page/29.md", rows[4].Path)
}

func TestListAgentMemoriesPagination(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	for i := 0; i < 30; i++ {
		insertAgentMemory(t, db, user.ID, fmt.Sprintf("/projects/a/%02d.md", i), "content")
	}
	insertAgentMemory(t, db, user.ID, "/projects/b/other.md", "content")
	insertAgentMemory(t, db, user.ID, "/outside.md", "content")

	rows, err := db.ListAgentMemories(ctx, database.ListAgentMemoriesParams{
		UserID: user.ID, DirectoryRegex: `^/projects/a(/.*)?$`, OffsetValue: 0,
	})
	require.NoError(t, err)
	require.Len(t, rows, 26)
	require.Equal(t, "/projects/a/00.md", rows[0].Path)
	require.Equal(t, "/projects/a/25.md", rows[25].Path)

	rows, err = db.ListAgentMemories(ctx, database.ListAgentMemoriesParams{
		UserID: user.ID, DirectoryRegex: `^/projects/a(/.*)?$`, OffsetValue: 25,
	})
	require.NoError(t, err)
	require.Len(t, rows, 5)
	require.Equal(t, "/projects/a/25.md", rows[0].Path)
	require.Equal(t, "/projects/a/29.md", rows[4].Path)
}
