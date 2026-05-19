-- Inserts a user skill fixture so migration coverage includes the table.
INSERT INTO user_skills (
	id,
	user_id,
	name,
	description,
	content,
	created_at,
	updated_at
) VALUES (
	'7f070eb2-991e-4f7f-b780-40c4e0f49001',
	'30095c71-380b-457a-8995-97b8ee6e5307',
	'example-skill',
	'Example skill fixture.',
	'Example content.',
	'2026-05-07 00:00:00+00',
	'2026-05-07 00:00:00+00'
);
