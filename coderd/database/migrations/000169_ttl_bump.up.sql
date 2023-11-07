BEGIN;

DROP VIEW template_with_users;

ALTER TABLE templates ADD COLUMN default_ttl_bump bigint DEFAULT '-1'::bigint NOT NULL;
COMMENT ON COLUMN templates.default_ttl_bump IS 'Amount of time to bump workspace ttl from activity. Anything <0 will default to the ttl time.';


ALTER TABLE workspaces ADD COLUMN ttl_bump bigint DEFAULT '-1'::bigint NOT NULL;
COMMENT ON COLUMN workspaces.ttl_bump IS 'Amount of time to bump workspace ttl from activity. Anything <0 will default to the ttl time.';

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
