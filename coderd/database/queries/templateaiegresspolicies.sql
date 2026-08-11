-- name: GetTemplateAIEgressPolicy :one
SELECT *
FROM template_ai_egress_policies
WHERE template_id = $1
ORDER BY revision DESC
LIMIT 1;

-- name: InsertTemplateAIEgressPolicy :one
INSERT INTO template_ai_egress_policies (
	template_id,
	revision,
	rules,
	created_by
)
SELECT
	sqlc.arg(template_id),
	COALESCE(MAX(revision), 0) + 1,
	sqlc.arg(rules)::jsonb,
	sqlc.arg(created_by)
FROM template_ai_egress_policies
WHERE template_id = sqlc.arg(template_id)
RETURNING *;
