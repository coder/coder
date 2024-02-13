CREATE TABLE workspace_agent_port_share (
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	agent_name text NOT NULL,
	port integer NOT NULL,
	share_level app_sharing_level NOT NULL
);

ALTER TABLE workspace_agent_port_share ADD PRIMARY KEY (workspace_id, agent_name, port);

ALTER TABLE templates ADD COLUMN max_port_sharing_level app_sharing_level NOT NULL DEFAULT 'owner'::app_sharing_level;

-- Update the template_with_users view by recreating it.
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
