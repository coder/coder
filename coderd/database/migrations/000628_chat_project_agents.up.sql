ALTER TABLE chat_projects
    ADD COLUMN invokable_agent_ids uuid[] NOT NULL DEFAULT '{}'::uuid[],
    ADD COLUMN workspace_tools_enabled boolean NOT NULL DEFAULT true,
    ADD COLUMN allowed_template_ids uuid[] NOT NULL DEFAULT '{}'::uuid[],
    ADD COLUMN mcp_server_ids uuid[] NOT NULL DEFAULT '{}'::uuid[],
    ADD COLUMN external_auth_provider_ids text[] NOT NULL DEFAULT '{}'::text[],
    ADD COLUMN template_parameter_rules jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN agent_user_id uuid REFERENCES users(id) ON DELETE RESTRICT,
    ADD COLUMN model_config_id uuid REFERENCES chat_model_configs(id) ON DELETE SET NULL,
    ADD CONSTRAINT chat_projects_invokable_agents_not_self CHECK (NOT (id = ANY (invokable_agent_ids)));

COMMENT ON COLUMN chat_projects.invokable_agent_ids IS 'Other agents (chat projects) that chats in this agent may invoke to delegate work.';
COMMENT ON COLUMN chat_projects.workspace_tools_enabled IS 'Whether chats in this agent may create, start, and use workspaces.';
COMMENT ON COLUMN chat_projects.allowed_template_ids IS 'Templates chats in this project may create workspaces from. Empty allows every template the chat owner can use.';
COMMENT ON COLUMN chat_projects.mcp_server_ids IS 'MCP server configs attached to every chat created in this project.';
COMMENT ON COLUMN chat_projects.template_parameter_rules IS 'Per-template parameter value restrictions for workspaces created by chats in this project. Each entry has template_id, parameter_name, and allowed_values; the first allowed value is the default.';
COMMENT ON COLUMN chat_projects.external_auth_provider_ids IS 'External auth providers workspaces in this project may require. Empty allows every provider.';

COMMENT ON COLUMN chat_projects.agent_user_id IS 'Agent user the agent acts as. Chats in the project are owned by this user. NULL until provisioned.';

COMMENT ON COLUMN chat_projects.model_config_id IS 'Model new chats in the agent use when the chat does not pick one. NULL uses the deployment default. A soft-deleted model keeps its ID here and chats fall back to the default.';

CREATE UNIQUE INDEX idx_chat_projects_agent_user_id ON chat_projects (agent_user_id) WHERE agent_user_id IS NOT NULL;

CREATE TABLE chat_project_triggers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES chat_projects(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name text NOT NULL,
    prompt text NOT NULL DEFAULT '',
    hashed_secret bytea NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    last_triggered_at timestamptz,
    source text NOT NULL DEFAULT 'webhook',
    events text[] NOT NULL DEFAULT '{}'::text[],
    repository text NOT NULL DEFAULT '',
    channel text NOT NULL DEFAULT '',
    text_pattern text NOT NULL DEFAULT '',
    text_pattern_is_regex boolean NOT NULL DEFAULT false,
    ignore_thread_replies boolean NOT NULL DEFAULT false,
    senders text[] NOT NULL DEFAULT '{}'::text[],
    continue_conversations boolean NOT NULL DEFAULT false,
    busy_behavior text NOT NULL DEFAULT 'queue',
    slack_signing_secret text NOT NULL DEFAULT '',
    slack_signing_secret_key_id text REFERENCES dbcrypt_keys(active_key_digest),
    github_webhook_secret text NOT NULL DEFAULT '',
    github_webhook_secret_key_id text REFERENCES dbcrypt_keys(active_key_digest),
    schedule text NOT NULL DEFAULT '',
    next_run_at timestamptz,
    CONSTRAINT chat_project_triggers_source CHECK (source IN ('webhook', 'github', 'slack', 'schedule')),
    CONSTRAINT chat_project_triggers_busy_behavior CHECK (busy_behavior IN ('queue', 'interrupt')),
    CONSTRAINT chat_project_triggers_name_not_blank CHECK (length(trim(name)) > 0),
    CONSTRAINT chat_project_triggers_name_length CHECK (length(name) <= 64),
    CONSTRAINT chat_project_triggers_prompt_length CHECK (length(prompt) <= 8192)
);

COMMENT ON TABLE chat_project_triggers IS 'Triggers that start a chat in a project when their webhook is called or their schedule comes due.';
COMMENT ON COLUMN chat_project_triggers.hashed_secret IS 'SHA-256 of the secret embedded in the webhook URL. Random for schedule triggers, which have no webhook URL.';
COMMENT ON COLUMN chat_project_triggers.source IS 'What starts the trigger: webhook (any body), github (GitHub webhooks), slack (Slack Events API and slash commands), or schedule (a cron schedule).';
COMMENT ON COLUMN chat_project_triggers.schedule IS 'Cron expression with five fields and an optional CRON_TZ= prefix for schedule triggers. Empty for other sources.';
COMMENT ON COLUMN chat_project_triggers.next_run_at IS 'When a schedule trigger runs next. NULL when the trigger is disabled or is not a schedule trigger.';
COMMENT ON COLUMN chat_project_triggers.events IS 'Events that start a chat, such as pull_request.opened for GitHub or app_mention for Slack. Empty matches every event.';
COMMENT ON COLUMN chat_project_triggers.repository IS 'GitHub repository (owner/name) the trigger is limited to. Empty matches every repository.';
COMMENT ON COLUMN chat_project_triggers.channel IS 'Slack channel ID the trigger is limited to. Empty matches every channel.';
COMMENT ON COLUMN chat_project_triggers.text_pattern IS 'Text the event must contain, such as a Slack message or GitHub comment. Empty matches every event.';
COMMENT ON COLUMN chat_project_triggers.text_pattern_is_regex IS 'Whether text_pattern is a regular expression instead of case-insensitive text.';
COMMENT ON COLUMN chat_project_triggers.ignore_thread_replies IS 'Whether Slack replies in threads are ignored.';
COMMENT ON COLUMN chat_project_triggers.senders IS 'Slack user IDs or GitHub logins whose events start chats. Empty matches everyone.';
COMMENT ON COLUMN chat_project_triggers.continue_conversations IS 'Whether events in a conversation the trigger already started a chat for, such as a Slack thread or a GitHub issue, are sent to that chat instead of starting a new one.';
COMMENT ON COLUMN chat_project_triggers.busy_behavior IS 'What a continued chat does with a new event while it is still working: queue waits for the current turn, interrupt stops it.';
COMMENT ON COLUMN chat_project_triggers.slack_signing_secret IS 'Slack app signing secret used to verify X-Slack-Signature. Empty skips the check. Encrypted with dbcrypt when enabled.';
COMMENT ON COLUMN chat_project_triggers.github_webhook_secret IS 'GitHub webhook secret used to verify X-Hub-Signature-256. Empty skips the check. Encrypted with dbcrypt when enabled.';

CREATE INDEX idx_chat_project_triggers_project_id ON chat_project_triggers (project_id);

CREATE INDEX idx_chat_project_triggers_next_run_at ON chat_project_triggers (next_run_at) WHERE source = 'schedule' AND enabled;

CREATE TABLE chat_project_trigger_deliveries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trigger_id uuid NOT NULL REFERENCES chat_project_triggers(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    status text NOT NULL,
    event text NOT NULL DEFAULT '',
    message text NOT NULL DEFAULT '',
    chat_id uuid REFERENCES chats(id) ON DELETE SET NULL,
    CONSTRAINT chat_project_trigger_deliveries_status CHECK (status IN ('started', 'continued', 'ignored', 'rejected', 'handshake'))
);

COMMENT ON TABLE chat_project_trigger_deliveries IS 'Recent authenticated webhook calls to a trigger and what each one did, so people can see why an event did or did not start a chat.';
COMMENT ON COLUMN chat_project_trigger_deliveries.status IS 'started: a chat started. continued: the event was sent to the chat of its conversation. ignored: the event did not match the trigger. rejected: the call failed. handshake: a GitHub ping or Slack URL verification was answered.';
COMMENT ON COLUMN chat_project_trigger_deliveries.event IS 'Event name, such as issue_comment.created or app_mention. Empty for plain webhooks.';
COMMENT ON COLUMN chat_project_trigger_deliveries.message IS 'Why the event was ignored or rejected.';

CREATE INDEX idx_chat_project_trigger_deliveries_trigger_id ON chat_project_trigger_deliveries (trigger_id, created_at DESC);

CREATE TABLE chat_project_trigger_conversations (
    trigger_id uuid NOT NULL REFERENCES chat_project_triggers(id) ON DELETE CASCADE,
    conversation_key text NOT NULL,
    chat_id uuid NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (trigger_id, conversation_key)
);

COMMENT ON TABLE chat_project_trigger_conversations IS 'The chat a trigger started for each external conversation, so later events in the conversation continue it.';
COMMENT ON COLUMN chat_project_trigger_conversations.conversation_key IS 'Source-specific conversation ID: team/channel/thread_ts for Slack, owner/repo#number for GitHub, or the X-Coder-Conversation-Id header for webhooks.';

CREATE INDEX idx_chat_project_trigger_conversations_chat_id ON chat_project_trigger_conversations (chat_id);

CREATE TABLE chat_project_trigger_events (
    trigger_id uuid NOT NULL REFERENCES chat_project_triggers(id) ON DELETE CASCADE,
    event_key text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (trigger_id, event_key)
);

COMMENT ON TABLE chat_project_trigger_events IS 'Recently handled event IDs per trigger, so retried or duplicate deliveries, such as Slack retries, are handled once.';

ALTER TABLE mcp_server_configs
    ADD COLUMN api_key_prefix text NOT NULL DEFAULT '',
    DROP CONSTRAINT mcp_server_configs_auth_type_check,
    ADD CONSTRAINT mcp_server_configs_auth_type_check
        CHECK (auth_type IN ('none', 'oauth2', 'api_key', 'custom_headers', 'user_oidc', 'user_api_key'));

COMMENT ON COLUMN mcp_server_configs.api_key_prefix IS 'For user_api_key auth, text sent before each user''s token in api_key_header, such as "Bearer ".';

CREATE TABLE mcp_server_github_app_credentials (
    mcp_server_config_id uuid NOT NULL REFERENCES mcp_server_configs(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    app_id bigint NOT NULL CHECK (app_id > 0),
    installation_id bigint NOT NULL CHECK (installation_id > 0),
    private_key text NOT NULL,
    private_key_key_id text REFERENCES dbcrypt_keys(active_key_digest),
    repositories text[] NOT NULL DEFAULT '{}'::text[],
    use_for_git boolean NOT NULL DEFAULT false,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (mcp_server_config_id, user_id)
);

CREATE INDEX idx_mcp_server_github_app_credentials_user_id ON mcp_server_github_app_credentials USING btree (user_id);

COMMENT ON TABLE mcp_server_github_app_credentials IS 'GitHub App installations an agent''s user acts as on user_api_key MCP servers, instead of a pasted token. Coder mints short-lived installation tokens from them.';

COMMENT ON COLUMN mcp_server_github_app_credentials.private_key IS 'The GitHub App''s PEM private key, encrypted when database encryption is enabled. Never returned by the API.';

COMMENT ON COLUMN mcp_server_github_app_credentials.repositories IS 'Repository names minted tokens are limited to. Empty means every repository of the installation.';

COMMENT ON COLUMN mcp_server_github_app_credentials.use_for_git IS 'Whether the agent''s workspaces get installation tokens for git through external auth. Tokens cover the listed repositories, or the whole installation when none are listed.';

-- Agent users are a distinct kind of user, like service accounts: they have
-- no email and cannot sign in.
ALTER TABLE users
    ADD COLUMN is_agent boolean NOT NULL DEFAULT false,
    DROP CONSTRAINT users_email_not_empty,
    ADD CONSTRAINT users_email_not_empty CHECK ((is_service_account = true OR is_agent = true) = (email = '')),
    ADD CONSTRAINT users_agent_login_type CHECK (is_agent = false OR login_type = 'none'),
    ADD CONSTRAINT users_agent_not_service_account CHECK (NOT (is_agent AND is_service_account));

COMMENT ON COLUMN users.is_agent IS 'Determines if a user is the identity of a Coder Agent (chat project). Agent users cannot login.';

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
    users.is_agent AS user_is_agent,
    groups.organization_id,
    groups.name AS group_name,
    all_members.group_id
   FROM ((all_members
     JOIN users ON ((users.id = all_members.user_id)))
     JOIN groups ON ((groups.id = all_members.group_id)))
  WHERE (users.deleted = false);

-- A delegated chat can be owned by the invoked agent's user rather than the
-- root chat's owner. Child chats inherit sharing from their root chat, so
-- also give the root chat's owner read and write access in that case.
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
    COALESCE(
        CASE
            WHEN root.owner_id <> c.owner_id THEN root.user_acl || jsonb_build_object(root.owner_id::text, '{"permissions": ["read", "update"]}'::jsonb)
        END,
        root.user_acl,
        c.user_acl
    ) AS user_acl,
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
