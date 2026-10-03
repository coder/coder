-- Per-turn reasoning effort. The chats_expanded view must be dropped
-- and recreated so the new chats column can appear in its column list.
DROP VIEW IF EXISTS chats_expanded;

CREATE TYPE chat_reasoning_effort AS ENUM ('none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max');

ALTER TABLE chats ADD COLUMN last_reasoning_effort chat_reasoning_effort;
ALTER TABLE chat_messages ADD COLUMN reasoning_effort chat_reasoning_effort;
ALTER TABLE chat_queued_messages ADD COLUMN reasoning_effort chat_reasoning_effort;

COMMENT ON COLUMN chats.last_reasoning_effort IS 'Stores the most recent message effort once per-turn selection is wired.';
COMMENT ON COLUMN chat_messages.reasoning_effort IS 'Stores the selected effort for the turn triggered by this message.';
COMMENT ON COLUMN chat_queued_messages.reasoning_effort IS 'Stores the selected effort until the queued row is promoted.';

CREATE VIEW chats_expanded AS
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
    c.plan_mode,
    c.client_type,
    c.last_turn_summary,
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
    c.context_error
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));

-- Copy legacy per-provider effort values to top-level
-- reasoning_effort. Preserve legacy keys so the down migration can
-- restore the original options shape.
UPDATE chat_model_configs
SET options = options || jsonb_build_object(
    'reasoning_effort',
    jsonb_build_object('default', legacy.effort, 'max', legacy.effort)
)
FROM (
    SELECT
        id,
        COALESCE(
            NULLIF(lower(trim(options #>> '{provider_options,openai,reasoning_effort}')), ''),
            NULLIF(lower(trim(options #>> '{provider_options,azure,reasoning_effort}')), ''),
            NULLIF(lower(trim(options #>> '{provider_options,anthropic,effort}')), ''),
            NULLIF(lower(trim(options #>> '{provider_options,bedrock,effort}')), ''),
            NULLIF(lower(trim(options #>> '{provider_options,openaicompat,reasoning_effort}')), ''),
            NULLIF(lower(trim(options #>> '{provider_options,openrouter,reasoning,effort}')), ''),
            NULLIF(lower(trim(options #>> '{provider_options,vercel,reasoning,effort}')), '')
        ) AS effort
    FROM chat_model_configs
) legacy
WHERE chat_model_configs.id = legacy.id
  AND legacy.effort IS NOT NULL
  AND legacy.effort IN ('none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max');
