-- Explode default-org chat model configs to every live non-default
-- organization (CODAGT-709, stage 3 of 3: org-scoping cutover). After this
-- migration every live org owns a full set of model configs and all
-- references inside live non-default orgs point at same-org rows.
--
-- Mapping design (operator ruling): NO provenance column, NO persisted
-- mapping of any kind. A transaction-scoped TEMPORARY lookup table
-- (orig_id, org_id, copy_id) ON COMMIT DROP maps each default-org original
-- to its per-org copy; the copy insert, all four reference remaps, and the
-- compaction-threshold fan-out join it. The table vanishes when the
-- migration framework commits this migration's transaction, so nothing
-- mapping-related persists. Copy ids come from gen_random_uuid(); no
-- hash-derived ids (md5() errors on FIPS-mode PostgreSQL builds).

CREATE TEMPORARY TABLE model_config_copy_map (
    orig_id uuid NOT NULL,
    org_id uuid NOT NULL,
    copy_id uuid NOT NULL,
    PRIMARY KEY (orig_id, org_id)
) ON COMMIT DROP;

-- (a) Stage LIVE default-org chat_model_configs x every live non-default org
-- in the temp map with a fresh copy id, then insert the copies. Staging the
-- id in the map first lets the remap statements below resolve copies without
-- recomputing anything.
INSERT INTO model_config_copy_map (orig_id, org_id, copy_id)
SELECT cmc.id, o.id, gen_random_uuid()
FROM chat_model_configs cmc
JOIN organizations def ON def.id = cmc.organization_id AND def.is_default
CROSS JOIN organizations o
WHERE NOT o.is_default AND NOT o.deleted
  AND NOT cmc.deleted;

-- Copies inherit every behavioral field from the original, including
-- created_at/updated_at and created_by/updated_by: a copy is the same
-- logical config re-homed, and the audit-facing identity of who configured
-- it survives the explosion. group_acl is re-keyed to the copy's org (the
-- Everyone group of an organization always has the organization's own ID,
-- see 000058) carrying the original's entry verbatim, so members of the
-- target org keep read access through the everyone entry.
INSERT INTO chat_model_configs
    (id, model, display_name, created_by, updated_by, enabled, is_default,
     deleted, deleted_at, created_at, updated_at, context_limit,
     compression_threshold, options, ai_provider_id, organization_id,
     group_acl, user_acl)
SELECT
    m.copy_id,
    cmc.model, cmc.display_name, cmc.created_by, cmc.updated_by, cmc.enabled,
    cmc.is_default, cmc.deleted, cmc.deleted_at, cmc.created_at,
    cmc.updated_at, cmc.context_limit, cmc.compression_threshold, cmc.options,
    cmc.ai_provider_id, m.org_id,
    jsonb_build_object(
        m.org_id::text,
        COALESCE(cmc.group_acl -> cmc.organization_id::text,
                 '{"permissions": ["read"]}'::jsonb)
    ),
    '{}'::jsonb
FROM model_config_copy_map m
JOIN chat_model_configs cmc ON cmc.id = m.orig_id
WHERE NOT cmc.deleted;

-- (a2) Stage + copy SOFT-DELETED default-org chat_model_configs ONLY to live
-- non-default orgs that actually reference them. A reference is any of:
--   chats.last_model_config_id, chat_messages.model_config_id (via chat),
--   chat_queued_messages.model_config_id (via chat), or
--   chat_debug_runs.model_config_id (via chat) pointing at the deleted config.
-- Copies keep deleted/deleted_at so every historical reference has an
-- FK-valid, attribution-preserving target without resurrecting the config.
INSERT INTO model_config_copy_map (orig_id, org_id, copy_id)
SELECT DISTINCT ON (cmc.id, o.id) cmc.id, o.id, gen_random_uuid()
FROM chat_model_configs cmc
JOIN organizations def ON def.id = cmc.organization_id AND def.is_default
JOIN organizations o ON NOT o.is_default AND NOT o.deleted
WHERE cmc.deleted
  AND (
    EXISTS (SELECT 1 FROM chats c
            WHERE c.last_model_config_id = cmc.id AND c.organization_id = o.id)
    OR
    EXISTS (SELECT 1 FROM chat_messages mm
            JOIN chats c ON c.id = mm.chat_id
            WHERE mm.model_config_id = cmc.id AND c.organization_id = o.id)
    OR
    EXISTS (SELECT 1 FROM chat_queued_messages q
            JOIN chats c ON c.id = q.chat_id
            WHERE q.model_config_id = cmc.id AND c.organization_id = o.id)
    OR
    EXISTS (SELECT 1 FROM chat_debug_runs d
            JOIN chats c ON c.id = d.chat_id
            WHERE d.model_config_id = cmc.id AND c.organization_id = o.id)
  );

INSERT INTO chat_model_configs
    (id, model, display_name, created_by, updated_by, enabled, is_default,
     deleted, deleted_at, created_at, updated_at, context_limit,
     compression_threshold, options, ai_provider_id, organization_id,
     group_acl, user_acl)
SELECT
    m.copy_id,
    cmc.model, cmc.display_name, cmc.created_by, cmc.updated_by, cmc.enabled,
    cmc.is_default, cmc.deleted, cmc.deleted_at, cmc.created_at,
    cmc.updated_at, cmc.context_limit, cmc.compression_threshold, cmc.options,
    cmc.ai_provider_id, m.org_id,
    jsonb_build_object(
        m.org_id::text,
        COALESCE(cmc.group_acl -> cmc.organization_id::text,
                 '{"permissions": ["read"]}'::jsonb)
    ),
    '{}'::jsonb
FROM model_config_copy_map m
JOIN chat_model_configs cmc ON cmc.id = m.orig_id
WHERE cmc.deleted;

-- (b) Remap chats.last_model_config_id in live non-default orgs to the
-- same-org copy via the temp map. Soft-deleted orgs are excluded: their
-- chats keep original references (FK-valid, unreachable). The NOT EXISTS
-- live-original guard skips references to a config that has only a deleted
-- copy in the org while its live fan-out copy does not exist (the live
-- fan-out is total, so this can only happen when the original is soft-
-- deleted in this org's history but live in the default org); without it
-- the reference would remap to the deleted copy, silently turning an
-- active model into a deleted one.
UPDATE chats c
SET last_model_config_id = m.copy_id
FROM model_config_copy_map m
JOIN chat_model_configs orig ON orig.id = m.orig_id
WHERE c.last_model_config_id = m.orig_id
  AND m.org_id = c.organization_id
  AND NOT EXISTS (SELECT 1 FROM organizations corg
                  WHERE corg.id = c.organization_id AND corg.deleted)
  AND NOT EXISTS (SELECT 1 FROM model_config_copy_map d
                  JOIN chat_model_configs dcp ON dcp.id = d.copy_id AND dcp.deleted
                  WHERE d.orig_id = m.orig_id
                    AND d.org_id = c.organization_id
                    AND NOT orig.deleted);

-- (b2) Remap chat_messages.model_config_id via the owning chat's org (live only).
UPDATE chat_messages mm
SET model_config_id = m.copy_id
FROM chats c, model_config_copy_map m
JOIN chat_model_configs orig ON orig.id = m.orig_id
WHERE c.id = mm.chat_id
  AND mm.model_config_id = m.orig_id
  AND m.org_id = c.organization_id
  AND NOT EXISTS (SELECT 1 FROM organizations corg
                  WHERE corg.id = c.organization_id AND corg.deleted)
  AND NOT EXISTS (SELECT 1 FROM model_config_copy_map d
                  JOIN chat_model_configs dcp ON dcp.id = d.copy_id AND dcp.deleted
                  WHERE d.orig_id = m.orig_id
                    AND d.org_id = c.organization_id
                    AND NOT orig.deleted);

-- (b3) Remap chat_queued_messages.model_config_id via the owning chat's org
-- (live only). The column has no FK, so dangling ids would not fail; the
-- remap keeps a queued message's promoted model inside its chat's org.
UPDATE chat_queued_messages q
SET model_config_id = m.copy_id
FROM chats c, model_config_copy_map m
JOIN chat_model_configs orig ON orig.id = m.orig_id
WHERE c.id = q.chat_id
  AND q.model_config_id = m.orig_id
  AND m.org_id = c.organization_id
  AND NOT EXISTS (SELECT 1 FROM organizations corg
                  WHERE corg.id = c.organization_id AND corg.deleted)
  AND NOT EXISTS (SELECT 1 FROM model_config_copy_map d
                  JOIN chat_model_configs dcp ON dcp.id = d.copy_id AND dcp.deleted
                  WHERE d.orig_id = m.orig_id
                    AND d.org_id = c.organization_id
                    AND NOT orig.deleted);

-- (b4) Remap chat_debug_runs.model_config_id via the owning chat's org
-- (live only). Also FK-less; attribution-only, remapped for consistency.
UPDATE chat_debug_runs d
SET model_config_id = m.copy_id
FROM chats c, model_config_copy_map m
JOIN chat_model_configs orig ON orig.id = m.orig_id
WHERE c.id = d.chat_id
  AND d.model_config_id = m.orig_id
  AND m.org_id = c.organization_id
  AND NOT EXISTS (SELECT 1 FROM organizations corg
                  WHERE corg.id = c.organization_id AND corg.deleted)
  AND NOT EXISTS (SELECT 1 FROM model_config_copy_map dx
                  JOIN chat_model_configs dcp ON dcp.id = dx.copy_id AND dcp.deleted
                  WHERE dx.orig_id = m.orig_id
                    AND dx.org_id = c.organization_id
                    AND NOT orig.deleted);

-- (c) Fan out user_configs compaction-threshold keys. A key
-- 'chat_compaction_threshold_pct:<orig-id>' earns one row per copy of that
-- original in the temp map, same user, same value, key rewritten to the
-- copy id. The fan-out is copy-precise by construction (it can only
-- produce keys for copies that exist): live originals reach every live
-- org, soft-deleted originals reach only the orgs that received a
-- referenced copy, and an original with zero map rows (deleted and
-- unreferenced) produces nothing. Original keys stay: they reference
-- default-org originals, still valid. The fan-out is deliberately NOT
-- membership-filtered: chats pinned to deleted models are the norm, and a
-- threshold must keep resolving for any chat that lands on a copy. The PK
-- (user_id, key) cannot collide because copy ids are fresh and no existing
-- key embeds a copy id; ON CONFLICT DO NOTHING is belt-and-braces only.
INSERT INTO user_configs (user_id, key, value)
SELECT uc.user_id, 'chat_compaction_threshold_pct:' || m.copy_id::text, uc.value
FROM user_configs uc
JOIN model_config_copy_map m
  ON uc.key = 'chat_compaction_threshold_pct:' || m.orig_id::text
ON CONFLICT (user_id, key) DO NOTHING;

-- (d) Seed the everyone-in-org read entry on any existing row whose
-- group_acl lacks its own org's key. 000562 seeded every row that existed
-- then; this covers rows created between 000562 and this migration (the
-- pre-cutover handlers did not seed it). The entry's permissions are
-- preserved when an entry already exists for another org's key shape.
UPDATE chat_model_configs
SET group_acl = jsonb_build_object(
    organization_id::text,
    jsonb_build_object('permissions', jsonb_build_array('read'::text))
) || group_acl
WHERE NOT (group_acl ? organization_id::text);
