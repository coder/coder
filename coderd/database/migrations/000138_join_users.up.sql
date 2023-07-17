BEGIN;

CREATE VIEW
	visible_users
AS
SELECT
	id, username, avatar_url
FROM
	users;

COMMENT ON VIEW visible_users IS 'Visible fields of users are allowed to be joined with other tables for including context of other resources.';

CREATE VIEW
    template_with_users
AS
    SELECT
        templates.*,
        visible_users.username AS created_by_username,
        visible_users.avatar_url AS created_by_avatar_url
    FROM
        templates
    LEFT JOIN
		visible_users
	ON
	    templates.created_by = visible_users.id;

COMMENT ON VIEW template_with_users IS 'Joins in the username + avatar url of the created by user.';

COMMIT;
