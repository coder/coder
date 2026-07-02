DROP VIEW IF EXISTS chats_expanded;

-- Move the reasoning effort default back to the provider-appropriate
-- legacy path. Rows whose provider cannot be determined just lose the
-- reasoning_effort key in the cleanup below.
UPDATE chat_model_configs cmc
SET options = (cmc.options - 'reasoning_effort') || jsonb_build_object(
    'provider_options',
    COALESCE(cmc.options -> 'provider_options', '{}'::jsonb) || jsonb_build_object(
        'openai',
        COALESCE(cmc.options #> '{provider_options,openai}', '{}'::jsonb) ||
            jsonb_build_object('reasoning_effort', cmc.options #> '{reasoning_effort,default}')
    )
)
FROM ai_providers ap
WHERE ap.id = cmc.ai_provider_id
  AND ap.type IN ('openai', 'azure')
  AND cmc.options #>> '{reasoning_effort,default}' IS NOT NULL;

UPDATE chat_model_configs cmc
SET options = (cmc.options - 'reasoning_effort') || jsonb_build_object(
    'provider_options',
    COALESCE(cmc.options -> 'provider_options', '{}'::jsonb) || jsonb_build_object(
        'anthropic',
        COALESCE(cmc.options #> '{provider_options,anthropic}', '{}'::jsonb) ||
            jsonb_build_object('effort', cmc.options #> '{reasoning_effort,default}')
    )
)
FROM ai_providers ap
WHERE ap.id = cmc.ai_provider_id
  AND ap.type IN ('anthropic', 'bedrock')
  AND cmc.options #>> '{reasoning_effort,default}' IS NOT NULL;

UPDATE chat_model_configs cmc
SET options = (cmc.options - 'reasoning_effort') || jsonb_build_object(
    'provider_options',
    COALESCE(cmc.options -> 'provider_options', '{}'::jsonb) || jsonb_build_object(
        'openaicompat',
        COALESCE(cmc.options #> '{provider_options,openaicompat}', '{}'::jsonb) ||
            jsonb_build_object('reasoning_effort', cmc.options #> '{reasoning_effort,default}')
    )
)
FROM ai_providers ap
WHERE ap.id = cmc.ai_provider_id
  AND ap.type = 'openai-compat'
  AND cmc.options #>> '{reasoning_effort,default}' IS NOT NULL;

UPDATE chat_model_configs cmc
SET options = (cmc.options - 'reasoning_effort') || jsonb_build_object(
    'provider_options',
    COALESCE(cmc.options -> 'provider_options', '{}'::jsonb) || jsonb_build_object(
        'openrouter',
        COALESCE(cmc.options #> '{provider_options,openrouter}', '{}'::jsonb) || jsonb_build_object(
            'reasoning',
            COALESCE(cmc.options #> '{provider_options,openrouter,reasoning}', '{}'::jsonb) ||
                jsonb_build_object('effort', cmc.options #> '{reasoning_effort,default}')
        )
    )
)
FROM ai_providers ap
WHERE ap.id = cmc.ai_provider_id
  AND ap.type = 'openrouter'
  AND cmc.options #>> '{reasoning_effort,default}' IS NOT NULL;

UPDATE chat_model_configs cmc
SET options = (cmc.options - 'reasoning_effort') || jsonb_build_object(
    'provider_options',
    COALESCE(cmc.options -> 'provider_options', '{}'::jsonb) || jsonb_build_object(
        'vercel',
        COALESCE(cmc.options #> '{provider_options,vercel}', '{}'::jsonb) || jsonb_build_object(
            'reasoning',
            COALESCE(cmc.options #> '{provider_options,vercel,reasoning}', '{}'::jsonb) ||
                jsonb_build_object('effort', cmc.options #> '{reasoning_effort,default}')
        )
    )
)
FROM ai_providers ap
WHERE ap.id = cmc.ai_provider_id
  AND ap.type = 'vercel'
  AND cmc.options #>> '{reasoning_effort,default}' IS NOT NULL;

UPDATE chat_model_configs
SET options = options - 'reasoning_effort'
WHERE options ? 'reasoning_effort';

ALTER TABLE chats DROP COLUMN last_reasoning_effort;
ALTER TABLE chat_messages DROP COLUMN reasoning_effort;
ALTER TABLE chat_queued_messages DROP COLUMN reasoning_effort;

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
