CREATE TYPE cors_behavior AS ENUM (
    'simple',
    'passthru'
);

ALTER TABLE templates
ADD COLUMN cors_behavior cors_behavior NOT NULL DEFAULT 'simple'::cors_behavior;

-- Update the template_with_users view by recreating it.
DROP VIEW IF EXISTS template_with_names;
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
    templates.cors_behavior,          -- <--- adding this column
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
