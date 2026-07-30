-- Reverse of 000566_chat_model_override_org_scope.up.sql.
--
-- Restore the deployment-level site_configs/user_configs override keys from
-- the DEFAULT org's typed rows (the only org the up migration seeded), then
-- drop the typed tables and the supporting unique constraint.
--
-- Irreversibility: the up migration dropped model-mode values that named a
-- missing or soft-deleted config, and did not seed non-default orgs. Those
-- values are not reconstructed by this down: they resolved to "unavailable,
-- use default" already, so post-down behavior matches pre-up behavior.
-- Absence maps to absence (operator ruling 2026-07-30): a context with no
-- row produces no key, which the pre-up code reads as unset.
--
-- 'agents_advisor_config' is not rewritten here: the up removed the model
-- fields from its JSON in place. The pre-up GET handler treats a blob
-- without 'model_config_id' as "no advisor model selection", matching
-- pre-up behavior for deployments that never set one. The advisor override
-- itself is restored to the pre-U4-head's unsuffixed
-- 'agents_advisor_model_override' key shape only if the pre-U4-head's
-- re-key ran; for a deployment coming from the un-re-keyed state, the
-- model fields would have lived in the JSON and are not restorable. Both
-- pre-U4 read paths degrade a missing advisor override to the chat model.
--
-- Personal-override rows with contexts outside (root, general, explore)
-- were left untouched by the up and are left untouched here.

-- (1) Admin overrides: restore the four deployment-level keys from the
-- default org's rows, formatted '<config-id>[:<effort>]'.
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
      SELECT 1 FROM chat_model_configs cmc
      WHERE cmc.id = omo.model_config_id AND NOT cmc.deleted
  )
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

-- (2) Advisor: restore the unsuffixed pre-U4-head key from the default
-- org's advisor row when the pinned config is still live.
INSERT INTO site_configs (key, value)
SELECT
    'agents_advisor_model_override',
    omo.model_config_id::text || CASE
        WHEN omo.reasoning_effort IS NOT NULL AND omo.reasoning_effort != ''
            THEN ':' || omo.reasoning_effort
        ELSE ''
    END
FROM chat_organization_model_overrides omo
JOIN organizations o ON o.id = omo.organization_id
WHERE o.is_default
  AND omo.context = 'advisor'
  AND EXISTS (
      SELECT 1 FROM chat_model_configs cmc
      WHERE cmc.id = omo.model_config_id AND NOT cmc.deleted
  )
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

-- (3) Personal overrides: fold the default org's rows back to the
-- deployment-level context keys. Sentinel modes become bare sentinel
-- strings; model mode becomes 'model:<config-id>[:<effort>]'. Model rows
-- whose pinned config was soft-deleted after seeding are dropped, matching
-- the pre-up "unavailable, use default" resolution of such values.
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
          SELECT 1 FROM chat_model_configs cmc
          WHERE cmc.id = umo.model_config_id AND NOT cmc.deleted
      )
  )
ON CONFLICT ON CONSTRAINT user_configs_pkey
DO UPDATE SET value = EXCLUDED.value;

DROP TABLE chat_user_model_overrides;
DROP TABLE chat_organization_model_overrides;

ALTER TABLE chat_model_configs
    DROP CONSTRAINT chat_model_configs_organization_id_id_key;
