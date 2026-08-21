package database_test

import (
	"context"
	"database/sql"
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

var invalidMemoryPaths = []string{
	"", "no-extension", "dir/", "/absolute.md", "a//b.md",
	"trailing/.md", "spaces in path.md", "note.txt",
	"./local.md", "../escape.md", "dir/./local.md", "dir/../escape.md",
}

func TestAgentMemorySchemaConstants(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	ctx := testutil.Context(t, testutil.WaitMedium)
	_, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	for trigger, limit := range map[string]int{
		"enforce_user_memories_insert_invariants": 100,
		"enforce_chat_memories_insert_invariants": 100,
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

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

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
		for _, invalid := range invalidMemoryPaths {
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

		_, err = db.UpdateUserMemoryByUserIDAndPath(ctx, database.UpdateUserMemoryByUserIDAndPathParams{
			UserID:  user.ID,
			Path:    "root.md",
			Content: strings.Repeat("a", 65537),
		})
		require.Error(t, err)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr)
		require.Equal(t, "user_memories_content_size", pqErr.Constraint)

		_, err = db.RenameUserMemoryByUserIDAndPath(ctx, database.RenameUserMemoryByUserIDAndPathParams{
			UserID:  user.ID,
			OldPath: "root.md",
			NewPath: "../escape.md",
		})
		require.Error(t, err)
		require.ErrorAs(t, err, &pqErr)
		require.Equal(t, "user_memories_path_format", pqErr.Constraint)

		_, err = db.RenameUserMemoryByUserIDAndPath(ctx, database.RenameUserMemoryByUserIDAndPathParams{
			UserID:  user.ID,
			OldPath: "root.md",
			NewPath: "keep.md",
		})
		require.Error(t, err)
		require.True(t, database.IsUniqueViolation(err, database.UniqueUserMemoriesUserIDPathIndex))

		_, err = db.UpdateUserMemoryByUserIDAndPath(ctx, database.UpdateUserMemoryByUserIDAndPathParams{
			UserID:  user.ID,
			Path:    "missing.md",
			Content: "updated",
		})
		require.ErrorIs(t, err, sql.ErrNoRows)

		_, err = db.RenameUserMemoryByUserIDAndPath(ctx, database.RenameUserMemoryByUserIDAndPathParams{
			UserID:  user.ID,
			OldPath: "missing.md",
			NewPath: "new.md",
		})
		require.ErrorIs(t, err, sql.ErrNoRows)

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

		deletedByEmptyPrefix, err := db.DeleteUserMemoriesByUserIDAndPathPrefix(ctx, database.DeleteUserMemoriesByUserIDAndPathPrefixParams{
			UserID:     user.ID,
			PathPrefix: "",
		})
		require.NoError(t, err)
		require.Empty(t, deletedByEmptyPrefix)

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

	t.Run("SoftDeleteWinsConcurrentInsert", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		user := dbgen.User(t, db, database.User{})

		deleteTx, err := sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		committed := false
		t.Cleanup(func() {
			if !committed {
				_ = deleteTx.Rollback()
			}
		})

		var lockedUserID uuid.UUID
		err = deleteTx.QueryRowContext(ctx,
			`SELECT id FROM users WHERE id = $1 FOR UPDATE`, user.ID,
		).Scan(&lockedUserID)
		require.NoError(t, err)
		require.Equal(t, user.ID, lockedUserID)

		insertConn, err := sqlDB.Conn(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { _ = insertConn.Close() })

		var insertPID int
		err = insertConn.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&insertPID)
		require.NoError(t, err)

		insertResult := make(chan error, 1)
		go func() {
			_, err := insertConn.ExecContext(ctx, `
				INSERT INTO user_memories (id, user_id, path, content)
				VALUES ($1, $2, 'concurrent.md', 'content')
			`, uuid.New(), user.ID)
			insertResult <- err
		}()

		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			var lockWaits int
			err := sqlDB.QueryRowContext(ctx, `
				SELECT count(*)
				FROM pg_stat_activity
				WHERE pid = $1 AND wait_event_type = 'Lock'
			`, insertPID).Scan(&lockWaits)
			return err == nil && lockWaits == 1
		}, testutil.IntervalFast, "wait for the memory insert to block on the user row")
		require.NoError(t, ctx.Err(), "waiting for the memory insert")

		_, err = deleteTx.ExecContext(ctx, `UPDATE users SET deleted = true WHERE id = $1`, user.ID)
		require.NoError(t, err)
		require.NoError(t, deleteTx.Commit())
		committed = true

		select {
		case err := <-insertResult:
			require.Error(t, err)
			var pqErr *pq.Error
			require.ErrorAs(t, err, &pqErr)
			require.Equal(t, "user_memory_user_deleted", pqErr.Constraint)
		case <-ctx.Done():
			require.Failf(t, "memory insert did not finish", "context ended: %v", ctx.Err())
		}

		list, err := db.ListUserMemoriesByUserID(ctx, user.ID)
		require.NoError(t, err)
		require.Empty(t, list)
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

	t.Run("ConcurrentInsertPerUserLimit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		limited := dbgen.User(t, db, database.User{})
		for i := range 99 {
			_, err := insertMemory(ctx, limited.ID, fmt.Sprintf("memory-%03d.md", i))
			require.NoError(t, err)
		}

		limitTx, err := sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		committed := false
		t.Cleanup(func() {
			if !committed {
				_ = limitTx.Rollback()
			}
		})
		_, err = limitTx.ExecContext(ctx, `
			INSERT INTO user_memories (id, user_id, path, content)
			VALUES ($1, $2, 'memory-099.md', 'content')
		`, uuid.New(), limited.ID)
		require.NoError(t, err)

		insertConn, err := sqlDB.Conn(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { _ = insertConn.Close() })

		var insertPID int
		err = insertConn.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&insertPID)
		require.NoError(t, err)

		insertResult := make(chan error, 1)
		go func() {
			_, err := insertConn.ExecContext(ctx, `
				INSERT INTO user_memories (id, user_id, path, content)
				VALUES ($1, $2, 'one-too-many.md', 'content')
			`, uuid.New(), limited.ID)
			insertResult <- err
		}()

		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			var lockWaits int
			err := sqlDB.QueryRowContext(ctx, `
				SELECT count(*)
				FROM pg_stat_activity
				WHERE pid = $1 AND wait_event_type = 'Lock'
			`, insertPID).Scan(&lockWaits)
			return err == nil && lockWaits == 1
		}, testutil.IntervalFast, "wait for the second memory insert to block on the user row")
		require.NoError(t, ctx.Err(), "waiting for the second memory insert")

		require.NoError(t, limitTx.Commit())
		committed = true

		select {
		case err := <-insertResult:
			require.Error(t, err)
			var pqErr *pq.Error
			require.ErrorAs(t, err, &pqErr)
			require.Equal(t, "user_memories_per_user_limit", pqErr.Constraint)
		case <-ctx.Done():
			require.Failf(t, "memory insert did not finish", "context ended: %v", ctx.Err())
		}

		list, err := db.ListUserMemoriesByUserID(ctx, limited.ID)
		require.NoError(t, err)
		require.Len(t, list, 100)
	})
}

func TestChatMemories(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

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
		for _, invalid := range invalidMemoryPaths {
			_, err := insertMemory(ctx, chat.ID, invalid)
			require.Error(t, err, "path %q should be rejected", invalid)
			var pqErr *pq.Error
			require.ErrorAs(t, err, &pqErr)
			require.Equal(t, "chat_memories_path_format", pqErr.Constraint)
		}
	})

	t.Run("ContentSizeRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat := insertTestChat(t, db)
		_, err := db.InsertChatMemory(ctx, database.InsertChatMemoryParams{
			ID:         uuid.New(),
			RootChatID: chat.ID,
			Path:       "big.md",
			Content:    strings.Repeat("a", 65537),
		})
		require.Error(t, err)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr)
		require.Equal(t, "chat_memories_content_size", pqErr.Constraint)
	})

	t.Run("SubagentChatRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		root := insertTestChat(t, db)
		child := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    root.OrganizationID,
			OwnerID:           root.OwnerID,
			LastModelConfigID: root.LastModelConfigID,
			ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
		})

		_, err := insertMemory(ctx, child.ID, "rejected.md")
		require.Error(t, err)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr)
		require.Equal(t, "chat_memory_root_chat_required", pqErr.Constraint)
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

		_, err = db.UpdateChatMemoryByRootChatIDAndPath(ctx, database.UpdateChatMemoryByRootChatIDAndPathParams{
			RootChatID: chat.ID,
			Path:       "scratch.md",
			Content:    strings.Repeat("a", 65537),
		})
		require.Error(t, err)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr)
		require.Equal(t, "chat_memories_content_size", pqErr.Constraint)

		_, err = db.RenameChatMemoryByRootChatIDAndPath(ctx, database.RenameChatMemoryByRootChatIDAndPathParams{
			RootChatID: chat.ID,
			OldPath:    "scratch.md",
			NewPath:    "../escape.md",
		})
		require.Error(t, err)
		require.ErrorAs(t, err, &pqErr)
		require.Equal(t, "chat_memories_path_format", pqErr.Constraint)

		_, err = db.RenameChatMemoryByRootChatIDAndPath(ctx, database.RenameChatMemoryByRootChatIDAndPathParams{
			RootChatID: chat.ID,
			OldPath:    "scratch.md",
			NewPath:    "notes/decisions.md",
		})
		require.Error(t, err)
		require.True(t, database.IsUniqueViolation(err, database.UniqueChatMemoriesRootChatIDPathIndex))

		_, err = db.UpdateChatMemoryByRootChatIDAndPath(ctx, database.UpdateChatMemoryByRootChatIDAndPathParams{
			RootChatID: chat.ID,
			Path:       "missing.md",
			Content:    "updated",
		})
		require.ErrorIs(t, err, sql.ErrNoRows)

		_, err = db.RenameChatMemoryByRootChatIDAndPath(ctx, database.RenameChatMemoryByRootChatIDAndPathParams{
			RootChatID: chat.ID,
			OldPath:    "missing.md",
			NewPath:    "new.md",
		})
		require.ErrorIs(t, err, sql.ErrNoRows)

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

		deletedByEmptyPrefix, err := db.DeleteChatMemoriesByRootChatIDAndPathPrefix(ctx, database.DeleteChatMemoriesByRootChatIDAndPathPrefixParams{
			RootChatID: chat.ID,
			PathPrefix: "",
		})
		require.NoError(t, err)
		require.Empty(t, deletedByEmptyPrefix)

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
