-- name: GetOrganizationChatModelConfigByID :one
SELECT *
FROM chat_model_configs
WHERE id = @id::uuid
  AND organization_id = @organization_id::uuid
  AND deleted = FALSE;

-- name: GetOrganizationChatModelConfigByIDForAuthorization :one
SELECT *
FROM chat_model_configs
WHERE id = @id::uuid
  AND organization_id IS NOT NULL
  AND deleted = FALSE;

-- name: GetOrganizationDefaultChatModelConfig :one
SELECT *
FROM chat_model_configs
WHERE organization_id = @organization_id::uuid
  AND is_default = TRUE
  AND deleted = FALSE;

-- name: GetOrganizationChatModelConfigs :many
SELECT cmc.*
FROM chat_model_configs cmc
LEFT JOIN ai_providers ap ON ap.id = cmc.ai_provider_id
WHERE cmc.organization_id = @organization_id::uuid
  AND cmc.deleted = FALSE
  -- Authorize Filter clause will be injected below in GetAuthorizedOrganizationChatModelConfigs
  -- @authorize_filter
ORDER BY ap.type::text ASC, cmc.model ASC, cmc.updated_at DESC, cmc.id DESC;

-- name: GetOrganizationEnabledChatModelConfigs :many
SELECT sqlc.embed(cmc), ap.type::text AS provider
FROM chat_model_configs cmc
JOIN ai_providers ap ON ap.id = cmc.ai_provider_id
WHERE cmc.organization_id = @organization_id::uuid
  AND cmc.enabled = TRUE
  AND cmc.deleted = FALSE
  AND ap.enabled = TRUE
  AND ap.deleted = FALSE
  -- Authorize Filter clause will be injected below in GetAuthorizedOrganizationEnabledChatModelConfigs
  -- @authorize_filter
ORDER BY ap.type::text ASC, cmc.model ASC, cmc.updated_at DESC, cmc.id DESC;

-- name: GetOrganizationEnabledChatModelConfigByID :one
SELECT cmc.*
FROM chat_model_configs cmc
JOIN ai_providers ap ON ap.id = cmc.ai_provider_id
WHERE cmc.id = @id::uuid
  AND cmc.organization_id = @organization_id::uuid
  AND cmc.deleted = FALSE
  AND cmc.enabled = TRUE
  AND ap.enabled = TRUE
  AND ap.deleted = FALSE;

-- name: InsertOrganizationChatModelConfig :one
INSERT INTO chat_model_configs (
  model, display_name, created_by, updated_by, enabled, is_default,
  context_limit, compression_threshold, options, ai_provider_id,
  organization_id, user_acl, group_acl, legacy_model_config_id,
  inherits_legacy_config
) VALUES (
  @model::text, @display_name::text, sqlc.narg('created_by')::uuid,
  sqlc.narg('updated_by')::uuid, @enabled::boolean, @is_default::boolean,
  @context_limit::bigint, @compression_threshold::integer, @options::jsonb,
  sqlc.narg('ai_provider_id')::uuid, @organization_id::uuid, @user_acl,
  @group_acl, NULL, FALSE
) RETURNING *;

-- name: UpdateOrganizationChatModelConfig :one
UPDATE chat_model_configs
SET model = @model::text,
    display_name = @display_name::text,
    updated_by = sqlc.narg('updated_by')::uuid,
    enabled = @enabled::boolean,
    is_default = @is_default::boolean,
    context_limit = @context_limit::bigint,
    compression_threshold = @compression_threshold::integer,
    options = @options::jsonb,
    ai_provider_id = sqlc.narg('ai_provider_id')::uuid,
    inherits_legacy_config = FALSE,
    updated_at = NOW()
WHERE id = @id::uuid
  AND organization_id = @organization_id::uuid
  AND deleted = FALSE
RETURNING *;

-- name: SoftDeleteOrganizationChatModelConfig :exec
UPDATE chat_model_configs
SET inherits_legacy_config = FALSE,
    deleted = TRUE,
    deleted_at = NOW(),
    updated_at = NOW()
WHERE id = @id::uuid
  AND organization_id = @organization_id::uuid
  AND deleted = FALSE;

-- name: UnsetOrganizationDefaultChatModelConfigs :exec
UPDATE chat_model_configs
SET is_default = FALSE, updated_at = NOW()
WHERE organization_id = @organization_id::uuid
  AND is_default = TRUE
  AND deleted = FALSE;

-- name: ElectOrganizationDefaultChatModelConfig :one
-- The caller must clear the existing organization default before electing a replacement.
UPDATE chat_model_configs
SET is_default = TRUE, updated_at = NOW()
WHERE id = (
  SELECT cmc.id
  FROM chat_model_configs cmc
  LEFT JOIN ai_providers ap ON ap.id = cmc.ai_provider_id
  WHERE cmc.organization_id = @organization_id::uuid
    AND cmc.deleted = FALSE
  ORDER BY
    (cmc.enabled AND COALESCE(ap.enabled AND NOT ap.deleted, FALSE)) DESC,
    ap.type::text ASC,
    cmc.model ASC,
    cmc.updated_at DESC,
    cmc.id DESC
  LIMIT 1
)
RETURNING *;

-- name: GetOrganizationChatModelConfigACL :one
SELECT user_acl, group_acl
FROM chat_model_configs
WHERE id = @id::uuid
  AND organization_id = @organization_id::uuid
  AND deleted = FALSE;

-- name: UpdateOrganizationChatModelConfigACL :exec
UPDATE chat_model_configs
SET user_acl = @user_acl,
    group_acl = @group_acl,
    updated_at = NOW()
WHERE id = @id::uuid
  AND organization_id = @organization_id::uuid
  AND deleted = FALSE;

-- name: GetOrganizationChatModelConfigByLegacyID :one
SELECT *
FROM chat_model_configs
WHERE organization_id = @organization_id::uuid
  AND legacy_model_config_id = @legacy_model_config_id::uuid
  AND deleted = FALSE;

-- name: GetChatModelConfigLineageByID :one
SELECT id, organization_id, legacy_model_config_id, inherits_legacy_config
FROM chat_model_configs
WHERE id = @id::uuid;

-- name: InsertInheritedOrganizationChatModelConfigs :exec
INSERT INTO chat_model_configs (
  id, model, display_name, created_by, updated_by, enabled, is_default,
  deleted, deleted_at, created_at, updated_at, context_limit,
  compression_threshold, options, ai_provider_id, organization_id,
  user_acl, group_acl, legacy_model_config_id, inherits_legacy_config
)
SELECT gen_random_uuid(), legacy.model, legacy.display_name, legacy.created_by,
  legacy.updated_by, legacy.enabled, legacy.is_default, legacy.deleted,
  legacy.deleted_at, legacy.created_at, legacy.updated_at, legacy.context_limit,
  legacy.compression_threshold, legacy.options, legacy.ai_provider_id, org.id,
  '{}'::jsonb, jsonb_build_object(org.id::text, jsonb_build_array('read')),
  legacy.id, TRUE
FROM chat_model_configs legacy
CROSS JOIN organizations org
WHERE legacy.id = @legacy_model_config_id::uuid
  AND legacy.organization_id IS NULL
  AND org.deleted = FALSE
ON CONFLICT (organization_id, legacy_model_config_id)
  WHERE organization_id IS NOT NULL AND legacy_model_config_id IS NOT NULL
DO NOTHING;

-- name: SynchronizeInheritedOrganizationChatModelConfigs :exec
UPDATE chat_model_configs copy
SET model = legacy.model,
    display_name = legacy.display_name,
    updated_by = legacy.updated_by,
    enabled = legacy.enabled,
    context_limit = legacy.context_limit,
    compression_threshold = legacy.compression_threshold,
    options = legacy.options,
    ai_provider_id = legacy.ai_provider_id,
    updated_at = legacy.updated_at
FROM chat_model_configs legacy
WHERE legacy.id = @legacy_model_config_id::uuid
  AND legacy.organization_id IS NULL
  AND copy.legacy_model_config_id = legacy.id
  AND copy.organization_id IS NOT NULL
  AND copy.inherits_legacy_config = TRUE;

-- name: SoftDeleteInheritedOrganizationChatModelConfigs :exec
UPDATE chat_model_configs
SET inherits_legacy_config = FALSE,
    deleted = TRUE,
    deleted_at = NOW(),
    updated_at = NOW()
WHERE legacy_model_config_id = @legacy_model_config_id::uuid
  AND organization_id IS NOT NULL
  AND inherits_legacy_config = TRUE
  AND deleted = FALSE;

-- name: DetachOrganizationChatModelConfig :exec
UPDATE chat_model_configs
SET inherits_legacy_config = FALSE, updated_at = NOW()
WHERE id = @id::uuid
  AND organization_id = @organization_id::uuid
  AND inherits_legacy_config = TRUE;

-- name: GetOrganizationChatModelConfigDefaultInheritance :one
SELECT * FROM chat_model_config_org_default_inheritance
WHERE organization_id = @organization_id::uuid;

-- name: SetOrganizationChatModelConfigDefaultInheritance :exec
INSERT INTO chat_model_config_org_default_inheritance (organization_id, inherits_legacy_default)
VALUES (@organization_id::uuid, @inherits_legacy_default::boolean)
ON CONFLICT (organization_id) DO UPDATE
SET inherits_legacy_default = EXCLUDED.inherits_legacy_default;

-- name: SynchronizeInheritedOrganizationChatModelConfigDefaults :exec
WITH target_defaults AS (
  SELECT cmc.organization_id, cmc.id
  FROM chat_model_configs cmc
  JOIN chat_model_config_org_default_inheritance inheritance
    ON inheritance.organization_id = cmc.organization_id
  WHERE inheritance.inherits_legacy_default = TRUE
    AND cmc.legacy_model_config_id = @legacy_model_config_id::uuid
    AND cmc.inherits_legacy_config = TRUE
    AND cmc.deleted = FALSE
), moved_defaults AS (
  UPDATE chat_model_configs current_default
  SET is_default = FALSE, updated_at = NOW()
  FROM target_defaults target
  WHERE current_default.organization_id = target.organization_id
    AND current_default.is_default = TRUE
    AND current_default.deleted = FALSE
  RETURNING current_default.organization_id
)
UPDATE chat_model_configs target
SET is_default = TRUE, updated_at = NOW()
FROM target_defaults selected
LEFT JOIN moved_defaults moved
  ON moved.organization_id = selected.organization_id
WHERE target.id = selected.id;
