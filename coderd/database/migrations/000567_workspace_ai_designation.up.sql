-- The AI designation marker: when set, every workspace agent of every build
-- of this workspace is bound to this AI agent identity. Server-authoritative;
-- it survives rebuilds. See AI_AGENT_SECURITY_ARCHITECTURE.md, Vertical 2.
ALTER TABLE workspaces
	ADD COLUMN ai_agent_id uuid REFERENCES ai_agents (user_id);

CREATE INDEX workspaces_ai_agent_id_idx ON workspaces (ai_agent_id);

DROP VIEW workspaces_expanded;

CREATE VIEW workspaces_expanded AS
 SELECT workspaces.id,
    workspaces.created_at,
    workspaces.updated_at,
    workspaces.owner_id,
    workspaces.organization_id,
    workspaces.template_id,
    workspaces.deleted,
    workspaces.name,
    workspaces.autostart_schedule,
    workspaces.ttl,
    workspaces.last_used_at,
    workspaces.dormant_at,
    workspaces.deleting_at,
    workspaces.automatic_updates,
    workspaces.favorite,
    workspaces.next_start_at,
    workspaces.group_acl,
    workspaces.user_acl,
    workspaces.ai_agent_id,
    visible_users.avatar_url AS owner_avatar_url,
    visible_users.username AS owner_username,
    visible_users.name AS owner_name,
    organizations.name AS organization_name,
    organizations.display_name AS organization_display_name,
    organizations.icon AS organization_icon,
    organizations.description AS organization_description,
    templates.name AS template_name,
    templates.display_name AS template_display_name,
    templates.icon AS template_icon,
    templates.description AS template_description,
    tasks.id AS task_id,
    COALESCE(( SELECT jsonb_object_agg(acl.key, jsonb_build_object('name', COALESCE(g.name, ''::text), 'avatar_url', COALESCE(g.avatar_url, ''::text))) AS jsonb_object_agg
           FROM (jsonb_each(workspaces.group_acl) acl(key, value)
             LEFT JOIN groups g ON ((g.id = (acl.key)::uuid)))), '{}'::jsonb) AS group_acl_display_info,
    COALESCE(( SELECT jsonb_object_agg(acl.key, jsonb_build_object('name', COALESCE(vu.name, ''::text), 'avatar_url', COALESCE(vu.avatar_url, ''::text))) AS jsonb_object_agg
           FROM (jsonb_each(workspaces.user_acl) acl(key, value)
             LEFT JOIN visible_users vu ON ((vu.id = (acl.key)::uuid)))), '{}'::jsonb) AS user_acl_display_info
   FROM ((((workspaces
     JOIN visible_users ON ((workspaces.owner_id = visible_users.id)))
     JOIN organizations ON ((workspaces.organization_id = organizations.id)))
     JOIN templates ON ((workspaces.template_id = templates.id)))
     LEFT JOIN tasks ON ((workspaces.id = tasks.workspace_id)));
