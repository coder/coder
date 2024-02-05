ALTER TABLE templates RENAME COLUMN inactivity_ttl TO time_til_dormant;
ALTER TABLE templates RENAME COLUMN locked_ttl TO time_til_dormant_autodelete;
ALTER TABLE workspaces RENAME COLUMN locked_at TO dormant_at;

-- Update the template_with_users view;a
DROP VIEW template_with_users;
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
