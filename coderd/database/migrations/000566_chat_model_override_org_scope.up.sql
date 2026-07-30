-- Move chat model overrides from key-embedded site_configs/user_configs
-- rows into typed tables (CODAGT-74, unit U4, sub-ticket CODAGT-872).
--
-- Storage before this migration: per-organization settings lived as
-- site_configs keys carrying the organization UUID in the key string, and
-- personal overrides as user_configs keys of the form
-- 'chat_personal_model_override:<org-uuid>:{context}'. That shape forced a
-- Go key builder, a dbauthz key parser, LIKE enumeration, and four runtime
-- cross-organization guards, none of which the typed schema needs.
--
-- After: chat_organization_model_overrides pins one model config per
-- (organization, context) for the five admin contexts, and
-- chat_user_model_overrides records one personal override per
-- (user, organization, context) for the three personal contexts. The
-- composite foreign key to chat_model_configs(organization_id, id) makes an
-- override naming another organization's config unrepresentable, which is
-- what lets the runtime guards be deleted. Presence means pinned; absence
-- means unset (operator ruling 2026-07-30).
--
-- Seeding rules (carried from the pre-U4 re-key migration, operator ruling
-- 2026-07-27, re-expressed as typed-column writes):
--   * Values are seeded ONLY for the default org, as an identity re-key of
--     the deployment-level value.
--   * Keys with EMPTY values are skipped (empty means unset).
--   * Model-mode values referencing soft-deleted or nonexistent configs are
--     DROPPED: they already resolve to "unavailable, use default".
--   * Sentinel personal modes (chat_default, deployment_default) pass
--     through as mode rows with NULL model_config_id.
--   * Personal contexts outside (root, general, explore), including
--     empty-context keys, are left untouched in user_configs in both
--     directions.
--   * The ':<effort>' value suffix splits into the typed reasoning_effort
--     column.
--   * Every uuid cast is guarded against empty/malformed input through a
--     regex-filtered scalar subquery: a plain AND guard is insufficient
--     because PostgreSQL does not guarantee cast evaluation order.
--
-- The uuid regex matches only canonical 36-character UUIDs; a uuid stored
-- in any non-canonical form is dropped rather than seeded, matching the
-- pre-U4 read-time rejection of such values.

-- (0) Supporting constraint for the composite foreign keys. Redundant for
-- uniqueness (id is the primary key) but required so the override tables
-- can reference (organization_id, id) and thereby bind a pin to a config
-- in the same organization.
ALTER TABLE chat_model_configs
    ADD CONSTRAINT chat_model_configs_organization_id_id_key
    UNIQUE (organization_id, id);

CREATE TABLE chat_organization_model_overrides (
    id               uuid NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    organization_id  uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    context          text NOT NULL,
    model_config_id  uuid NOT NULL,
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
    id               uuid NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id  uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    context          text NOT NULL,
    mode             text NOT NULL,
    model_config_id  uuid,
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

-- (1) Admin overrides: seed the default org's rows from the four
-- deployment-level keys. The parsed id must resolve to a LIVE chat model
-- config in the default org (soft-deleted targets are dropped).
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
      SELECT 1 FROM chat_model_configs cmc
      WHERE cmc.id = p.config_id AND NOT cmc.deleted
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

-- (2) Advisor: seed the advisor context row from the model fields of the
-- 'agents_advisor_config' JSON blob, then rewrite the blob to runtime
-- fields only. A blob without 'model_config_id', or with it null or the
-- zero UUID, carries no override: nothing is seeded and the blob keeps
-- its runtime fields.
WITH advisor AS (
    SELECT
        sc.value::jsonb AS blob,
        (
            SELECT (sc.value::jsonb ->> 'model_config_id')::uuid
            WHERE sc.value::jsonb ->> 'model_config_id'
                ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
        ) AS config_id
    FROM site_configs sc
    WHERE sc.key = 'agents_advisor_config'
      AND trim(sc.value) != ''
      AND sc.value::jsonb ? 'model_config_id'
)
INSERT INTO chat_organization_model_overrides
    (organization_id, context, model_config_id, reasoning_effort)
SELECT
    o.id,
    'advisor',
    a.config_id,
    NULLIF(trim(a.blob ->> 'reasoning_effort'), '')
FROM advisor a
CROSS JOIN organizations o
WHERE o.is_default
  AND a.config_id IS NOT NULL
  AND a.config_id != '00000000-0000-0000-0000-000000000000'::uuid
  AND EXISTS (
      SELECT 1 FROM chat_model_configs cmc
      WHERE cmc.id = a.config_id AND NOT cmc.deleted
  )
ON CONFLICT ON CONSTRAINT chat_organization_model_overrides_organization_id_context_key
DO NOTHING;

UPDATE site_configs
SET value = (
    value::jsonb - 'model_config_id' - 'reasoning_effort'
)::text
WHERE key = 'agents_advisor_config';

-- (2b) Deployment-era per-org advisor keys ('agents_advisor_model_override:<org-uuid>')
-- were written by the pre-U4 head's own re-key migration; any deployment
-- that ran that head holds them. Fold the default org's value into the
-- advisor row, then drop every org-suffixed advisor key.
WITH parsed AS (
    SELECT
        sc.value,
        (
            SELECT split_part(trim(sc.value), ':', 1)::uuid
            WHERE split_part(trim(sc.value), ':', 1)
                ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
        ) AS config_id
    FROM site_configs sc
    JOIN organizations o ON o.is_default
    WHERE sc.key = 'agents_advisor_model_override:' || o.id::text
      AND trim(sc.value) != ''
)
INSERT INTO chat_organization_model_overrides
    (organization_id, context, model_config_id, reasoning_effort)
SELECT
    o.id,
    'advisor',
    p.config_id,
    NULLIF(split_part(trim(p.value), ':', 2), '')
FROM parsed p
CROSS JOIN organizations o
WHERE o.is_default
  AND p.config_id IS NOT NULL
  AND EXISTS (
      SELECT 1 FROM chat_model_configs cmc
      WHERE cmc.id = p.config_id AND NOT cmc.deleted
  )
ON CONFLICT ON CONSTRAINT chat_organization_model_overrides_organization_id_context_key
DO NOTHING;

DELETE FROM site_configs
WHERE key LIKE 'agents\_advisor\_model\_override:%'
  AND key ~ '^agents_advisor_model_override:[0-9a-fA-F-]{36}$';

-- (3) Personal overrides: fan out the deployment-level context keys to
-- default-org rows. Sentinels become mode rows with NULL model_config_id;
-- model-mode values must name a LIVE config; empty values are skipped;
-- malformed, dangling, and deleted-target values are dropped because they
-- already resolve to "unavailable, use default". Keys whose context is
-- outside (root, general, explore) are left untouched in both directions.
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
              SELECT 1 FROM chat_model_configs cmc
              WHERE cmc.id = p.config_id AND NOT cmc.deleted
          )
      )
  )
ON CONFLICT ON CONSTRAINT chat_user_model_overrides_user_organization_context_key
DO NOTHING;

DELETE FROM user_configs
WHERE key LIKE 'chat\_personal\_model\_override:%'
  AND substring(key FROM 'chat_personal_model_override:(.*)') IN ('root', 'general', 'explore');

-- (3b) Deployment-era org-suffixed personal keys
-- ('chat_personal_model_override:<org-uuid>:{context}') written by the
-- pre-U4 head fold back the same way, for the default org only.
WITH parsed AS (
    SELECT
        uc.user_id,
        substring(uc.key FROM '^chat_personal_model_override:[0-9a-fA-F-]{36}:(.*)$') AS context,
        uc.value,
        (
            SELECT split_part(trim(uc.value), ':', 2)::uuid
            WHERE split_part(trim(uc.value), ':', 1) = 'model'
              AND split_part(trim(uc.value), ':', 2)
                  ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
        ) AS config_id
    FROM user_configs uc
    JOIN organizations o ON o.is_default
    WHERE uc.key ~ ('^chat_personal_model_override:' || o.id::text || ':(root|general|explore)$')
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
              SELECT 1 FROM chat_model_configs cmc
              WHERE cmc.id = p.config_id AND NOT cmc.deleted
          )
      )
  )
ON CONFLICT ON CONSTRAINT chat_user_model_overrides_user_organization_context_key
DO NOTHING;

DELETE FROM user_configs uc
USING organizations o
WHERE o.is_default
  AND uc.key ~ ('^chat_personal_model_override:' || o.id::text || ':(root|general|explore)$');

-- The deployment toggle 'agents_chat_personal_model_overrides_enabled'
-- stays in site_configs by contract; it is deliberately not read here.
