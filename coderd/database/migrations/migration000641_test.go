package migrations_test

import (
	"database/sql"
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

	newChat := func(parentID, rootID uuid.UUID) database.Chat {
		t.Helper()
		return dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			ParentChatID:      uuid.NullUUID{UUID: parentID, Valid: parentID != uuid.Nil},
			RootChatID:        uuid.NullUUID{UUID: rootID, Valid: rootID != uuid.Nil},
		})
	}

	root := newChat(uuid.Nil, uuid.Nil)
	child := newChat(root.ID, root.ID)
	grandchild := newChat(child.ID, root.ID)
	withoutRootID := newChat(root.ID, uuid.Nil)

	otherRoot := newChat(uuid.Nil, uuid.Nil)

	archivedRoot := newChat(uuid.Nil, uuid.Nil)
	archivedChild := newChat(archivedRoot.ID, archivedRoot.ID)
	_, err := db.ArchiveChatByID(ctx, archivedRoot.ID)
	require.NoError(t, err)

	chatIDs := []uuid.UUID{
		root.ID,
		child.ID,
		grandchild.ID,
		withoutRootID.ID,
		otherRoot.ID,
		archivedRoot.ID,
		archivedChild.ID,
	}

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

	// Keep existing root metadata even if a subagent chat's row was refreshed
	// later.
	rootStatus := seedStatus(root, "shared", origin, refreshedAt.Add(-time.Hour))
	seedStatus(child, "shared", origin, refreshedAt)
	seedStatus(grandchild, "shared", origin, refreshedAt)

	// Without a root row, keep the most recently refreshed row from a
	// subagent chat.
	seedStatus(child, "subagent-only", origin, refreshedAt.Add(-time.Hour))
	subagentStatus := seedStatus(grandchild, "subagent-only", origin, refreshedAt)

	// The same branch on another origin is a separate ref.
	otherOriginStatus := seedStatus(child, "shared", "https://github.com/coder/other", refreshedAt)

	// A subagent chat without root_chat_id uses its parent.
	fallbackStatus := seedStatus(withoutRootID, "fallback", origin, refreshedAt)

	// Refs from archived subagent chats also move to the root chat.
	archivedStatus := seedStatus(archivedChild, "archived", origin, refreshedAt)

	// An unrelated root chat retains its row for the same ref.
	otherRootStatus := seedStatus(otherRoot, "shared", origin, refreshedAt)

	// Refs without PR metadata also move to the root chat.
	noPRStatus, err := db.UpsertChatDiffStatusReference(ctx, database.UpsertChatDiffStatusReferenceParams{
		ChatID:          child.ID,
		GitRemoteOrigin: origin,
		GitBranch:       "no-pr",
		StaleAt:         refreshedAt,
	})
	require.NoError(t, err)

	movedTo := func(chat database.Chat, status database.ChatDiffStatus) database.ChatDiffStatus {
		status.ChatID = chat.ID
		return status
	}
	want := []database.ChatDiffStatus{
		rootStatus,
		movedTo(root, subagentStatus),
		movedTo(root, otherOriginStatus),
		movedTo(root, fallbackStatus),
		movedTo(archivedRoot, archivedStatus),
		otherRootStatus,
		movedTo(root, noPRStatus),
	}

	stepTo(t, sqlDB, 641)

	statuses, err := db.GetChatDiffStatusesByChatIDs(ctx, chatIDs)
	require.NoError(t, err)
	require.ElementsMatch(t, want, statuses)
}
