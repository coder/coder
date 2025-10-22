-- Drop the view that depends on the templates table
DROP VIEW template_with_names;

-- Add 'organization' to the app_sharing_level enum
CREATE TYPE new_app_sharing_level AS ENUM (
	'owner',
	'authenticated',
	'organization',
	'public'
);

-- Update workspace_agent_port_share table to use new enum
ALTER TABLE workspace_agent_port_share
	ALTER COLUMN share_level TYPE new_app_sharing_level USING (share_level::text::new_app_sharing_level);

-- Update workspace_apps table to use new enum
ALTER TABLE workspace_apps
	ALTER COLUMN sharing_level DROP DEFAULT,
	ALTER COLUMN sharing_level TYPE new_app_sharing_level USING (sharing_level::text::new_app_sharing_level),
	ALTER COLUMN sharing_level SET DEFAULT 'owner'::new_app_sharing_level;

-- Update templates table to use new enum
ALTER TABLE templates
	ALTER COLUMN max_port_sharing_level DROP DEFAULT,
	ALTER COLUMN max_port_sharing_level TYPE new_app_sharing_level USING (max_port_sharing_level::text::new_app_sharing_level),
	ALTER COLUMN max_port_sharing_level SET DEFAULT 'owner'::new_app_sharing_level;

-- Drop old enum and rename new one
DROP TYPE app_sharing_level;
ALTER TYPE new_app_sharing_level RENAME TO app_sharing_level;

-- Recreate the template_with_names view
CREATE VIEW template_with_names AS
	SELECT templates.id,
		templates.created_at,
		templates.updated_at,
		templates.organization_id,
		templates.deleted,
		templates.name,
		templates.provisioner,
		templates.active_version_id,
		templates.description,
		templates.default_ttl,
		templates.created_by,
		templates.icon,
		templates.user_acl,
		templates.group_acl,
		templates.display_name,
		templates.allow_user_cancel_workspace_jobs,
		templates.allow_user_autostart,
		templates.allow_user_autostop,
		templates.failure_ttl,
		templates.time_til_dormant,
		templates.time_til_dormant_autodelete,
		templates.autostop_requirement_days_of_week,
		templates.autostop_requirement_weeks,
		templates.autostart_block_days_of_week,
		templates.require_active_version,
		templates.deprecated,
		templates.activity_bump,
		templates.max_port_sharing_level,
		templates.use_classic_parameter_flow,
		COALESCE(visible_users.avatar_url, ''::text) AS created_by_avatar_url,
		COALESCE(visible_users.username, ''::text) AS created_by_username,
		COALESCE(visible_users.name, ''::text) AS created_by_name,
		COALESCE(organizations.name, ''::text) AS organization_name,
		COALESCE(organizations.display_name, ''::text) AS organization_display_name,
		COALESCE(organizations.icon, ''::text) AS organization_icon
	FROM ((templates
	  LEFT JOIN visible_users ON ((templates.created_by = visible_users.id)))
	  LEFT JOIN organizations ON ((templates.organization_id = organizations.id)));

COMMENT ON VIEW template_with_names IS 'Joins in the display name information such as username, avatar, and organization name.';
