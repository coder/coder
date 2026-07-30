-- Reverse of 000566_chat_model_override_org_scope.up.sql.
--
-- Restore the deployment-level override keys from the DEFAULT org's
-- per-org keys (the only org the up migration seeded), then delete every
-- per-org override key and every org-suffixed personal override key in the
-- three fanned contexts.
--
-- Irreversibility: the up migration dropped model-mode values that named a
-- missing or soft-deleted config, and did not seed non-default orgs. Those
-- values are not reconstructed by this down: the pre-up rows they came
-- from resolved to "unavailable, use default" already, so the post-down
-- behavior matches the pre-up behavior.
--
-- 'agents_advisor_config' is not touched: the up migration removed the
-- model fields from its JSON in place, so the pre-M5 GET handler simply
-- sees no model selection after this down, which matches how it already
-- treats a blob without 'model_config_id'.
--
-- Personal-override rows with contexts outside (root, general, explore)
-- were left untouched by the up and are left untouched here.
--
-- uuid casts are guarded the same way as in the up: a regex-filtered
-- scalar subquery, because PostgreSQL does not guarantee cast evaluation
-- order, and hostile keys (empty or malformed org suffixes, unparsable
-- values) must not abort the down.

-- (1) Admin overrides: restore the deployment-level keys from the default
-- org's suffixed keys. Non-empty, parseable values naming a LIVE config
-- are restored; empty or unparsable values restore '' (unset), matching
-- the pre-up behavior of those rows at read time.
WITH parsed AS (
    SELECT
        substring(sc.key FROM '^(agents_chat_(?:general|explore|title_generation|compaction)_model_override):') AS base_key,
        sc.value,
        (
            SELECT split_part(trim(sc.value), ':', 1)::uuid
            WHERE split_part(trim(sc.value), ':', 1)
                ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
        ) AS config_id
    FROM site_configs sc
    JOIN organizations o ON o.is_default
    WHERE sc.key LIKE 'agents\_chat\_%\_model\_override:%'
      AND sc.key ~ ('^agents_chat_(general|explore|title_generation|compaction)_model_override:' || o.id::text || '$')
)
INSERT INTO site_configs (key, value)
SELECT
    p.base_key,
    CASE
        WHEN trim(p.value) != ''
            AND p.config_id IS NOT NULL
            AND EXISTS (
                SELECT 1 FROM chat_model_configs cmc
                WHERE cmc.id = p.config_id AND NOT cmc.deleted
            )
        THEN p.value
        ELSE ''
    END
FROM parsed p
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

DELETE FROM site_configs
WHERE key LIKE 'agents\_chat\_%\_model\_override:%'
  AND key ~ '^agents_chat_(general|explore|title_generation|compaction)_model_override:[0-9a-fA-F-]{36}$';

-- (2) Advisor: restore 'agents_advisor_model_override' (unsuffixed, the
-- pre-M5-era key this unit supersedes) from the default org's key when the
-- stored value names a live config, then drop every per-org advisor key.
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
)
INSERT INTO site_configs (key, value)
SELECT
    'agents_advisor_model_override',
    p.value
FROM parsed p
WHERE trim(p.value) != ''
  AND p.config_id IS NOT NULL
  AND EXISTS (
      SELECT 1 FROM chat_model_configs cmc
      WHERE cmc.id = p.config_id AND NOT cmc.deleted
  )
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

DELETE FROM site_configs
WHERE key LIKE 'agents\_advisor\_model\_override:%'
  AND key ~ '^agents_advisor_model_override:[0-9a-fA-F-]{36}$';

-- (3) Personal overrides: fold the default org's suffixed keys back to the
-- deployment-level context keys, then delete every org-suffixed key in the
-- three fanned contexts. Fold-back accepts the same value classes the up
-- accepted (empty, sentinel, or model mode naming a live config); anything
-- else is dropped with the suffixed rows.
WITH parsed AS (
    SELECT
        uc.user_id,
        uc.value,
        substring(uc.key FROM '^chat_personal_model_override:[0-9a-fA-F-]{36}:(.*)$') AS context
    FROM user_configs uc
    JOIN organizations o ON o.is_default
    WHERE uc.key LIKE 'chat\_personal\_model\_override:%'
      AND uc.key ~ ('^chat_personal_model_override:' || o.id::text || ':(root|general|explore)$')
),
accepted AS (
    SELECT
        p.user_id,
        p.context,
        p.value
    FROM parsed p
    WHERE trim(p.value) = ''
       OR trim(p.value) IN ('chat_default', 'deployment_default')
       OR (
           split_part(trim(p.value), ':', 1) = 'model'
           AND (
               SELECT split_part(trim(p.value), ':', 2)::uuid
               WHERE split_part(trim(p.value), ':', 2)
                   ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
           ) IN (SELECT cmc.id FROM chat_model_configs cmc WHERE NOT cmc.deleted)
       )
)
INSERT INTO user_configs (user_id, key, value)
SELECT
    a.user_id,
    'chat_personal_model_override:' || a.context,
    a.value
FROM accepted a
ON CONFLICT (user_id, key) DO UPDATE SET value = EXCLUDED.value;

DELETE FROM user_configs
WHERE key LIKE 'chat\_personal\_model\_override:%'
  AND key ~ '^chat_personal_model_override:[0-9a-fA-F-]{36}:(root|general|explore)$';
