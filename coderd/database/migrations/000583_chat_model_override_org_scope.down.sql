INSERT INTO site_configs (key, value)
SELECT
    'agents_chat_' || omo.context || '_model_override',
    omo.model_config_id::text || CASE
        WHEN omo.reasoning_effort IS NOT NULL AND omo.reasoning_effort != ''
            THEN ':' || omo.reasoning_effort
        ELSE ''
    END
FROM chat_organization_model_overrides omo
JOIN organizations o ON o.id = omo.organization_id
WHERE o.is_default
  AND omo.context IN ('general', 'explore', 'title_generation', 'compaction')
  AND EXISTS (
      SELECT 1
      FROM chat_model_configs cmc
      WHERE cmc.id = omo.model_config_id
        AND cmc.organization_id = omo.organization_id
        AND NOT cmc.deleted
  )
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

-- The stored advisor config is unrestricted text, so guard the jsonb parse of
-- the existing row with an exception handler; a malformed value is replaced
-- rather than aborting the rollback.
DO $$
DECLARE
    fields jsonb;
    existing jsonb;
BEGIN
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'model_config_id', omo.model_config_id,
        'reasoning_effort', omo.reasoning_effort
    ))
    INTO fields
    FROM chat_organization_model_overrides omo
    JOIN organizations o ON o.id = omo.organization_id
    WHERE o.is_default
      AND omo.context = 'advisor'
      AND EXISTS (
          SELECT 1
          FROM chat_model_configs cmc
          WHERE cmc.id = omo.model_config_id
            AND cmc.organization_id = omo.organization_id
            AND NOT cmc.deleted
      );
    IF fields IS NULL THEN
        RETURN;
    END IF;
    BEGIN
        SELECT NULLIF(value, '')::jsonb INTO existing
        FROM site_configs
        WHERE key = 'agents_advisor_config';
    EXCEPTION WHEN others THEN
        existing := NULL;
    END;
    IF existing IS NULL OR jsonb_typeof(existing) != 'object' THEN
        existing := '{}'::jsonb;
    END IF;
    INSERT INTO site_configs (key, value)
    VALUES ('agents_advisor_config', (existing || fields)::text)
    ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;
END;
$$;

INSERT INTO user_configs (user_id, key, value)
SELECT
    umo.user_id,
    'chat_personal_model_override:' || umo.context,
    CASE
        WHEN umo.mode IN ('chat_default', 'deployment_default') THEN umo.mode
        ELSE 'model:' || umo.model_config_id::text || CASE
            WHEN umo.reasoning_effort IS NOT NULL AND umo.reasoning_effort != ''
                THEN ':' || umo.reasoning_effort
            ELSE ''
        END
    END
FROM chat_user_model_overrides umo
JOIN organizations o ON o.id = umo.organization_id
WHERE o.is_default
  AND (
      umo.mode IN ('chat_default', 'deployment_default')
      OR EXISTS (
          SELECT 1
          FROM chat_model_configs cmc
          WHERE cmc.id = umo.model_config_id
            AND cmc.organization_id = umo.organization_id
            AND NOT cmc.deleted
      )
  )
ON CONFLICT ON CONSTRAINT user_configs_pkey
DO UPDATE SET value = EXCLUDED.value;

DROP TABLE chat_user_model_overrides;
DROP TABLE chat_organization_model_overrides;

ALTER TABLE chat_model_configs
    DROP CONSTRAINT chat_model_configs_organization_id_id_key;
