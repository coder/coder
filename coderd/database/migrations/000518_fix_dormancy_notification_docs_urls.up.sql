-- Update stale docs URLs in the dormancy notification templates so that
-- they point at the current documentation path and anchors:
--   /docs/templates/schedule#dormancy-threshold-enterprise
--     -> /docs/admin/templates/managing-templates/schedule#dormancy-threshold
--   /docs/templates/schedule#dormancy-auto-deletion-enterprise
--     -> /docs/admin/templates/managing-templates/schedule#dormancy-auto-deletion
--
-- We use REPLACE on body_template, scoped by id and LIKE so the update
-- is robust to the various intermediate forms that prior migrations
-- (000232, 000262, 000305, 000311) have left on disk.

UPDATE notification_templates
SET
	body_template = REPLACE(
		REPLACE(
			body_template,
			'/docs/templates/schedule#dormancy-threshold-enterprise',
			'/docs/admin/templates/managing-templates/schedule#dormancy-threshold'
		),
		'/docs/templates/schedule#dormancy-auto-deletion-enterprise',
		'/docs/admin/templates/managing-templates/schedule#dormancy-auto-deletion'
	)
WHERE
	id IN (
		'0ea69165-ec14-4314-91f1-69566ac3c5a0',
		'51ce2fdf-c9ca-4be1-8d70-628674f9bc42'
	)
	AND body_template LIKE '%/docs/templates/schedule%';
