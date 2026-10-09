INSERT INTO skills (
	id,
	organization_id,
	name,
	description,
	content,
	enabled,
	group_acl,
	created_at,
	updated_at
) VALUES (
	'7f070eb2-991e-4f7f-b780-40c4e0f49002',
	'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
	'example-skill',
	'Example organization skill fixture.',
	'Example content.',
	false,
	'{"bb640d07-ca8a-4869-b6bc-ae61ebb2fda1": {"permissions": ["read"]}}',
	'2026-10-09 00:00:00+00',
	'2026-10-09 00:00:00+00'
);
