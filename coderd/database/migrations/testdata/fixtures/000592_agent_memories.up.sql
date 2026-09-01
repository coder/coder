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

-- Attach a chat memory to the earliest root chat from the earlier chat
-- fixtures without hard-coding a specific chat ID. The scalar subquery keeps
-- the failure mode loud: if no root chat fixture exists, the NULL violates
-- the root_chat_id NOT NULL constraint instead of silently inserting zero
-- rows and losing chat_memories migration coverage.
INSERT INTO chat_memories (
	id,
	root_chat_id,
	path,
	content,
	created_at,
	updated_at
) VALUES (
	'e2f7d9b1-64a0-4f6a-9d2c-5b8f3a2c9002',
	(
		SELECT id FROM chats
		WHERE parent_chat_id IS NULL AND root_chat_id IS NULL
		ORDER BY created_at, id
		LIMIT 1
	),
	'notes/example.md',
	'Example chat memory content.',
	'2026-08-20 00:00:00+00',
	'2026-08-20 00:00:00+00'
);
