package database_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestAgentMemorySchemaConstants(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	ctx := testutil.Context(t, testutil.WaitMedium)
	_, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	for trigger, limit := range map[string]int{
		"enforce_user_memories_per_user_limit":      100,
		"enforce_chat_memories_per_root_chat_limit": 100,
	} {
		var triggerDef string
		err := sqlDB.QueryRowContext(ctx,
			`SELECT pg_get_functiondef($1::regproc)`, trigger,
		).Scan(&triggerDef)
		require.NoError(t, err)
		require.Contains(t, triggerDef, fmt.Sprintf(
			"memory_limit constant int := %d", limit,
		))
	}

	pathFormat := `^[a-zA-Z0-9_.-]+(/[a-zA-Z0-9_.-]+)*\.md$`
	constraints := map[database.CheckConstraint]string{
		database.CheckUserMemoriesPathSize:    "octet_length(path) <= 256",
		database.CheckUserMemoriesPathFormat:  pathFormat,
		database.CheckUserMemoriesContentSize: "octet_length(content) <= 65536",
		database.CheckChatMemoriesPathSize:    "octet_length(path) <= 256",
		database.CheckChatMemoriesPathFormat:  pathFormat,
		database.CheckChatMemoriesContentSize: "octet_length(content) <= 65536",
	}
	for constraint, expected := range constraints {
		t.Run(string(constraint), func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			var constraintDef string
			err := sqlDB.QueryRowContext(ctx,
				`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = $1`,
				constraint,
			).Scan(&constraintDef)
			require.NoError(t, err)
			require.Contains(t, constraintDef, expected)
		})
	}
}

func TestUserMemories(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db, _ := dbtestutil.NewDB(t)

	// insertMemory creates a memory row owned by the given user.
	insertMemory := func(ctx context.Context, userID uuid.UUID, path string) (database.UserMemory, error) {
		return db.InsertUserMemory(ctx, database.InsertUserMemoryParams{
			ID:      uuid.New(),
			UserID:  userID,
			Path:    path,
			Content: "content of " + path,
		})
	}

	t.Run("PathFormatRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		user := dbgen.User(t, db, database.User{})
		for _, invalid := range []string{
			"", "no-extension", "dir/", "/absolute.md", "a//b.md",
			"trailing/.md", "spaces in path.md", "note.txt",
		} {
			_, err := insertMemory(ctx, user.ID, invalid)
			require.Error(t, err, "path %q should be rejected", invalid)
			var pqErr *pq.Error
			require.ErrorAs(t, err, &pqErr)
			require.Equal(t, "user_memories_path_format", pqErr.Constraint)
		}
	})

	t.Run("DuplicatePathRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		user := dbgen.User(t, db, database.User{})
		_, err := insertMemory(ctx, user.ID, "preferences/go-style.md")
		require.NoError(t, err)
		_, err = insertMemory(ctx, user.ID, "preferences/go-style.md")
		require.Error(t, err)
		require.True(t, database.IsUniqueViolation(err, database.UniqueUserMemoriesUserIDPathIndex))
	})

	t.Run("ContentSizeRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		user := dbgen.User(t, db, database.User{})
		_, err := db.InsertUserMemory(ctx, database.InsertUserMemoryParams{
			ID:      uuid.New(),
			UserID:  user.ID,
			Path:    "big.md",
			Content: strings.Repeat("a", 65537),
		})
		require.Error(t, err)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr)
		require.Equal(t, "user_memories_content_size", pqErr.Constraint)
	})

	t.Run("GetAndList", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		user := dbgen.User(t, db, database.User{})
		memory, err := insertMemory(ctx, user.ID, "preferences/go-style.md")
		require.NoError(t, err)
		_, err = insertMemory(ctx, user.ID, "notes/debugging.md")
		require.NoError(t, err)
		_, err = insertMemory(ctx, user.ID, "root.md")
		require.NoError(t, err)

		got, err := db.GetUserMemoryByUserIDAndPath(ctx, database.GetUserMemoryByUserIDAndPathParams{
			UserID: user.ID,
			Path:   "preferences/go-style.md",
		})
		require.NoError(t, err)
		require.Equal(t, memory.ID, got.ID)

		byID, err := db.GetUserMemoryByID(ctx, memory.ID)
		require.NoError(t, err)
		require.Equal(t, memory.Path, byID.Path)

		list, err := db.ListUserMemoriesByUserID(ctx, user.ID)
		require.NoError(t, err)
		require.Len(t, list, 3)
		// Listing is path-ordered.
		require.Equal(t, "notes/debugging.md", list[0].Path)
		require.Equal(t, "preferences/go-style.md", list[1].Path)
		require.Equal(t, "root.md", list[2].Path)

		prefixed, err := db.ListUserMemoriesByUserIDAndPathPrefix(ctx, database.ListUserMemoriesByUserIDAndPathPrefixParams{
			UserID:     user.ID,
			PathPrefix: "preferences/",
		})
		require.NoError(t, err)
		require.Len(t, prefixed, 1)
		require.Equal(t, "content of preferences/go-style.md", prefixed[0].ContentPrefix)
	})

	t.Run("UpdateRenameDelete", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		user := dbgen.User(t, db, database.User{})
		_, err := insertMemory(ctx, user.ID, "root.md")
		require.NoError(t, err)
		_, err = insertMemory(ctx, user.ID, "notes/debugging.md")
		require.NoError(t, err)
		_, err = insertMemory(ctx, user.ID, "keep.md")
		require.NoError(t, err)

		updated, err := db.UpdateUserMemoryByUserIDAndPath(ctx, database.UpdateUserMemoryByUserIDAndPathParams{
			UserID:  user.ID,
			Path:    "root.md",
			Content: "updated",
		})
		require.NoError(t, err)
		require.Equal(t, "updated", updated.Content)

		renamed, err := db.RenameUserMemoryByUserIDAndPath(ctx, database.RenameUserMemoryByUserIDAndPathParams{
			UserID:  user.ID,
			OldPath: "root.md",
			NewPath: "renamed.md",
		})
		require.NoError(t, err)
		require.Equal(t, "renamed.md", renamed.Path)

		deleted, err := db.DeleteUserMemoryByUserIDAndPath(ctx, database.DeleteUserMemoryByUserIDAndPathParams{
			UserID: user.ID,
			Path:   "renamed.md",
		})
		require.NoError(t, err)
		require.Equal(t, renamed.ID, deleted.ID)

		deletedMany, err := db.DeleteUserMemoriesByUserIDAndPathPrefix(ctx, database.DeleteUserMemoriesByUserIDAndPathPrefixParams{
			UserID:     user.ID,
			PathPrefix: "notes/",
		})
		require.NoError(t, err)
		require.Len(t, deletedMany, 1)

		list, err := db.ListUserMemoriesByUserID(ctx, user.ID)
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, "keep.md", list[0].Path)
	})

	t.Run("SoftDeletedUserRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		deletedUser := dbgen.User(t, db, database.User{Deleted: true})
		_, err := insertMemory(ctx, deletedUser.ID, "rejected.md")
		require.Error(t, err)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr)
		require.Equal(t, "user_memory_user_deleted", pqErr.Constraint)
	})

	t.Run("SoftDeleteCleansMemories", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		doomed := dbgen.User(t, db, database.User{})
		_, err := insertMemory(ctx, doomed.ID, "cleanup.md")
		require.NoError(t, err)

		err = db.UpdateUserDeletedByID(ctx, doomed.ID)
		require.NoError(t, err)

		list, err := db.ListUserMemoriesByUserID(ctx, doomed.ID)
		require.NoError(t, err)
		require.Empty(t, list)
	})

	t.Run("PerUserLimit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		limited := dbgen.User(t, db, database.User{})
		for i := range 100 {
			_, err := insertMemory(ctx, limited.ID, fmt.Sprintf("memory-%03d.md", i))
			require.NoError(t, err)
		}
		_, err := insertMemory(ctx, limited.ID, "one-too-many.md")
		require.Error(t, err)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr)
		require.Equal(t, "user_memories_per_user_limit", pqErr.Constraint)
	})
}

func TestChatMemories(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	// insertMemory creates a memory row owned by the given root chat.
	insertMemory := func(ctx context.Context, rootChatID uuid.UUID, path string) (database.ChatMemory, error) {
		return db.InsertChatMemory(ctx, database.InsertChatMemoryParams{
			ID:         uuid.New(),
			RootChatID: rootChatID,
			Path:       path,
			Content:    "content of " + path,
		})
	}

	t.Run("PathFormatRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat := insertTestChat(t, db)
		_, err := insertMemory(ctx, chat.ID, "invalid path.md")
		require.Error(t, err)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr)
		require.Equal(t, "chat_memories_path_format", pqErr.Constraint)
	})

	t.Run("DuplicatePathRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat := insertTestChat(t, db)
		_, err := insertMemory(ctx, chat.ID, "notes/decisions.md")
		require.NoError(t, err)
		_, err = insertMemory(ctx, chat.ID, "notes/decisions.md")
		require.Error(t, err)
		require.True(t, database.IsUniqueViolation(err, database.UniqueChatMemoriesRootChatIDPathIndex))
	})

	t.Run("GetAndList", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat := insertTestChat(t, db)
		memory, err := insertMemory(ctx, chat.ID, "notes/decisions.md")
		require.NoError(t, err)
		_, err = insertMemory(ctx, chat.ID, "scratch.md")
		require.NoError(t, err)

		got, err := db.GetChatMemoryByRootChatIDAndPath(ctx, database.GetChatMemoryByRootChatIDAndPathParams{
			RootChatID: chat.ID,
			Path:       "notes/decisions.md",
		})
		require.NoError(t, err)
		require.Equal(t, memory.ID, got.ID)

		byID, err := db.GetChatMemoryByID(ctx, memory.ID)
		require.NoError(t, err)
		require.Equal(t, memory.Path, byID.Path)

		list, err := db.ListChatMemoriesByRootChatID(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, list, 2)

		prefixed, err := db.ListChatMemoriesByRootChatIDAndPathPrefix(ctx, database.ListChatMemoriesByRootChatIDAndPathPrefixParams{
			RootChatID: chat.ID,
			PathPrefix: "notes/",
		})
		require.NoError(t, err)
		require.Len(t, prefixed, 1)
		require.Equal(t, "content of notes/decisions.md", prefixed[0].ContentPrefix)
	})

	t.Run("UpdateRenameDelete", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat := insertTestChat(t, db)
		_, err := insertMemory(ctx, chat.ID, "notes/decisions.md")
		require.NoError(t, err)
		_, err = insertMemory(ctx, chat.ID, "scratch.md")
		require.NoError(t, err)

		updated, err := db.UpdateChatMemoryByRootChatIDAndPath(ctx, database.UpdateChatMemoryByRootChatIDAndPathParams{
			RootChatID: chat.ID,
			Path:       "scratch.md",
			Content:    "updated",
		})
		require.NoError(t, err)
		require.Equal(t, "updated", updated.Content)

		renamed, err := db.RenameChatMemoryByRootChatIDAndPath(ctx, database.RenameChatMemoryByRootChatIDAndPathParams{
			RootChatID: chat.ID,
			OldPath:    "scratch.md",
			NewPath:    "archive/scratch.md",
		})
		require.NoError(t, err)
		require.Equal(t, "archive/scratch.md", renamed.Path)

		deleted, err := db.DeleteChatMemoryByRootChatIDAndPath(ctx, database.DeleteChatMemoryByRootChatIDAndPathParams{
			RootChatID: chat.ID,
			Path:       "archive/scratch.md",
		})
		require.NoError(t, err)
		require.Equal(t, renamed.ID, deleted.ID)

		deletedMany, err := db.DeleteChatMemoriesByRootChatIDAndPathPrefix(ctx, database.DeleteChatMemoriesByRootChatIDAndPathPrefixParams{
			RootChatID: chat.ID,
			PathPrefix: "notes/",
		})
		require.NoError(t, err)
		require.Len(t, deletedMany, 1)

		list, err := db.ListChatMemoriesByRootChatID(ctx, chat.ID)
		require.NoError(t, err)
		require.Empty(t, list)
	})

	t.Run("HardDeleteCascades", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		doomed := insertTestChat(t, db)
		_, err := insertMemory(ctx, doomed.ID, "cascade.md")
		require.NoError(t, err)

		// No Store method hard-deletes chats; use raw SQL to
		// exercise the ON DELETE CASCADE foreign key.
		_, err = sqlDB.ExecContext(ctx, `DELETE FROM chats WHERE id = $1`, doomed.ID)
		require.NoError(t, err)

		list, err := db.ListChatMemoriesByRootChatID(ctx, doomed.ID)
		require.NoError(t, err)
		require.Empty(t, list)
	})

	t.Run("PerRootChatLimit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		limited := insertTestChat(t, db)
		for i := range 100 {
			_, err := insertMemory(ctx, limited.ID, fmt.Sprintf("memory-%03d.md", i))
			require.NoError(t, err)
		}
		_, err := insertMemory(ctx, limited.ID, "one-too-many.md")
		require.Error(t, err)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr)
		require.Equal(t, "chat_memories_per_root_chat_limit", pqErr.Constraint)
	})
}

// insertTestChat creates a minimal chat row that memory rows can reference.
func insertTestChat(t *testing.T, db database.Store) database.Chat {
	t.Helper()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
	modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:     "test-model",
		CreatedBy: uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy: uuid.NullUUID{UUID: owner.ID, Valid: true},
	})
	return dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		LastModelConfigID: modelCfg.ID,
	})
}
