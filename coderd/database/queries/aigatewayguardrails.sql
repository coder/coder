-- name: GetAIGatewayGuardrailByID :one
SELECT * FROM ai_gateway_guardrails
WHERE id = @id::uuid AND deleted = FALSE;

-- name: GetAIGatewayGuardrailByName :one
SELECT * FROM ai_gateway_guardrails
WHERE name = @name::text AND deleted = FALSE;

-- name: GetAIGatewayGuardrails :many
SELECT * FROM ai_gateway_guardrails
WHERE
    (@include_deleted::boolean OR NOT deleted)
ORDER BY name ASC;

-- name: InsertAIGatewayGuardrail :one
INSERT INTO ai_gateway_guardrails (
    id,
    name,
    display_name,
    adapter_type
) VALUES (
    @id::uuid,
    @name::text,
    sqlc.narg('display_name')::text,
    @adapter_type::text
)
RETURNING *;

-- name: UpdateAIGatewayGuardrail :one
UPDATE ai_gateway_guardrails
SET
    display_name = sqlc.narg('display_name')::text,
    enabled = @enabled::boolean,
    updated_at = NOW()
WHERE id = @id::uuid AND deleted = FALSE
RETURNING *;

-- name: UpdateAIGatewayGuardrailActiveVersion :exec
UPDATE ai_gateway_guardrails
SET active_version_id = @active_version_id::uuid, updated_at = NOW()
WHERE id = @id::uuid AND deleted = FALSE;

-- name: DeleteAIGatewayGuardrailByID :exec
UPDATE ai_gateway_guardrails
SET deleted = TRUE, enabled = FALSE, updated_at = NOW()
WHERE id = @id::uuid AND deleted = FALSE;

-- name: InsertAIGatewayGuardrailVersion :one
INSERT INTO ai_gateway_guardrail_versions (
    id,
    guardrail_id,
    version_number,
    config,
    credential,
    credential_key_id,
    description,
    created_by
) VALUES (
    @id::uuid,
    @guardrail_id::uuid,
    @version_number::integer,
    @config::jsonb,
    @credential::text,
    sqlc.narg('credential_key_id')::text,
    sqlc.narg('description')::text,
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: GetAIGatewayGuardrailVersionByID :one
SELECT * FROM ai_gateway_guardrail_versions
WHERE id = @id::uuid;

-- name: GetAIGatewayGuardrailVersionsByGuardrailID :many
SELECT * FROM ai_gateway_guardrail_versions
WHERE guardrail_id = @guardrail_id::uuid
ORDER BY version_number DESC;

-- name: InsertAIGatewayPipelineVersionGuardrail :one
INSERT INTO ai_gateway_pipeline_version_guardrails (
    id,
    pipeline_version_id,
    guardrail_version_id,
    hook,
    mode,
    fail_mode,
    network_timeout_ms,
    enabled
) VALUES (
    @id::uuid,
    @pipeline_version_id::uuid,
    @guardrail_version_id::uuid,
    @hook::ai_gateway_hook,
    @mode::ai_gateway_guardrail_mode,
    @fail_mode::ai_gateway_fail_mode,
    @network_timeout_ms::integer,
    @enabled::boolean
)
RETURNING *;

-- name: UpdateAIGatewayPipelineVersionGuardrailEnabled :exec
-- Flips a guardrail member's enabled flag in place on a specific pipeline
-- version. Like the policy variant, enable/disable is a live pause control, not
-- a composition change, so it mutates the membership row instead of minting a
-- new pipeline version.
UPDATE ai_gateway_pipeline_version_guardrails
SET enabled = @enabled::boolean
WHERE pipeline_version_id = @pipeline_version_id::uuid
    AND guardrail_version_id = @guardrail_version_id::uuid
    AND hook = @hook::ai_gateway_hook;

-- name: GetAIGatewayPipelineVersionGuardrails :many
SELECT * FROM ai_gateway_pipeline_version_guardrails
WHERE pipeline_version_id = @pipeline_version_id::uuid;

-- name: GetAIGatewayPipelineVersionGuardrailSnapshot :many
-- Like GetActiveAIGatewayPipelineGuardrails but resolves a specific (typically
-- unpromoted) pipeline version instead of the active one, for version-targeted
-- evaluation (§10.9). The pipeline's enabled flag is intentionally NOT checked,
-- mirroring the policy snapshot variant. credential is decrypted by the dbcrypt
-- layer.
SELECT
    pl.provider_id,
    prov.name AS provider_name,
    pl.id AS pipeline_id,
    pv.id AS pipeline_version_id,
    pv.version_number AS pipeline_version_number,
    m.hook,
    m.mode,
    m.fail_mode,
    m.network_timeout_ms,
    g.id AS guardrail_id,
    g.name AS guardrail_name,
    g.adapter_type,
    gver.id AS guardrail_version_id,
    gver.config,
    gver.credential,
    gver.credential_key_id
FROM ai_gateway_pipelines pl
JOIN ai_providers prov ON prov.id = pl.provider_id
JOIN ai_gateway_pipeline_versions pv ON pv.id = @pipeline_version_id::uuid AND pv.pipeline_id = pl.id
JOIN ai_gateway_pipeline_version_guardrails m ON m.pipeline_version_id = pv.id
JOIN ai_gateway_guardrail_versions gver ON gver.id = m.guardrail_version_id
JOIN ai_gateway_guardrails g ON g.id = gver.guardrail_id
WHERE pl.deleted = FALSE
    AND m.enabled = TRUE
    AND g.enabled = TRUE
    AND prov.deleted = FALSE
    AND g.deleted = FALSE
ORDER BY pl.provider_id, m.hook, g.name;

-- name: GetActiveAIGatewayPipelineGuardrails :many
-- The runtime snapshot load for guardrails: every guardrail member of every
-- live, enabled pipeline's active version, joined to its pinned guardrail
-- version. Disabled or soft-deleted guardrails are excluded. credential is
-- decrypted by the dbcrypt layer. Run on the primary to avoid replica lag
-- against the post-commit reload notification.
SELECT
    pl.provider_id,
    prov.name AS provider_name,
    pl.id AS pipeline_id,
    pv.id AS pipeline_version_id,
    pv.version_number AS pipeline_version_number,
    m.hook,
    m.mode,
    m.fail_mode,
    m.network_timeout_ms,
    g.id AS guardrail_id,
    g.name AS guardrail_name,
    g.adapter_type,
    gver.id AS guardrail_version_id,
    gver.config,
    gver.credential,
    gver.credential_key_id
FROM ai_gateway_pipelines pl
JOIN ai_providers prov ON prov.id = pl.provider_id
JOIN ai_gateway_pipeline_versions pv ON pv.id = pl.active_version_id
JOIN ai_gateway_pipeline_version_guardrails m ON m.pipeline_version_id = pv.id
JOIN ai_gateway_guardrail_versions gver ON gver.id = m.guardrail_version_id
JOIN ai_gateway_guardrails g ON g.id = gver.guardrail_id
WHERE pl.deleted = FALSE
    AND pl.enabled = TRUE
    AND m.enabled = TRUE
    AND g.enabled = TRUE
    AND prov.deleted = FALSE
    AND g.deleted = FALSE
ORDER BY pl.provider_id, m.hook, g.name;

-- name: CountAIGatewayGuardrailVersionsInActivePipelines :one
-- Used to block soft-deleting a guardrail whose versions are referenced by an
-- active pipeline version. The operator must first remove it from the pipeline.
SELECT COUNT(*) FROM ai_gateway_pipeline_version_guardrails m
JOIN ai_gateway_pipelines pl ON pl.active_version_id = m.pipeline_version_id
JOIN ai_gateway_guardrail_versions gver ON gver.id = m.guardrail_version_id
WHERE gver.guardrail_id = @guardrail_id::uuid AND pl.deleted = FALSE;

-- name: GetAIGatewayPipelinesReferencingGuardrail :many
-- Live pipelines whose TIP (latest) version pins any version of the given
-- guardrail. Used to propagate a newly activated guardrail version into the
-- pipelines that use it (assisted upgrade). Referencing the tip, not the active
-- version, means a guardrail added to a pipeline but not yet promoted still
-- receives the propagated edit, so a guardrail edit always mints a pipeline
-- version (matching policies).
SELECT DISTINCT pl.*
FROM ai_gateway_pipelines pl
JOIN ai_gateway_pipeline_versions tip ON tip.pipeline_id = pl.id
    AND tip.version_number = (
        SELECT MAX(v.version_number)
        FROM ai_gateway_pipeline_versions v
        WHERE v.pipeline_id = pl.id
    )
JOIN ai_gateway_pipeline_version_guardrails m ON m.pipeline_version_id = tip.id
JOIN ai_gateway_guardrail_versions gver ON gver.id = m.guardrail_version_id
WHERE pl.deleted = FALSE AND gver.guardrail_id = @guardrail_id::uuid;

-- name: GetAIGatewayPipelineGuardrailDrift :many
-- Guardrail membership rows whose pinned guardrail version is not the
-- guardrail's current active version. Surfaced as a drift metric.
SELECT
    pl.provider_id,
    pl.id AS pipeline_id,
    g.id AS guardrail_id,
    g.name AS guardrail_name,
    m.guardrail_version_id AS pinned_version_id,
    g.active_version_id AS current_version_id
FROM ai_gateway_pipelines pl
JOIN ai_gateway_pipeline_version_guardrails m ON m.pipeline_version_id = pl.active_version_id
JOIN ai_gateway_guardrail_versions gver ON gver.id = m.guardrail_version_id
JOIN ai_gateway_guardrails g ON g.id = gver.guardrail_id
WHERE pl.deleted = FALSE
    AND g.deleted = FALSE
    AND (g.active_version_id IS NULL OR m.guardrail_version_id != g.active_version_id);
