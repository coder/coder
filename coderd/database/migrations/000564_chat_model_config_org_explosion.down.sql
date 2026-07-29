-- DOWN for the chat model config org explosion. UNSUPPORTED and best-effort
-- per operator ruling: there is no persisted provenance, so this down
-- cannot distinguish a copy from an organically created non-default-org row
-- that happens to share (ai_provider_id, model) with a default-org row. It
-- must run green and must never lose chats; fidelity loss on pathological
-- duplicates is accepted.
--
-- Copy identification: a non-default-org row is treated as a copy iff a
-- default-org row exists with the same (ai_provider_id, model). Retarget
-- resolves that default-org row deterministically with DISTINCT ON ordered
-- by (created_at ASC, id ASC) so duplicates pick one stable row.

-- Restore chats.last_model_config_id from copies back to the default-org
-- original matched by (ai_provider_id, model).
UPDATE chats c
SET last_model_config_id = orig.id
FROM chat_model_configs cp
JOIN LATERAL (
    SELECT d.id
    FROM chat_model_configs d
    JOIN organizations def ON def.id = d.organization_id AND def.is_default
    WHERE d.ai_provider_id IS NOT DISTINCT FROM cp.ai_provider_id
      AND d.model = cp.model
    ORDER BY d.created_at ASC, d.id ASC
    LIMIT 1
) orig ON true
WHERE c.last_model_config_id = cp.id
  AND NOT EXISTS (SELECT 1 FROM organizations odef
                  WHERE odef.id = cp.organization_id AND odef.is_default);

-- Restore chat_messages.model_config_id from copies back to originals.
UPDATE chat_messages mm
SET model_config_id = orig.id
FROM chat_model_configs cp
JOIN LATERAL (
    SELECT d.id
    FROM chat_model_configs d
    JOIN organizations def ON def.id = d.organization_id AND def.is_default
    WHERE d.ai_provider_id IS NOT DISTINCT FROM cp.ai_provider_id
      AND d.model = cp.model
    ORDER BY d.created_at ASC, d.id ASC
    LIMIT 1
) orig ON true
WHERE mm.model_config_id = cp.id
  AND NOT EXISTS (SELECT 1 FROM organizations odef
                  WHERE odef.id = cp.organization_id AND odef.is_default);

-- Restore chat_queued_messages.model_config_id from copies back to originals.
UPDATE chat_queued_messages q
SET model_config_id = orig.id
FROM chat_model_configs cp
JOIN LATERAL (
    SELECT d.id
    FROM chat_model_configs d
    JOIN organizations def ON def.id = d.organization_id AND def.is_default
    WHERE d.ai_provider_id IS NOT DISTINCT FROM cp.ai_provider_id
      AND d.model = cp.model
    ORDER BY d.created_at ASC, d.id ASC
    LIMIT 1
) orig ON true
WHERE q.model_config_id = cp.id
  AND NOT EXISTS (SELECT 1 FROM organizations odef
                  WHERE odef.id = cp.organization_id AND odef.is_default);

-- Restore chat_debug_runs.model_config_id from copies back to originals.
UPDATE chat_debug_runs d
SET model_config_id = orig.id
FROM chat_model_configs cp
JOIN LATERAL (
    SELECT d.id
    FROM chat_model_configs d
    JOIN organizations def ON def.id = d.organization_id AND def.is_default
    WHERE d.ai_provider_id IS NOT DISTINCT FROM cp.ai_provider_id
      AND d.model = cp.model
    ORDER BY d.created_at ASC, d.id ASC
    LIMIT 1
) orig ON true
WHERE d.model_config_id = cp.id
  AND NOT EXISTS (SELECT 1 FROM organizations odef
                  WHERE odef.id = cp.organization_id AND odef.is_default);

-- Delete copied chat_model_configs (non-default-org rows whose
-- (ai_provider_id, model) matches a default-org row). References were
-- retargeted above, so the deletes cannot violate the chats/chat_messages
-- FKs.
DELETE FROM chat_model_configs cp
WHERE EXISTS (
    SELECT 1 FROM chat_model_configs orig
    JOIN organizations def ON def.id = orig.organization_id AND def.is_default
    WHERE orig.ai_provider_id IS NOT DISTINCT FROM cp.ai_provider_id
      AND orig.model = cp.model
)
AND NOT EXISTS (
    SELECT 1 FROM organizations odef
    WHERE odef.id = cp.organization_id AND odef.is_default
);

-- Best-effort threshold-key cleanup: delete compaction-threshold keys whose
-- embedded config id no longer exists anywhere after the copy deletes.
-- Original keys survive because default-org originals always survive.
-- Keys with a malformed or empty suffix are guarded BEFORE the uuid cast
-- (they cannot name an existing config, so they are pruned like any other
-- dangling key) instead of aborting the down.
DELETE FROM user_configs uc
WHERE uc.key LIKE 'chat_compaction_threshold_pct:%'
  AND NOT EXISTS (
      SELECT 1 FROM chat_model_configs cmc
      WHERE cmc.id = (
        SELECT substring(uc.key FROM 'chat_compaction_threshold_pct:(.*)')::uuid
        WHERE substring(uc.key FROM 'chat_compaction_threshold_pct:(.*)')
              ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
      )
  );
