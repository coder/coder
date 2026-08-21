-- name: GetChatOrganizationModelOverrides :many
SELECT id, organization_id, context, model_config_id, reasoning_effort
FROM chat_organization_model_overrides
WHERE organization_id = @organization_id
ORDER BY context;

-- name: GetChatOrganizationModelOverride :one
SELECT id, organization_id, context, model_config_id, reasoning_effort
FROM chat_organization_model_overrides
WHERE organization_id = @organization_id
  AND context = @context;

-- name: UpsertChatOrganizationModelOverride :exec
INSERT INTO chat_organization_model_overrides
    (organization_id, context, model_config_id, reasoning_effort)
VALUES (@organization_id, @context, @model_config_id, @reasoning_effort)
ON CONFLICT ON CONSTRAINT chat_organization_model_overrides_organization_id_context_key
DO UPDATE SET
    model_config_id = EXCLUDED.model_config_id,
    reasoning_effort = EXCLUDED.reasoning_effort;

-- name: DeleteChatOrganizationModelOverride :exec
DELETE FROM chat_organization_model_overrides
WHERE organization_id = @organization_id
  AND context = @context;

-- name: GetChatUserModelOverrides :many
SELECT id, user_id, organization_id, context, mode, model_config_id, reasoning_effort
FROM chat_user_model_overrides
WHERE user_id = @user_id
  AND organization_id = @organization_id
ORDER BY context;

-- name: GetChatUserModelOverride :one
SELECT id, user_id, organization_id, context, mode, model_config_id, reasoning_effort
FROM chat_user_model_overrides
WHERE user_id = @user_id
  AND organization_id = @organization_id
  AND context = @context;

-- name: UpsertChatUserModelOverride :exec
INSERT INTO chat_user_model_overrides
    (user_id, organization_id, context, mode, model_config_id, reasoning_effort)
VALUES (@user_id, @organization_id, @context, @mode, @model_config_id, @reasoning_effort)
ON CONFLICT ON CONSTRAINT chat_user_model_overrides_user_organization_context_key
DO UPDATE SET
    mode = EXCLUDED.mode,
    model_config_id = EXCLUDED.model_config_id,
    reasoning_effort = EXCLUDED.reasoning_effort;

-- Compatibility methods keep the current runtime compiling until it adopts
-- the organization-aware query surface. They read and write the default org.

-- name: GetChatGeneralModelOverride :one
SELECT COALESCE((
    SELECT model_config_id::text || COALESCE(':' || reasoning_effort, '')
    FROM chat_organization_model_overrides omo
    JOIN organizations o ON o.id = omo.organization_id
    WHERE o.is_default AND omo.context = 'general'
), '')::text AS model_config_id;

-- name: GetChatExploreModelOverride :one
SELECT COALESCE((
    SELECT model_config_id::text || COALESCE(':' || reasoning_effort, '')
    FROM chat_organization_model_overrides omo
    JOIN organizations o ON o.id = omo.organization_id
    WHERE o.is_default AND omo.context = 'explore'
), '')::text AS model_config_id;

-- name: GetChatTitleGenerationModelOverride :one
SELECT COALESCE((
    SELECT model_config_id::text || COALESCE(':' || reasoning_effort, '')
    FROM chat_organization_model_overrides omo
    JOIN organizations o ON o.id = omo.organization_id
    WHERE o.is_default AND omo.context = 'title_generation'
), '')::text AS model_config_id;

-- name: GetChatCompactionModelOverride :one
SELECT COALESCE((
    SELECT model_config_id::text || COALESCE(':' || reasoning_effort, '')
    FROM chat_organization_model_overrides omo
    JOIN organizations o ON o.id = omo.organization_id
    WHERE o.is_default AND omo.context = 'compaction'
), '')::text AS model_config_id;

-- name: UpsertChatGeneralModelOverride :exec
WITH input AS (
    SELECT
        o.id AS organization_id,
        CASE WHEN trim(@value::text) = '' THEN NULL ELSE split_part(trim(@value::text), ':', 1)::uuid END AS model_config_id,
        NULLIF(split_part(trim(@value::text), ':', 2), '') AS reasoning_effort
    FROM organizations o
    WHERE o.is_default
), deleted AS (
    DELETE FROM chat_organization_model_overrides omo
    USING input i
    WHERE omo.organization_id = i.organization_id
      AND omo.context = 'general'
      AND i.model_config_id IS NULL
)
INSERT INTO chat_organization_model_overrides (organization_id, context, model_config_id, reasoning_effort)
SELECT organization_id, 'general', model_config_id, reasoning_effort
FROM input
WHERE model_config_id IS NOT NULL
ON CONFLICT ON CONSTRAINT chat_organization_model_overrides_organization_id_context_key
DO UPDATE SET model_config_id = EXCLUDED.model_config_id, reasoning_effort = EXCLUDED.reasoning_effort;

-- name: UpsertChatExploreModelOverride :exec
WITH input AS (
    SELECT
        o.id AS organization_id,
        CASE WHEN trim(@value::text) = '' THEN NULL ELSE split_part(trim(@value::text), ':', 1)::uuid END AS model_config_id,
        NULLIF(split_part(trim(@value::text), ':', 2), '') AS reasoning_effort
    FROM organizations o
    WHERE o.is_default
), deleted AS (
    DELETE FROM chat_organization_model_overrides omo
    USING input i
    WHERE omo.organization_id = i.organization_id
      AND omo.context = 'explore'
      AND i.model_config_id IS NULL
)
INSERT INTO chat_organization_model_overrides (organization_id, context, model_config_id, reasoning_effort)
SELECT organization_id, 'explore', model_config_id, reasoning_effort
FROM input
WHERE model_config_id IS NOT NULL
ON CONFLICT ON CONSTRAINT chat_organization_model_overrides_organization_id_context_key
DO UPDATE SET model_config_id = EXCLUDED.model_config_id, reasoning_effort = EXCLUDED.reasoning_effort;

-- name: UpsertChatTitleGenerationModelOverride :exec
WITH input AS (
    SELECT
        o.id AS organization_id,
        CASE WHEN trim(@value::text) = '' THEN NULL ELSE split_part(trim(@value::text), ':', 1)::uuid END AS model_config_id,
        NULLIF(split_part(trim(@value::text), ':', 2), '') AS reasoning_effort
    FROM organizations o
    WHERE o.is_default
), deleted AS (
    DELETE FROM chat_organization_model_overrides omo
    USING input i
    WHERE omo.organization_id = i.organization_id
      AND omo.context = 'title_generation'
      AND i.model_config_id IS NULL
)
INSERT INTO chat_organization_model_overrides (organization_id, context, model_config_id, reasoning_effort)
SELECT organization_id, 'title_generation', model_config_id, reasoning_effort
FROM input
WHERE model_config_id IS NOT NULL
ON CONFLICT ON CONSTRAINT chat_organization_model_overrides_organization_id_context_key
DO UPDATE SET model_config_id = EXCLUDED.model_config_id, reasoning_effort = EXCLUDED.reasoning_effort;

-- name: UpsertChatCompactionModelOverride :exec
WITH input AS (
    SELECT
        o.id AS organization_id,
        CASE WHEN trim(@value::text) = '' THEN NULL ELSE split_part(trim(@value::text), ':', 1)::uuid END AS model_config_id,
        NULLIF(split_part(trim(@value::text), ':', 2), '') AS reasoning_effort
    FROM organizations o
    WHERE o.is_default
), deleted AS (
    DELETE FROM chat_organization_model_overrides omo
    USING input i
    WHERE omo.organization_id = i.organization_id
      AND omo.context = 'compaction'
      AND i.model_config_id IS NULL
)
INSERT INTO chat_organization_model_overrides (organization_id, context, model_config_id, reasoning_effort)
SELECT organization_id, 'compaction', model_config_id, reasoning_effort
FROM input
WHERE model_config_id IS NOT NULL
ON CONFLICT ON CONSTRAINT chat_organization_model_overrides_organization_id_context_key
DO UPDATE SET model_config_id = EXCLUDED.model_config_id, reasoning_effort = EXCLUDED.reasoning_effort;

-- name: ListUserChatPersonalModelOverrides :many
SELECT
    ('chat_personal_model_override:' || umo.context)::text AS key,
    (CASE
        WHEN umo.mode IN ('chat_default', 'deployment_default') THEN umo.mode
        ELSE 'model:' || umo.model_config_id::text || COALESCE(':' || umo.reasoning_effort, '')
    END)::text AS value
FROM chat_user_model_overrides umo
JOIN organizations o ON o.id = umo.organization_id
WHERE umo.user_id = @user_id
  AND o.is_default
ORDER BY key;

-- name: GetUserChatPersonalModelOverride :one
SELECT (CASE
    WHEN umo.mode IN ('chat_default', 'deployment_default') THEN umo.mode
    ELSE 'model:' || umo.model_config_id::text || COALESCE(':' || umo.reasoning_effort, '')
END)::text AS personal_model_override
FROM chat_user_model_overrides umo
JOIN organizations o ON o.id = umo.organization_id
WHERE umo.user_id = @user_id
  AND o.is_default
  AND umo.context = substring(@key::text FROM 'chat_personal_model_override:(.*)');

-- name: UpsertUserChatPersonalModelOverride :exec
WITH input AS (
    SELECT
        @user_id::uuid AS user_id,
        o.id AS organization_id,
        substring(@key::text FROM 'chat_personal_model_override:(.*)') AS context,
        trim(@value::text) AS value,
        CASE
            WHEN split_part(trim(@value::text), ':', 1) = 'model'
                THEN split_part(trim(@value::text), ':', 2)::uuid
            ELSE NULL
        END AS model_config_id
    FROM organizations o
    WHERE o.is_default
), deleted AS (
    DELETE FROM chat_user_model_overrides umo
    USING input i
    WHERE umo.user_id = i.user_id
      AND umo.organization_id = i.organization_id
      AND umo.context = i.context
      AND i.value = ''
)
INSERT INTO chat_user_model_overrides
    (user_id, organization_id, context, mode, model_config_id, reasoning_effort)
SELECT
    user_id,
    organization_id,
    context,
    CASE WHEN value IN ('chat_default', 'deployment_default') THEN value ELSE 'model' END,
    model_config_id,
    CASE WHEN split_part(value, ':', 1) = 'model' THEN NULLIF(split_part(value, ':', 3), '') ELSE NULL END
FROM input
WHERE value != ''
ON CONFLICT ON CONSTRAINT chat_user_model_overrides_user_organization_context_key
DO UPDATE SET
    mode = EXCLUDED.mode,
    model_config_id = EXCLUDED.model_config_id,
    reasoning_effort = EXCLUDED.reasoning_effort;
