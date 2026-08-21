-- Inserts memory fixtures so migration coverage includes both tables.
INSERT INTO user_memories (
	id,
	user_id,
	path,
	content,
	created_at,
	updated_at
) VALUES (
	'e2f7d9b1-64a0-4f6a-9d2c-5b8f3a2c9001',
	'30095c71-380b-457a-8995-97b8ee6e5307',
	'preferences/example.md',
	'Example user memory content.',
	'2026-08-20 00:00:00+00',
	'2026-08-20 00:00:00+00'
);

-- The earlier chat fixtures already insert at least one row into chats;
-- attach a chat memory to the first such chat so migration tests see a
-- non-empty chat_memories table without hard-coding a specific chat ID.
INSERT INTO chat_memories (
	id,
	root_chat_id,
	path,
	content,
	created_at,
	updated_at
)
SELECT
	'e2f7d9b1-64a0-4f6a-9d2c-5b8f3a2c9002',
	chats.id,
	'notes/example.md',
	'Example chat memory content.',
	'2026-08-20 00:00:00+00',
	'2026-08-20 00:00:00+00'
FROM chats
WHERE parent_chat_id IS NULL AND root_chat_id IS NULL
ORDER BY created_at, id
LIMIT 1;
