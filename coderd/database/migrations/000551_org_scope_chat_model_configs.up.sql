-- Chat model configurations become organization-owned. Preserve every
-- historical config by cloning it once for every organization, then rewrite
-- live chat references to the clone in the chat's organization. Deleted
-- organizations may retain chats, so their references must remain valid.
ALTER TABLE chat_model_configs
    ADD COLUMN organization_id uuid REFERENCES organizations(id) ON DELETE CASCADE,
    ADD COLUMN user_acl jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN group_acl jsonb NOT NULL DEFAULT '{}'::jsonb;

DROP INDEX idx_chat_model_configs_single_default;

CREATE TEMP TABLE chat_model_config_org_map (
    legacy_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    model_config_id uuid NOT NULL,
    PRIMARY KEY (legacy_id, organization_id),
    UNIQUE (model_config_id)
) ON COMMIT DROP;

INSERT INTO chat_model_config_org_map (
    legacy_id,
    organization_id,
    model_config_id
)
SELECT
    cmc.id,
    o.id,
    gen_random_uuid()
FROM chat_model_configs cmc
CROSS JOIN organizations o;

INSERT INTO chat_model_configs (
    id,
    model,
    display_name,
    created_by,
    updated_by,
    enabled,
    is_default,
    deleted,
    deleted_at,
    created_at,
    updated_at,
    context_limit,
    compression_threshold,
    options,
    ai_provider_id,
    organization_id,
    user_acl,
    group_acl
)
SELECT
    map.model_config_id,
    cmc.model,
    cmc.display_name,
    cmc.created_by,
    cmc.updated_by,
    cmc.enabled,
    cmc.is_default,
    cmc.deleted,
    cmc.deleted_at,
    cmc.created_at,
    cmc.updated_at,
    cmc.context_limit,
    cmc.compression_threshold,
    cmc.options,
    cmc.ai_provider_id,
    map.organization_id,
    '{}'::jsonb,
    jsonb_build_object(map.organization_id::text, jsonb_build_array('read'))
FROM chat_model_configs cmc
JOIN chat_model_config_org_map map ON map.legacy_id = cmc.id
WHERE cmc.organization_id IS NULL;

UPDATE chats c
SET last_model_config_id = map.model_config_id
FROM chat_model_config_org_map map
WHERE map.legacy_id = c.last_model_config_id
    AND map.organization_id = c.organization_id;

ALTER TABLE chat_messages
    ADD COLUMN organization_id uuid;

UPDATE chat_messages cm
SET organization_id = c.organization_id
FROM chats c
WHERE c.id = cm.chat_id;

UPDATE chat_messages cm
SET model_config_id = map.model_config_id
FROM chat_model_config_org_map map
WHERE map.legacy_id = cm.model_config_id
    AND map.organization_id = cm.organization_id;

ALTER TABLE chat_queued_messages
    ADD COLUMN organization_id uuid;

UPDATE chat_queued_messages cqm
SET organization_id = c.organization_id
FROM chats c
WHERE c.id = cqm.chat_id;

UPDATE chat_queued_messages cqm
SET model_config_id = map.model_config_id
FROM chat_model_config_org_map map
WHERE map.legacy_id = cqm.model_config_id
    AND map.organization_id = cqm.organization_id;

UPDATE chat_debug_runs cdr
SET model_config_id = map.model_config_id
FROM chats c, chat_model_config_org_map map
WHERE c.id = cdr.chat_id
    AND map.legacy_id = cdr.model_config_id
    AND map.organization_id = c.organization_id;

-- Site configuration values are now namespaced by organization. Values that
-- are malformed or reference an already-removed config remain unchanged,
-- while valid model IDs point at each organization's clone.
CREATE OR REPLACE FUNCTION pg_temp.try_uuid(value text)
RETURNS uuid
LANGUAGE plpgsql
IMMUTABLE
AS $$
BEGIN
    RETURN value::uuid;
EXCEPTION WHEN invalid_text_representation THEN
    RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION pg_temp.try_model_override_id(value text)
RETURNS uuid
LANGUAGE plpgsql
IMMUTABLE
AS $$
BEGIN
    RETURN split_part(value, ':', 1)::uuid;
EXCEPTION WHEN invalid_text_representation THEN
    RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION pg_temp.try_jsonb(value text)
RETURNS jsonb
LANGUAGE plpgsql
IMMUTABLE
AS $$
BEGIN
    RETURN value::jsonb;
EXCEPTION WHEN invalid_text_representation THEN
    RETURN NULL;
END;
$$;

WITH model_override_keys(key) AS (
    VALUES
        ('agents_chat_explore_model_override'),
        ('agents_chat_general_model_override'),
        ('agents_chat_title_generation_model_override'),
        ('agents_chat_compaction_model_override')
)
INSERT INTO site_configs (key, value)
SELECT
    o.id::text || ':' || model_override_keys.key,
    CASE
        WHEN map.model_config_id IS NULL THEN sc.value
        ELSE map.model_config_id::text || substring(sc.value FROM 37)
    END
FROM model_override_keys
JOIN site_configs sc ON sc.key = model_override_keys.key
CROSS JOIN organizations o
LEFT JOIN chat_model_config_org_map map
    ON map.legacy_id = pg_temp.try_model_override_id(sc.value)
    AND map.organization_id = o.id
WHERE o.deleted = false;

DELETE FROM site_configs
WHERE key IN (
    'agents_chat_explore_model_override',
    'agents_chat_general_model_override',
    'agents_chat_title_generation_model_override',
    'agents_chat_compaction_model_override'
);

WITH advisor_config AS (
    SELECT
        sc.value,
        pg_temp.try_jsonb(sc.value) AS json_value
    FROM site_configs sc
    WHERE sc.key = 'agents_advisor_config'
)
INSERT INTO site_configs (key, value)
SELECT
    o.id::text || ':agents_advisor_config',
    CASE
        WHEN advisor_config.json_value IS NULL THEN advisor_config.value
        WHEN map.model_config_id IS NULL THEN advisor_config.json_value::text
        ELSE jsonb_set(
            advisor_config.json_value,
            '{model_config_id}',
            to_jsonb(map.model_config_id::text),
            true
        )::text
    END
FROM advisor_config
CROSS JOIN organizations o
LEFT JOIN chat_model_config_org_map map
    ON map.legacy_id = pg_temp.try_uuid(advisor_config.json_value ->> 'model_config_id')
    AND map.organization_id = o.id
WHERE o.deleted = false;

DELETE FROM site_configs WHERE key = 'agents_advisor_config';

-- Personal settings are duplicated only for organizations in which the user
-- is currently a member. The old keys are removed so callers cannot fall
-- back to deployment-wide model IDs.
WITH active_memberships AS (
    SELECT om.user_id, om.organization_id
    FROM organization_members om
    JOIN organizations o ON o.id = om.organization_id
    WHERE o.deleted = false
),
personal_overrides AS (
    SELECT
        uc.user_id,
        substring(uc.key FROM length('chat_personal_model_override:') + 1) AS context,
        uc.value
    FROM user_configs uc
    WHERE uc.key ~ '^chat_personal_model_override:[^:]+$'
)
INSERT INTO user_configs (user_id, key, value)
SELECT
    po.user_id,
    'chat_personal_model_override:' || am.organization_id::text || ':' || po.context,
    CASE
        WHEN map.model_config_id IS NULL THEN po.value
        ELSE 'model:' || map.model_config_id::text || substring(po.value FROM 43)
    END
FROM personal_overrides po
JOIN active_memberships am ON am.user_id = po.user_id
LEFT JOIN chat_model_config_org_map map
    ON map.legacy_id = pg_temp.try_uuid(substring(po.value FROM 7 FOR 36))
    AND map.organization_id = am.organization_id
ON CONFLICT ON CONSTRAINT user_configs_pkey DO UPDATE SET value = EXCLUDED.value;

WITH active_memberships AS (
    SELECT om.user_id, om.organization_id
    FROM organization_members om
    JOIN organizations o ON o.id = om.organization_id
    WHERE o.deleted = false
),
compaction_thresholds AS (
    SELECT
        uc.user_id,
        substring(uc.key FROM length('chat_compaction_threshold_pct:') + 1) AS legacy_model_config_id,
        uc.value
    FROM user_configs uc
    WHERE uc.key ~ '^chat_compaction_threshold_pct:[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
)
INSERT INTO user_configs (user_id, key, value)
SELECT
    ct.user_id,
    'chat_compaction_threshold_pct:' || am.organization_id::text || ':' ||
        COALESCE(map.model_config_id::text, ct.legacy_model_config_id),
    ct.value
FROM compaction_thresholds ct
JOIN active_memberships am ON am.user_id = ct.user_id
LEFT JOIN chat_model_config_org_map map
    ON map.legacy_id = pg_temp.try_uuid(ct.legacy_model_config_id)
    AND map.organization_id = am.organization_id
ON CONFLICT ON CONSTRAINT user_configs_pkey DO UPDATE SET value = EXCLUDED.value;

DELETE FROM user_configs
WHERE key ~ '^chat_personal_model_override:[^:]+$'
    OR key ~ '^chat_compaction_threshold_pct:[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';

DELETE FROM chat_model_configs WHERE organization_id IS NULL;

ALTER TABLE chat_model_configs
    ALTER COLUMN organization_id SET NOT NULL;

ALTER TABLE chat_messages
    ALTER COLUMN organization_id SET NOT NULL;

ALTER TABLE chat_queued_messages
    ALTER COLUMN organization_id SET NOT NULL;

ALTER TABLE chat_model_configs
    ADD CONSTRAINT chat_model_configs_id_organization_id_key UNIQUE (id, organization_id);

ALTER TABLE chats
    ADD CONSTRAINT chats_id_organization_id_key UNIQUE (id, organization_id);

ALTER TABLE chats
    DROP CONSTRAINT chats_last_model_config_id_fkey,
    ADD CONSTRAINT chats_last_model_config_id_organization_id_fkey
        FOREIGN KEY (last_model_config_id, organization_id)
        REFERENCES chat_model_configs (id, organization_id);

ALTER TABLE chat_messages
    DROP CONSTRAINT chat_messages_chat_id_fkey,
    DROP CONSTRAINT chat_messages_model_config_id_fkey,
    ADD CONSTRAINT chat_messages_chat_id_organization_id_fkey
        FOREIGN KEY (chat_id, organization_id)
        REFERENCES chats (id, organization_id)
        ON DELETE CASCADE,
    ADD CONSTRAINT chat_messages_model_config_id_organization_id_fkey
        FOREIGN KEY (model_config_id, organization_id)
        REFERENCES chat_model_configs (id, organization_id);

ALTER TABLE chat_queued_messages
    DROP CONSTRAINT chat_queued_messages_chat_id_fkey,
    ADD CONSTRAINT chat_queued_messages_chat_id_organization_id_fkey
        FOREIGN KEY (chat_id, organization_id)
        REFERENCES chats (id, organization_id)
        ON DELETE CASCADE,
    ADD CONSTRAINT chat_queued_messages_model_config_id_organization_id_fkey
        FOREIGN KEY (model_config_id, organization_id)
        REFERENCES chat_model_configs (id, organization_id);

CREATE INDEX idx_chat_model_configs_organization_id
    ON chat_model_configs (organization_id);

CREATE UNIQUE INDEX idx_chat_model_configs_single_default
    ON chat_model_configs (organization_id)
    WHERE is_default = true
        AND deleted = false;

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_model_config:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_model_config:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_model_config:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_model_config:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_model_config:share';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_model_config:*';
