-- name: ListChatNotes :many
SELECT
    path,
    octet_length(content)::bigint AS bytes,
    updated_at
FROM chat_notes
WHERE chat_id = @chat_id::uuid
ORDER BY path;

-- name: GetChatNote :one
SELECT *
FROM chat_notes
WHERE chat_id = @chat_id::uuid AND path = @path::text;

-- name: UpsertChatNote :one
INSERT INTO chat_notes (chat_id, path, content)
VALUES (@chat_id::uuid, @path::text, @content::text)
ON CONFLICT (chat_id, path) DO UPDATE
SET content = EXCLUDED.content, updated_at = now()
RETURNING *;

-- AppendChatNote adds content as a new line at the end of a note, and
-- creates the note when it does not exist. The table check keeps the
-- 16 KiB cap.
-- name: AppendChatNote :one
INSERT INTO chat_notes (chat_id, path, content)
VALUES (@chat_id::uuid, @path::text, @content::text)
ON CONFLICT (chat_id, path) DO UPDATE
SET
    content = CASE
        WHEN chat_notes.content = '' THEN EXCLUDED.content
        ELSE chat_notes.content || E'\n' || EXCLUDED.content
    END,
    updated_at = now()
RETURNING *;

-- name: CountChatNotes :one
SELECT COUNT(*)::bigint
FROM chat_notes
WHERE chat_id = @chat_id::uuid;

-- name: GetChatNotesChangedSince :one
SELECT EXISTS (
    SELECT 1
    FROM chat_notes
    WHERE chat_id = @chat_id::uuid AND updated_at > @since::timestamptz
)::boolean AS changed;
