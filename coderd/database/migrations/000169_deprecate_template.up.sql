BEGIN;

-- The view will be rebuilt with the new column
DROP VIEW template_with_users;

ALTER TABLE templates
	ADD COLUMN deprecated TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN templates.deprecated IS 'If set to a non empty string, the template will no longer be able to be used. The message will be displayed to the user.';

-- Restore the old version of the template_with_users view.
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
