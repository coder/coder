-- name: GetAIProviderByID :one
SELECT
    *
FROM
    ai_providers
WHERE
    id = @id::uuid AND deleted = FALSE;

-- name: GetAIProviderByIDForReferenceLock :one
SELECT
    *
FROM
    ai_providers
WHERE
    id = @id::uuid AND deleted = FALSE
-- Lock the provider row until the model-config write completes. The
-- transaction alone does not stop a concurrent soft-delete or disable
-- between validation and writing the model config reference.
FOR SHARE;

-- name: GetAIProviderByName :one
SELECT
    *
FROM
    ai_providers
WHERE
    name = @name::text AND deleted = FALSE;

-- name: GetAIProviders :many
-- Returns AI provider rows. Soft-deleted and disabled rows are excluded
-- unless include_deleted or include_disabled is set.
SELECT
    *
FROM
    ai_providers
WHERE
    (@include_deleted::boolean OR NOT deleted)
    AND (@include_disabled::boolean OR enabled)
ORDER BY
    name ASC;

-- name: InsertAIProvider :one
INSERT INTO ai_providers (
    id,
    type,
    name,
    display_name,
    icon,
    enabled,
    base_url,
    settings,
    settings_key_id
) VALUES (
    @id::uuid,
    @type::ai_provider_type,
    @name::text,
    sqlc.narg('display_name')::text,
    @icon::text,
    @enabled::boolean,
    @base_url::text,
    sqlc.narg('settings')::text,
    sqlc.narg('settings_key_id')::text
)
RETURNING
    *;

-- name: UpdateAIProvider :one
UPDATE
    ai_providers
SET
    type = @type::ai_provider_type,
    display_name = sqlc.narg('display_name')::text,
    icon = @icon::text,
    enabled = @enabled::boolean,
    base_url = @base_url::text,
    settings = sqlc.narg('settings')::text,
    settings_key_id = sqlc.narg('settings_key_id')::text,
    updated_at = NOW()
WHERE
    id = @id::uuid AND deleted = FALSE
RETURNING
    *;

-- name: DeleteAIProviderByID :exec
UPDATE
    ai_providers
SET
    deleted = TRUE,
    enabled = FALSE,
    updated_at = NOW()
WHERE
    id = @id::uuid AND deleted = FALSE;

-- name: UpdateEncryptedAIProviderSettings :one
-- Updates only the encrypted columns (settings, settings_key_id) and
-- the updated_at timestamp on a row, regardless of its deleted flag.
-- Used by the dbcrypt key rotation utility to re-encrypt or decrypt
-- rows in place.
UPDATE
    ai_providers
SET
    settings = sqlc.narg('settings')::text,
    settings_key_id = sqlc.narg('settings_key_id')::text,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: UpsertAIBedrockInferenceProfileModel :exec
-- Records the model an application inference profile ARN resolves to. The
-- provider write path resolves the ARN through the Bedrock control plane and
-- stores the answer here, so the gateway never has to. An upsert rather than
-- an insert so a later save corrects a stored value.
INSERT INTO
    ai_bedrock_inference_profile_models (inference_profile_arn, resolved_model)
VALUES
    (@inference_profile_arn::text, @resolved_model::text)
ON CONFLICT (inference_profile_arn) DO UPDATE SET
    resolved_model = @resolved_model::text;

-- name: GetAIBedrockInferenceProfileModels :many
SELECT
    *
FROM
    ai_bedrock_inference_profile_models
WHERE
    inference_profile_arn = ANY(@inference_profile_arns::text[]);
