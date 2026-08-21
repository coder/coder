ALTER TABLE chat_model_configs
    ADD CONSTRAINT chat_model_configs_organization_id_id_key
    UNIQUE (organization_id, id);

CREATE TABLE chat_organization_model_overrides (
    id uuid NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    context text NOT NULL,
    model_config_id uuid NOT NULL,
    reasoning_effort text,
    CONSTRAINT chat_organization_model_overrides_context_check
        CHECK (context IN ('general', 'explore', 'title_generation', 'compaction', 'advisor')),
    CONSTRAINT chat_organization_model_overrides_organization_id_context_key
        UNIQUE (organization_id, context),
    CONSTRAINT chat_organization_model_overrides_organization_model_config_fkey
        FOREIGN KEY (organization_id, model_config_id)
        REFERENCES chat_model_configs (organization_id, id)
);

CREATE TABLE chat_user_model_overrides (
    id uuid NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    context text NOT NULL,
    mode text NOT NULL,
    model_config_id uuid,
    reasoning_effort text,
    CONSTRAINT chat_user_model_overrides_context_check
        CHECK (context IN ('root', 'general', 'explore')),
    CONSTRAINT chat_user_model_overrides_mode_check
        CHECK (mode IN ('model', 'chat_default', 'deployment_default')),
    CONSTRAINT chat_user_model_overrides_model_requires_config_check
        CHECK ((mode = 'model') = (model_config_id IS NOT NULL)),
    CONSTRAINT chat_user_model_overrides_user_organization_context_key
        UNIQUE (user_id, organization_id, context),
    CONSTRAINT chat_user_model_overrides_organization_model_config_fkey
        FOREIGN KEY (organization_id, model_config_id)
        REFERENCES chat_model_configs (organization_id, id)
);

WITH parsed AS (
    SELECT
        substring(sc.key FROM '^agents_chat_(.*)_model_override$') AS context,
        sc.value,
        (
            SELECT split_part(trim(sc.value), ':', 1)::uuid
            WHERE split_part(trim(sc.value), ':', 1)
                ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
        ) AS config_id
    FROM site_configs sc
    WHERE sc.key IN (
        'agents_chat_general_model_override',
        'agents_chat_explore_model_override',
        'agents_chat_title_generation_model_override',
        'agents_chat_compaction_model_override'
    )
      AND trim(sc.value) != ''
)
INSERT INTO chat_organization_model_overrides
    (organization_id, context, model_config_id, reasoning_effort)
SELECT
    o.id,
    p.context,
    p.config_id,
    NULLIF(split_part(trim(p.value), ':', 2), '')
FROM parsed p
CROSS JOIN organizations o
WHERE o.is_default
  AND p.config_id IS NOT NULL
  AND EXISTS (
      SELECT 1
      FROM chat_model_configs cmc
      WHERE cmc.id = p.config_id
        AND cmc.organization_id = o.id
        AND NOT cmc.deleted
  )
ON CONFLICT ON CONSTRAINT chat_organization_model_overrides_organization_id_context_key
DO NOTHING;

DELETE FROM site_configs
WHERE key IN (
    'agents_chat_general_model_override',
    'agents_chat_explore_model_override',
    'agents_chat_title_generation_model_override',
    'agents_chat_compaction_model_override'
);

-- The advisor config is stored as unrestricted text, so a malformed value is
-- representable and a bare ::jsonb cast would abort the migration. Parse it
-- inside an exception handler (pg_input_is_valid needs PostgreSQL 16) and
-- leave a malformed row untouched.
DO $$
DECLARE
    raw text;
    blob jsonb;
    config_id uuid;
BEGIN
    SELECT value INTO raw FROM site_configs WHERE key = 'agents_advisor_config';
    IF raw IS NULL THEN
        RETURN;
    END IF;
    BEGIN
        blob := raw::jsonb;
    EXCEPTION WHEN others THEN
        RETURN;
    END;
    IF jsonb_typeof(blob) != 'object' THEN
        RETURN;
    END IF;

    IF blob ->> 'model_config_id'
        ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$' THEN
        config_id := (blob ->> 'model_config_id')::uuid;
        INSERT INTO chat_organization_model_overrides
            (organization_id, context, model_config_id, reasoning_effort)
        SELECT
            o.id,
            'advisor',
            config_id,
            NULLIF(trim(blob ->> 'reasoning_effort'), '')
        FROM organizations o
        WHERE o.is_default
          AND config_id != '00000000-0000-0000-0000-000000000000'::uuid
          AND EXISTS (
              SELECT 1
              FROM chat_model_configs cmc
              WHERE cmc.id = config_id
                AND cmc.organization_id = o.id
                AND NOT cmc.deleted
          )
        ON CONFLICT ON CONSTRAINT chat_organization_model_overrides_organization_id_context_key
        DO NOTHING;
    END IF;

    UPDATE site_configs
    SET value = (blob - 'model_config_id' - 'reasoning_effort')::text
    WHERE key = 'agents_advisor_config';
END;
$$;

WITH parsed AS (
    SELECT
        uc.user_id,
        substring(uc.key FROM 'chat_personal_model_override:(.*)') AS context,
        uc.value,
        (
            SELECT split_part(trim(uc.value), ':', 2)::uuid
            WHERE split_part(trim(uc.value), ':', 1) = 'model'
              AND split_part(trim(uc.value), ':', 2)
                  ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
        ) AS config_id
    FROM user_configs uc
    WHERE uc.key LIKE 'chat\_personal\_model\_override:%'
      AND substring(uc.key FROM 'chat_personal_model_override:(.*)') IN ('root', 'general', 'explore')
)
INSERT INTO chat_user_model_overrides
    (user_id, organization_id, context, mode, model_config_id, reasoning_effort)
SELECT
    p.user_id,
    o.id,
    p.context,
    CASE
        WHEN trim(p.value) IN ('chat_default', 'deployment_default') THEN trim(p.value)
        ELSE 'model'
    END,
    CASE
        WHEN trim(p.value) IN ('chat_default', 'deployment_default') THEN NULL
        ELSE p.config_id
    END,
    CASE
        WHEN split_part(trim(p.value), ':', 1) = 'model'
            THEN NULLIF(split_part(trim(p.value), ':', 3), '')
        ELSE NULL
    END
FROM parsed p
CROSS JOIN organizations o
WHERE o.is_default
  AND (
      trim(p.value) IN ('chat_default', 'deployment_default')
      OR (
          split_part(trim(p.value), ':', 1) = 'model'
          AND p.config_id IS NOT NULL
          AND EXISTS (
              SELECT 1
              FROM chat_model_configs cmc
              WHERE cmc.id = p.config_id
                AND cmc.organization_id = o.id
                AND NOT cmc.deleted
          )
      )
  )
ON CONFLICT ON CONSTRAINT chat_user_model_overrides_user_organization_context_key
DO NOTHING;

DELETE FROM user_configs
WHERE key LIKE 'chat\_personal\_model\_override:%'
  AND substring(key FROM 'chat_personal_model_override:(.*)') IN ('root', 'general', 'explore');
