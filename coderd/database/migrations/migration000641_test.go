package migrations_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

func TestMigration000641ChatDiffStatusesRootChats(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 640)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Historical fixtures must not depend on current sqlc column lists.
	root, _ := seedChatWithStatus(ctx, t, sqlDB, "waiting")
	newChat := func(parentID, rootID uuid.UUID) uuid.UUID {
		t.Helper()
		row := sqlDB.QueryRowContext(ctx, `
			INSERT INTO chats (
				id,
				owner_id,
				organization_id,
				last_model_config_id,
				title,
				status,
				parent_chat_id,
				root_chat_id
			)
			SELECT
				$1,
				owner_id,
				organization_id,
				last_model_config_id,
				'fixture-chat',
				'waiting',
				$2,
				$3
			FROM chats
			WHERE id = $4
			RETURNING id`,
			uuid.New(),
			uuid.NullUUID{UUID: parentID, Valid: parentID != uuid.Nil},
			uuid.NullUUID{UUID: rootID, Valid: rootID != uuid.Nil},
			root,
		)
		var chatID uuid.UUID
		err := row.Scan(&chatID)
		require.NoError(t, err)
		return chatID
	}

	child := newChat(root, root)
	grandchild := newChat(child, root)
	referenceOnlyChild := newChat(root, root)
	withoutRootID := newChat(root, uuid.Nil)

	otherRoot := newChat(uuid.Nil, uuid.Nil)

	archivedRoot := newChat(uuid.Nil, uuid.Nil)
	archivedChild := newChat(archivedRoot, archivedRoot)
	_, err := sqlDB.ExecContext(ctx,
		`UPDATE chats SET archived = true WHERE id = $1 OR root_chat_id = $1`, archivedRoot)
	require.NoError(t, err)

	// Compare every historical status field, with chat_id asserted separately.
	type diffStatusRow struct {
		chatID uuid.UUID
		fields map[string]any
	}
	decodeStatus := func(chatID uuid.UUID, raw []byte) diffStatusRow {
		t.Helper()
		var fields map[string]any
		err := json.Unmarshal(raw, &fields)
		require.NoError(t, err)
		return diffStatusRow{chatID: chatID, fields: fields}
	}

	seedStatus := func(chatID uuid.UUID, branch, remoteOrigin string, refreshed, reported time.Time) diffStatusRow {
		t.Helper()
		row := sqlDB.QueryRowContext(ctx, `
			INSERT INTO chat_diff_statuses (
				chat_id,
				git_remote_origin,
				git_branch,
				url,
				pull_request_state,
				pull_request_title,
				pull_request_draft,
				changes_requested,
				additions,
				deletions,
				changed_files,
				author_login,
				author_avatar_url,
				base_branch,
				head_branch,
				pr_number,
				commits,
				approved,
				reviewer_count,
				refreshed_at,
				stale_at,
				created_at,
				updated_at
			) VALUES (
				$1,
				$2,
				$3,
				'https://github.com/coder/coder/pull/1',
				'open',
				$4,
				true,
				true,
				10,
				3,
				2,
				'author',
				'https://example.com/avatar',
				'main',
				$3,
				1,
				5,
				true,
				2,
				$5,
				$5::timestamptz + INTERVAL '1 minute',
				$6::timestamptz - INTERVAL '24 hours',
				$6
			)
			RETURNING chat_id, to_jsonb(chat_diff_statuses) - 'chat_id'`,
			chatID, remoteOrigin, branch, chatID.String(), refreshed, reported)
		var rowChatID uuid.UUID
		var raw []byte
		err := row.Scan(&rowChatID, &raw)
		require.NoError(t, err)
		return decodeStatus(rowChatID, raw)
	}

	seedReference := func(chatID uuid.UUID, branch, remoteOrigin string, reported time.Time) diffStatusRow {
		t.Helper()
		row := sqlDB.QueryRowContext(ctx, `
			INSERT INTO chat_diff_statuses (
				chat_id,
				git_remote_origin,
				git_branch,
				stale_at,
				created_at,
				updated_at
			) VALUES (
				$1,
				$2,
				$3,
				$4,
				$4::timestamptz - INTERVAL '24 hours',
				$4
			)
			RETURNING chat_id, to_jsonb(chat_diff_statuses) - 'chat_id'`,
			chatID, remoteOrigin, branch, reported)
		var rowChatID uuid.UUID
		var raw []byte
		err := row.Scan(&rowChatID, &raw)
		require.NoError(t, err)
		return decodeStatus(rowChatID, raw)
	}

	origin := "https://github.com/coder/coder"
	reportedAt := dbtime.Now().Add(-2 * time.Hour)
	refreshedAt := reportedAt.Add(time.Hour)

	// Keep the existing root row even if a subagent chat's row has later
	// refreshed_at and updated_at values.
	rootStatus := seedStatus(root, "shared", origin, refreshedAt.Add(-time.Hour), reportedAt)
	seedStatus(child, "shared", origin, refreshedAt, reportedAt.Add(time.Minute))
	seedStatus(grandchild, "shared", origin, refreshedAt, reportedAt.Add(2*time.Minute))

	// The latest refreshed_at takes precedence over a later updated_at,
	// including a reference-only row whose refreshed_at is NULL.
	subagentStatus := seedStatus(grandchild, "subagent-only", origin, refreshedAt, reportedAt)
	seedStatus(child, "subagent-only", origin, refreshedAt.Add(-time.Hour), reportedAt.Add(time.Minute))
	seedReference(referenceOnlyChild, "subagent-only", origin, reportedAt.Add(2*time.Minute))

	// The same branch on another origin is a separate ref.
	otherOriginStatus := seedStatus(child, "shared", "https://github.com/coder/other", refreshedAt, reportedAt)

	// A subagent chat without root_chat_id uses its parent.
	fallbackStatus := seedStatus(withoutRootID, "fallback", origin, refreshedAt, reportedAt)

	// Refs from archived subagent chats also move to the root chat.
	archivedStatus := seedStatus(archivedChild, "archived", origin, refreshedAt, reportedAt)

	// An unrelated root chat retains its row for the same ref.
	otherRootStatus := seedStatus(otherRoot, "shared", origin, refreshedAt, reportedAt)

	// When refreshed_at is NULL for both rows, retain the later updated_at.
	seedReference(child, "no-pr", origin, reportedAt)
	noPRStatus := seedReference(grandchild, "no-pr", origin, reportedAt.Add(time.Minute))

	// When both timestamps tie, retain the row with the lower chat_id.
	lowerChatID, higherChatID := child, grandchild
	if lowerChatID.String() > higherChatID.String() {
		lowerChatID, higherChatID = higherChatID, lowerChatID
	}
	seedStatus(higherChatID, "chat-id-tie", origin, refreshedAt, reportedAt)
	chatIDTieStatus := seedStatus(lowerChatID, "chat-id-tie", origin, refreshedAt, reportedAt)

	movedTo := func(chatID uuid.UUID, status diffStatusRow) diffStatusRow {
		status.chatID = chatID
		return status
	}
	want := []diffStatusRow{
		rootStatus,
		movedTo(root, subagentStatus),
		movedTo(root, otherOriginStatus),
		movedTo(root, fallbackStatus),
		movedTo(archivedRoot, archivedStatus),
		otherRootStatus,
		movedTo(root, noPRStatus),
		movedTo(root, chatIDTieStatus),
	}

	stepTo(t, sqlDB, 641)

	// Read the complete table so unexpected rows cannot be omitted by an ID filter.
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT chat_id, to_jsonb(chat_diff_statuses) - 'chat_id' FROM chat_diff_statuses`)
	require.NoError(t, err)
	defer rows.Close()

	var statuses []diffStatusRow
	for rows.Next() {
		var chatID uuid.UUID
		var raw []byte
		err := rows.Scan(&chatID, &raw)
		require.NoError(t, err)
		statuses = append(statuses, decodeStatus(chatID, raw))
	}
	require.NoError(t, rows.Err())
	require.ElementsMatch(t, want, statuses)
}
