CREATE VIEW
	visible_users
AS
SELECT
	id, username, avatar_url
FROM
	users;

COMMENT ON VIEW visible_users IS 'Visible fields of users are allowed to be joined with other tables for including context of other resources.';

-- If you need to update this view, put 'DROP VIEW template_with_users;' before this.
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
