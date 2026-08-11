INSERT INTO template_ai_egress_policies (
	template_id,
	revision,
	rules,
	created_at,
	created_by
)
SELECT
	templates.id,
	1,
	'[{"host":"example.com","ports":[443]}]'::jsonb,
	'2024-01-01 00:00:00+00',
	users.id
FROM templates
CROSS JOIN users
ORDER BY templates.created_at, templates.id, users.created_at, users.id
LIMIT 1;
