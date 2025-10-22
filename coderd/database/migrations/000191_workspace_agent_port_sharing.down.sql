DROP TABLE workspace_agent_port_share;
DROP VIEW template_with_users;
ALTER TABLE templates DROP COLUMN max_port_sharing_level;

-- Update the template_with_users view by recreating it.

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
