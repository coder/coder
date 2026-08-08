-- Update the template_with_users view by recreating it.
DROP VIEW template_with_users;

ALTER TABLE "templates" ADD COLUMN "max_ttl" bigint DEFAULT '0'::bigint NOT NULL;
-- Most templates should have this set to false by now.
ALTER TABLE templates ADD COLUMN use_max_ttl boolean NOT NULL DEFAULT false;

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
