BEGIN;

DROP VIEW template_with_users;

ALTER TABLE templates
	ADD COLUMN autostart_block_days_of_week smallint NOT NULL DEFAULT 0;

COMMENT ON COLUMN templates.autostart_block_days_of_week IS 'A bitmap of days of week that autostart of a workspace is not allowed. Default allows all days. This is intended as a cost savings measure to prevent auto start on weekends (for example).';

-- Recreate view
CREATE VIEW
	template_with_users
AS
SELECT
	templates.*,
	coalesce(visible_users.avatar_url, '') AS created_by_avatar_url,
	coalesce(visible_users.username, '') AS created_by_username
FROM
	templates
		LEFT JOIN
	visible_users
	ON
			templates.created_by = visible_users.id;

COMMENT ON VIEW template_with_users IS 'Joins in the username + avatar url of the created by user.';

COMMIT;
