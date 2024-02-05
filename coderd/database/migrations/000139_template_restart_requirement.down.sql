-- Delete the new version of the template_with_users view to remove the column
-- dependency.
DROP VIEW template_with_users;

ALTER TABLE templates
	DROP COLUMN restart_requirement_days_of_week,
	DROP COLUMN restart_requirement_weeks;

ALTER TABLE users DROP COLUMN quiet_hours_schedule;

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
