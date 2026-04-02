-- name: InsertChatSharedSnapshot :one
INSERT INTO chat_shared_snapshots (
	id, token, chat_id, owner_id,
	chat_title, chat_status,
	messages, snapshot_at, expires_at, created_at
)
VALUES (
	$1, $2, $3, $4,
	$5, $6,
	$7, $8, $9, $10
)
RETURNING *;

-- name: GetChatSharedSnapshotByToken :one
SELECT * FROM chat_shared_snapshots WHERE token = $1;

-- name: GetChatSharedSnapshotsByChatID :many
SELECT * FROM chat_shared_snapshots
WHERE chat_id = $1
ORDER BY created_at DESC;

-- name: DeleteChatSharedSnapshot :exec
DELETE FROM chat_shared_snapshots WHERE id = $1;
