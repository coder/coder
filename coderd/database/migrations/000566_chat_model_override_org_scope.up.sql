-- Re-key chat model override settings from deployment-level keys to
-- per-organization keys (CODAGT-74, unit M5, sub-ticket CODAGT-872).
--
-- The four admin model overrides move from
-- 'agents_chat_<context>_model_override' to
-- 'agents_chat_<context>_model_override:<org-uuid>'; the advisor model
-- selection moves out of the 'agents_advisor_config' JSON blob (which keeps
-- only runtime fields) into 'agents_advisor_model_override:<org-uuid>'; and
-- personal model overrides fan out from
-- 'chat_personal_model_override:{context}' to
-- 'chat_personal_model_override:<org-uuid>:{context}' in user_configs.
--
-- Seeding rules (operator ruling 2026-07-27: no cross-release copy mapping
-- exists, so non-default orgs start with no overrides and configure their
-- own):
--   * Values are seeded ONLY for the default org, as an identity re-key of
--     the deployment-level value.
--   * Keys with EMPTY values are skipped, not seeded (dogfood has one); an
--     empty value means "unset".
--   * Model-mode values referencing soft-deleted configs are DROPPED, not
--     re-keyed: they already resolve to "unavailable, use default" today.
--   * Values referencing config rows that no longer exist are skipped.
--   * The ':<effort>' suffix is preserved verbatim.
--   * Sentinel personal-override modes (chat_default, deployment_default)
--     pass through unchanged; malformed personal values in the three fanned
--     contexts are treated as model mode and dropped because they name no
--     existing config. Keys with contexts outside (root, general, explore),
--     including empty-context keys, are left untouched in both directions
--     (the down restores only what the up re-keyed).
--   * Every uuid cast is guarded against empty/malformed input through a
--     regex-filtered scalar subquery: a plain AND guard is insufficient
--     because PostgreSQL does not guarantee cast evaluation order.
--
-- The uuid regex below matches only canonical 36-character UUIDs. A uuid
-- stored in any non-canonical form (e.g. without dashes) is dropped rather
-- than re-keyed; parseModelOverride in coderd/x/chatd rejects such values
-- at read time as well, so nothing resolvable is lost.

-- (1) Admin overrides: re-key the four deployment-level keys to the default
-- org's suffixed key. The parsed id must resolve to a LIVE chat model
-- config (soft-deleted targets are dropped, not re-keyed).
WITH parsed AS (
    SELECT
        sc.key,
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
INSERT INTO site_configs (key, value)
SELECT
    p.key || ':' || o.id::text,
    p.value
FROM parsed p
CROSS JOIN organizations o
WHERE o.is_default
  AND p.config_id IS NOT NULL
  AND EXISTS (
      SELECT 1 FROM chat_model_configs cmc
      WHERE cmc.id = p.config_id AND NOT cmc.deleted
  )
ON CONFLICT (key) DO NOTHING;

DELETE FROM site_configs
WHERE key IN (
    'agents_chat_general_model_override',
    'agents_chat_explore_model_override',
    'agents_chat_title_generation_model_override',
    'agents_chat_compaction_model_override'
);

-- (2) Advisor: split the model selection out of the 'agents_advisor_config'
-- JSON into the default org's 'agents_advisor_model_override:<org-uuid>'
-- key (value format '<config-id>[:<effort>]', matching the other override
-- contexts), and rewrite the JSON to runtime fields only. A stored blob
-- without 'model_config_id', or with it set to null or the zero UUID,
-- carries no override: nothing is seeded and the blob keeps its runtime
-- fields. The jsonb_set shape is identical to the stored blob minus the
-- model fields, so rewriting it unconditionally preserves every runtime
-- key (including 'enabled', whose stored value is inert: the experiment
-- flag is the sole gate).
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
),
seed AS (
    INSERT INTO site_configs (key, value)
    SELECT
        'agents_advisor_model_override:' || o.id::text,
        a.config_id::text || CASE
            WHEN a.blob ->> 'reasoning_effort' IS NOT NULL
                THEN ':' || (a.blob ->> 'reasoning_effort')
            ELSE ''
        END
    FROM advisor a
    CROSS JOIN organizations o
    WHERE o.is_default
      AND a.config_id IS NOT NULL
      AND a.config_id != '00000000-0000-0000-0000-000000000000'::uuid
      AND EXISTS (
          SELECT 1 FROM chat_model_configs cmc
          WHERE cmc.id = a.config_id AND NOT cmc.deleted
      )
    ON CONFLICT (key) DO NOTHING
    RETURNING key
)
UPDATE site_configs
SET value = (
    value::jsonb - 'model_config_id' - 'reasoning_effort'
)::text
WHERE key = 'agents_advisor_config';

-- (3) Personal overrides: fan out to the default org only. Contexts not in
-- (root, general, explore) are left untouched for the down migration to
-- restore (they are unreachable through the API today). Model-mode values
-- must parse and name a LIVE config; sentinel and empty values pass
-- through; everything else (malformed, dangling, deleted target) is
-- dropped because it already resolves to "unavailable, use default".
WITH parsed AS (
    SELECT
        uc.user_id,
        uc.key,
        uc.value,
        substring(uc.key FROM 'chat_personal_model_override:(.*)') AS context,
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
INSERT INTO user_configs (user_id, key, value)
SELECT
    p.user_id,
    'chat_personal_model_override:' || o.id::text || ':' || p.context,
    p.value
FROM parsed p
CROSS JOIN organizations o
WHERE o.is_default
  AND (
      -- Empty and sentinel values pass through verbatim.
      trim(p.value) = ''
      OR trim(p.value) IN ('chat_default', 'deployment_default')
      -- Model mode: the parsed id must resolve to a live config.
      OR (
          split_part(trim(p.value), ':', 1) = 'model'
          AND p.config_id IS NOT NULL
          AND EXISTS (
              SELECT 1 FROM chat_model_configs cmc
              WHERE cmc.id = p.config_id AND NOT cmc.deleted
          )
      )
  )
ON CONFLICT (user_id, key) DO NOTHING;

DELETE FROM user_configs
WHERE key LIKE 'chat\_personal\_model\_override:%'
  AND substring(key FROM 'chat_personal_model_override:(.*)') IN ('root', 'general', 'explore');
