-- Inserts an agent memory fixture so migration coverage includes the table.
INSERT INTO agent_memories (
	id,
	user_id,
	path,
	content,
	created_at,
	updated_at
) VALUES (
	'0bb9f154-959a-4699-9ed4-7c9773af5c9a',
	'30095c71-380b-457a-8995-97b8ee6e5307',
	'/fixture/example.md',
	'# Agent memory fixture',
	'2026-07-21 00:00:00+00',
	'2026-07-21 00:00:00+00'
);
