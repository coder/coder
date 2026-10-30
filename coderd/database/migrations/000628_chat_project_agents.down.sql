CREATE OR REPLACE VIEW chats_expanded AS
 SELECT c.id,
    c.owner_id,
    c.workspace_id,
    c.title,
    c.status,
    c.worker_id,
    c.started_at,
    c.heartbeat_at,
    c.created_at,
    c.updated_at,
    c.parent_chat_id,
    c.root_chat_id,
    c.last_model_config_id,
    c.last_reasoning_effort,
    c.archived,
    c.last_error,
    c.mode,
    c.mcp_server_ids,
    c.labels,
    c.build_id,
    c.agent_id,
    c.pin_order,
    c.last_read_message_id,
    c.dynamic_tools,
    c.organization_id,
    c.project_id,
    c.plan_mode,
    c.client_type,
    c.last_turn_summary,
    c.summary,
    c.summary_generated_at,
    c.snapshot_version,
    c.history_version,
    c.queue_version,
    c.generation_attempt,
    c.retry_state,
    c.retry_state_version,
    c.runner_id,
    c.requires_action_deadline_at,
    COALESCE(root.user_acl, c.user_acl) AS user_acl,
    COALESCE(root.group_acl, c.group_acl) AS group_acl,
    owner.username AS owner_username,
    owner.name AS owner_name,
    c.context_aggregate_hash,
    c.context_dirty_since,
    c.context_dirty_resources,
    c.context_error,
    c.compaction_requested_at,
    c.title_source,
    c.title_updated_at
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));

DROP VIEW group_members_expanded;

CREATE VIEW group_members_expanded AS
 WITH all_members AS (
         SELECT group_members.user_id,
            group_members.group_id
           FROM group_members
        UNION
         SELECT organization_members.user_id,
            organization_members.organization_id AS group_id
           FROM organization_members
        )
 SELECT users.id AS user_id,
    users.email AS user_email,
    users.username AS user_username,
    users.hashed_password AS user_hashed_password,
    users.created_at AS user_created_at,
    users.updated_at AS user_updated_at,
    users.status AS user_status,
    users.rbac_roles AS user_rbac_roles,
    users.login_type AS user_login_type,
    users.avatar_url AS user_avatar_url,
    users.deleted AS user_deleted,
    users.last_seen_at AS user_last_seen_at,
    users.quiet_hours_schedule AS user_quiet_hours_schedule,
    users.name AS user_name,
    users.github_com_user_id AS user_github_com_user_id,
    users.is_system AS user_is_system,
    users.is_service_account as user_is_service_account,
    groups.organization_id,
    groups.name AS group_name,
    all_members.group_id
   FROM ((all_members
     JOIN users ON ((users.id = all_members.user_id)))
     JOIN groups ON ((groups.id = all_members.group_id)))
  WHERE (users.deleted = false);

-- Agent users have no email, so keep them valid as service accounts.
UPDATE users SET is_service_account = true WHERE is_agent = true;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_agent_not_service_account,
    DROP CONSTRAINT IF EXISTS users_agent_login_type,
    DROP CONSTRAINT users_email_not_empty,
    ADD CONSTRAINT users_email_not_empty CHECK ((is_service_account = true) = (email = '')),
    DROP COLUMN is_agent;

DROP TABLE IF EXISTS mcp_server_github_app_credentials;

-- Rows using user_api_key would violate the restored CHECK constraint.
DELETE FROM mcp_server_configs WHERE auth_type = 'user_api_key';

ALTER TABLE mcp_server_configs
    DROP CONSTRAINT mcp_server_configs_auth_type_check,
    ADD CONSTRAINT mcp_server_configs_auth_type_check
        CHECK (auth_type IN ('none', 'oauth2', 'api_key', 'custom_headers', 'user_oidc')),
    DROP COLUMN IF EXISTS api_key_prefix;

DROP TABLE IF EXISTS chat_project_trigger_events;

DROP TABLE IF EXISTS chat_project_trigger_conversations;

DROP TABLE IF EXISTS chat_project_trigger_deliveries;

DROP TABLE IF EXISTS chat_project_triggers;

DROP INDEX IF EXISTS idx_chat_projects_agent_user_id;

ALTER TABLE chat_projects
    DROP CONSTRAINT IF EXISTS chat_projects_invokable_agents_not_self,
    DROP COLUMN IF EXISTS model_config_id,
    DROP COLUMN IF EXISTS agent_user_id,
    DROP COLUMN IF EXISTS template_parameter_rules,
    DROP COLUMN IF EXISTS external_auth_provider_ids,
    DROP COLUMN IF EXISTS mcp_server_ids,
    DROP COLUMN IF EXISTS allowed_template_ids,
    DROP COLUMN IF EXISTS workspace_tools_enabled,
    DROP COLUMN IF EXISTS invokable_agent_ids;
