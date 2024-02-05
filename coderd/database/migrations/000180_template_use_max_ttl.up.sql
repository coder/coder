-- Add column with default true, so existing templates will function as usual
ALTER TABLE templates ADD COLUMN use_max_ttl boolean NOT NULL DEFAULT true;

-- Find any templates with autostop_requirement_days_of_week set and set them to
-- use_max_ttl = false
UPDATE templates SET use_max_ttl = false WHERE autostop_requirement_days_of_week != 0;

-- Alter column to default false, because we want autostop_requirement to be the
-- default from now on
ALTER TABLE templates ALTER COLUMN use_max_ttl SET DEFAULT false;

DROP VIEW template_with_users;

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
