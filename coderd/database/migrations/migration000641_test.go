package migrations_test

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

func TestMigration000641ChatDiffStatusesRootChats(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 640)
	ctx := testutil.Context(t, testutil.WaitLong)
	db := database.New(sqlDB)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: org.ID})

	var chatIDs []uuid.UUID
	newChat := func(parentID, rootID uuid.NullUUID) database.Chat {
		t.Helper()
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			ParentChatID:      parentID,
			RootChatID:        rootID,
		})
		chatIDs = append(chatIDs, chat.ID)
		return chat
	}
	root := newChat(uuid.NullUUID{}, uuid.NullUUID{})
	rootID := uuid.NullUUID{UUID: root.ID, Valid: true}
	child := newChat(rootID, rootID)
	grandchild := newChat(uuid.NullUUID{UUID: child.ID, Valid: true}, rootID)
	withoutRootID := newChat(rootID, uuid.NullUUID{})
	otherRoot := newChat(uuid.NullUUID{}, uuid.NullUUID{})
	archivedRoot := newChat(uuid.NullUUID{}, uuid.NullUUID{})
	archivedRootID := uuid.NullUUID{UUID: archivedRoot.ID, Valid: true}
	archivedChild := newChat(archivedRootID, archivedRootID)
	_, err := db.ArchiveChatByID(ctx, archivedRoot.ID)
	require.NoError(t, err)

	origin := "https://github.com/coder/coder"
	refreshedAt := dbtime.Now().Add(-time.Hour)
	seedStatus := func(chat database.Chat, branch, remoteOrigin string, refreshed time.Time) database.ChatDiffStatus {
		t.Helper()
		status, err := db.UpsertChatDiffStatus(ctx, database.UpsertChatDiffStatusParams{
			ChatID:           chat.ID,
			GitRemoteOrigin:  remoteOrigin,
			GitBranch:        branch,
			Url:              sql.NullString{String: "https://github.com/coder/coder/pull/1", Valid: true},
			PullRequestState: sql.NullString{String: "open", Valid: true},
			PullRequestTitle: chat.ID.String(),
			PullRequestDraft: true,
			ChangesRequested: true,
			Additions:        10,
			Deletions:        3,
			ChangedFiles:     2,
			AuthorLogin:      sql.NullString{String: "author", Valid: true},
			AuthorAvatarUrl:  sql.NullString{String: "https://example.com/avatar", Valid: true},
			BaseBranch:       sql.NullString{String: "main", Valid: true},
			HeadBranch:       sql.NullString{String: branch, Valid: true},
			PrNumber:         sql.NullInt32{Int32: 1, Valid: true},
			Commits:          sql.NullInt32{Int32: 5, Valid: true},
			Approved:         sql.NullBool{Bool: true, Valid: true},
			ReviewerCount:    sql.NullInt32{Int32: 2, Valid: true},
			RefreshedAt:      refreshed,
			StaleAt:          refreshed.Add(time.Minute),
		})
		require.NoError(t, err)
		return status
	}

	// Keep existing root metadata even if a subagent was refreshed later.
	rootStatus := seedStatus(root, "shared", origin, refreshedAt.Add(-time.Hour))
	seedStatus(child, "shared", origin, refreshedAt)
	seedStatus(grandchild, "shared", origin, refreshedAt)

	// Without a root row, keep the most recently refreshed subagent row.
	seedStatus(child, "subagent-only", origin, refreshedAt.Add(-time.Hour))
	subagentStatus := seedStatus(grandchild, "subagent-only", origin, refreshedAt)
	subagentStatus.ChatID = root.ID

	otherOriginStatus := seedStatus(child, "shared", "https://github.com/coder/other", refreshedAt)
	otherOriginStatus.ChatID = root.ID
	fallbackStatus := seedStatus(withoutRootID, "fallback", origin, refreshedAt)
	fallbackStatus.ChatID = root.ID
	archivedStatus := seedStatus(archivedChild, "archived", origin, refreshedAt)
	archivedStatus.ChatID = archivedRoot.ID
	otherRootStatus := seedStatus(otherRoot, "shared", origin, refreshedAt)

	noPRStatus, err := db.UpsertChatDiffStatusReference(ctx, database.UpsertChatDiffStatusReferenceParams{
		ChatID:          child.ID,
		GitRemoteOrigin: origin,
		GitBranch:       "no-pr",
		StaleAt:         refreshedAt,
	})
	require.NoError(t, err)
	noPRStatus.ChatID = root.ID

	want := []database.ChatDiffStatus{
		rootStatus,
		subagentStatus,
		otherOriginStatus,
		fallbackStatus,
		archivedStatus,
		otherRootStatus,
		noPRStatus,
	}

	stepTo(t, sqlDB, 641)
	statuses, err := db.GetChatDiffStatusesByChatIDs(ctx, chatIDs)
	require.NoError(t, err)
	require.Len(t, statuses, len(want))
	require.ElementsMatch(t, want, statuses)

	downSQL, err := os.ReadFile("000641_chat_diff_statuses_root_chats.down.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(downSQL))
	require.NoError(t, err)
	statuses, err = db.GetChatDiffStatusesByChatIDs(ctx, chatIDs)
	require.NoError(t, err)
	require.Len(t, statuses, len(want))
	require.ElementsMatch(t, want, statuses)
}
