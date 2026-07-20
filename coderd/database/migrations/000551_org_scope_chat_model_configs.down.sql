-- Retain all copied model IDs and historical references. The default
-- organization supplies the restored global settings and default model. If it
-- is unavailable, use the earliest active organization.
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


CREATE TEMP TABLE chat_model_config_restore_organization (
    id uuid PRIMARY KEY
) ON COMMIT DROP;

INSERT INTO chat_model_config_restore_organization (id)
SELECT o.id
FROM organizations o
WHERE o.deleted = false
ORDER BY CASE WHEN o.is_default THEN 0 ELSE 1 END, o.created_at, o.id
LIMIT 1;

WITH model_override_keys(key) AS (
    VALUES
        ('agents_chat_explore_model_override'),
        ('agents_chat_general_model_override'),
        ('agents_chat_title_generation_model_override'),
        ('agents_chat_compaction_model_override')
)
INSERT INTO site_configs (key, value)
SELECT model_override_keys.key, sc.value
FROM model_override_keys
JOIN chat_model_config_restore_organization ro ON true
JOIN site_configs sc ON sc.key = ro.id::text || ':' || model_override_keys.key
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

DELETE FROM site_configs
WHERE key ~ '^[0-9a-fA-F-]+:agents_chat_(explore|general|title_generation|compaction)_model_override$';

WITH advisor_config AS (
    SELECT sc.value
    FROM chat_model_config_restore_organization ro
    JOIN site_configs sc ON sc.key = ro.id::text || ':agents_advisor_config'
)
INSERT INTO site_configs (key, value)
SELECT 'agents_advisor_config', advisor_config.value
FROM advisor_config
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

DELETE FROM site_configs WHERE key ~ '^[0-9a-fA-F-]+:agents_advisor_config$';

-- Restore one per-user setting. Prefer the default organization, then the
-- earliest active membership that has the requested setting.
WITH ranked_personal_overrides AS (
    SELECT
        uc.user_id,
        substring(uc.key FROM length('chat_personal_model_override:') + 38) AS context,
        uc.value,
        row_number() OVER (
            PARTITION BY uc.user_id, substring(uc.key FROM length('chat_personal_model_override:') + 38)
            ORDER BY
                CASE WHEN o.is_default THEN 0 ELSE 1 END,
                om.created_at,
                o.id
        ) AS setting_rank
    FROM user_configs uc
    JOIN organization_members om
        ON om.user_id = uc.user_id
        AND uc.key LIKE 'chat_personal_model_override:' || om.organization_id::text || ':%'
    JOIN organizations o ON o.id = om.organization_id
    WHERE o.deleted = false
)
INSERT INTO user_configs (user_id, key, value)
SELECT
    rpo.user_id,
    'chat_personal_model_override:' || rpo.context,
    rpo.value
FROM ranked_personal_overrides rpo
WHERE rpo.setting_rank = 1
ON CONFLICT ON CONSTRAINT user_configs_pkey DO UPDATE SET value = EXCLUDED.value;

WITH ranked_compaction_thresholds AS (
    SELECT
        uc.user_id,
        substring(uc.key FROM length('chat_compaction_threshold_pct:') + 38) AS model_config_id,
        uc.value,
        row_number() OVER (
            PARTITION BY uc.user_id, substring(uc.key FROM length('chat_compaction_threshold_pct:') + 38)
            ORDER BY
                CASE WHEN o.is_default THEN 0 ELSE 1 END,
                om.created_at,
                o.id
        ) AS setting_rank
    FROM user_configs uc
    JOIN organization_members om
        ON om.user_id = uc.user_id
        AND uc.key LIKE 'chat_compaction_threshold_pct:' || om.organization_id::text || ':%'
    JOIN organizations o ON o.id = om.organization_id
    WHERE o.deleted = false
)
INSERT INTO user_configs (user_id, key, value)
SELECT
    rct.user_id,
    'chat_compaction_threshold_pct:' || rct.model_config_id,
    rct.value
FROM ranked_compaction_thresholds rct
WHERE rct.setting_rank = 1
ON CONFLICT ON CONSTRAINT user_configs_pkey DO UPDATE SET value = EXCLUDED.value;

DELETE FROM user_configs
WHERE key ~ '^chat_personal_model_override:[0-9a-fA-F-]+:.+$'
    OR key ~ '^chat_compaction_threshold_pct:[0-9a-fA-F-]+:[0-9a-fA-F-]+$';

-- A global schema allows one active default. Keep the restored source
-- organization's default, clearing defaults in all other organizations.
UPDATE chat_model_configs cmc
SET is_default = false
WHERE cmc.is_default = true
    AND cmc.deleted = false
    AND NOT EXISTS (
        SELECT 1
        FROM chat_model_config_restore_organization ro
        WHERE ro.id = cmc.organization_id
    );

ALTER TABLE chats
    DROP CONSTRAINT chats_last_model_config_id_organization_id_fkey,
    ADD CONSTRAINT chats_last_model_config_id_fkey
        FOREIGN KEY (last_model_config_id) REFERENCES chat_model_configs (id);

ALTER TABLE chat_messages
    DROP CONSTRAINT chat_messages_chat_id_organization_id_fkey,
    DROP CONSTRAINT chat_messages_model_config_id_organization_id_fkey,
    ADD CONSTRAINT chat_messages_chat_id_fkey
        FOREIGN KEY (chat_id) REFERENCES chats (id) ON DELETE CASCADE,
    ADD CONSTRAINT chat_messages_model_config_id_fkey
        FOREIGN KEY (model_config_id) REFERENCES chat_model_configs (id);

ALTER TABLE chat_queued_messages
    DROP CONSTRAINT chat_queued_messages_chat_id_organization_id_fkey,
    DROP CONSTRAINT chat_queued_messages_model_config_id_organization_id_fkey,
    ADD CONSTRAINT chat_queued_messages_chat_id_fkey
        FOREIGN KEY (chat_id) REFERENCES chats (id) ON DELETE CASCADE;

ALTER TABLE chats DROP CONSTRAINT chats_id_organization_id_key;

DROP INDEX idx_chat_model_configs_organization_id;
DROP INDEX idx_chat_model_configs_single_default;

ALTER TABLE chat_model_configs
    DROP CONSTRAINT chat_model_configs_id_organization_id_key,
    DROP COLUMN organization_id,
    DROP COLUMN user_acl,
    DROP COLUMN group_acl;

ALTER TABLE chat_messages DROP COLUMN organization_id;
ALTER TABLE chat_queued_messages DROP COLUMN organization_id;

CREATE UNIQUE INDEX idx_chat_model_configs_single_default
    ON chat_model_configs ((1))
    WHERE is_default = true
        AND deleted = false;
