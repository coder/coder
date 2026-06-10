-- Revert the URL replacements applied by 000510. We use the reverse
-- REPLACE so any other downstream edits to body_template are preserved.

UPDATE notification_templates
SET
	body_template = REPLACE(
		REPLACE(
			body_template,
			'/docs/admin/templates/managing-templates/schedule#dormancy-threshold',
			'/docs/templates/schedule#dormancy-threshold-enterprise'
		),
		'/docs/admin/templates/managing-templates/schedule#dormancy-auto-deletion',
		'/docs/templates/schedule#dormancy-auto-deletion-enterprise'
	)
WHERE
	id IN (
		'0ea69165-ec14-4314-91f1-69566ac3c5a0',
		'51ce2fdf-c9ca-4be1-8d70-628674f9bc42'
	)
	AND body_template LIKE '%/docs/admin/templates/managing-templates/schedule%';
