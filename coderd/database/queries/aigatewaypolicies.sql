-- name: GetAIGatewayPolicyByID :one
SELECT * FROM ai_gateway_policies
WHERE id = @id::uuid AND deleted = FALSE;

-- name: GetAIGatewayPolicyByName :one
SELECT * FROM ai_gateway_policies
WHERE name = @name::text AND deleted = FALSE;

-- name: GetAIGatewayPolicies :many
-- Returns policy parent rows. Soft-deleted rows are excluded unless
-- include_deleted is set.
SELECT * FROM ai_gateway_policies
WHERE
    (@include_deleted::boolean OR NOT deleted)
ORDER BY name ASC;

-- name: InsertAIGatewayPolicy :one
INSERT INTO ai_gateway_policies (
    id,
    name,
    display_name,
    kind
) VALUES (
    @id::uuid,
    @name::text,
    sqlc.narg('display_name')::text,
    @kind::ai_gateway_policy_kind
)
RETURNING *;

-- name: UpdateAIGatewayPolicy :one
UPDATE ai_gateway_policies
SET
    display_name = sqlc.narg('display_name')::text,
    updated_at = NOW()
WHERE id = @id::uuid AND deleted = FALSE
RETURNING *;

-- name: UpdateAIGatewayPolicyActiveVersion :exec
UPDATE ai_gateway_policies
SET active_version_id = @active_version_id::uuid, updated_at = NOW()
WHERE id = @id::uuid AND deleted = FALSE;

-- name: DeleteAIGatewayPolicyByID :exec
UPDATE ai_gateway_policies
SET deleted = TRUE, updated_at = NOW()
WHERE id = @id::uuid AND deleted = FALSE;

-- name: InsertAIGatewayPolicyVersion :one
INSERT INTO ai_gateway_policy_versions (
    id,
    policy_id,
    version_number,
    rego,
    input_schema_version,
    output_schema_version,
    description,
    created_by
) VALUES (
    @id::uuid,
    @policy_id::uuid,
    @version_number::integer,
    @rego::text,
    @input_schema_version::integer,
    @output_schema_version::integer,
    sqlc.narg('description')::text,
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: GetAIGatewayPolicyVersionByID :one
SELECT * FROM ai_gateway_policy_versions
WHERE id = @id::uuid;

-- name: GetAIGatewayPolicyVersionsByPolicyID :many
SELECT * FROM ai_gateway_policy_versions
WHERE policy_id = @policy_id::uuid
ORDER BY version_number DESC;

-- name: GetAIGatewayPipelineByID :one
SELECT * FROM ai_gateway_pipelines
WHERE id = @id::uuid AND deleted = FALSE;

-- name: GetAIGatewayPipelineByProviderID :one
SELECT * FROM ai_gateway_pipelines
WHERE provider_id = @provider_id::uuid AND deleted = FALSE;

-- name: GetAIGatewayPipelines :many
SELECT * FROM ai_gateway_pipelines
WHERE
    (@include_deleted::boolean OR NOT deleted)
    AND (@include_disabled::boolean OR enabled)
ORDER BY created_at ASC;

-- name: InsertAIGatewayPipeline :one
INSERT INTO ai_gateway_pipelines (
    id,
    provider_id,
    enabled
) VALUES (
    @id::uuid,
    @provider_id::uuid,
    @enabled::boolean
)
RETURNING *;

-- name: UpdateAIGatewayPipeline :one
UPDATE ai_gateway_pipelines
SET enabled = @enabled::boolean, updated_at = NOW()
WHERE id = @id::uuid AND deleted = FALSE
RETURNING *;

-- name: UpdateAIGatewayPipelineActiveVersion :exec
UPDATE ai_gateway_pipelines
SET active_version_id = @active_version_id::uuid, updated_at = NOW()
WHERE id = @id::uuid AND deleted = FALSE;

-- name: DeleteAIGatewayPipelineByID :exec
UPDATE ai_gateway_pipelines
SET deleted = TRUE, enabled = FALSE, updated_at = NOW()
WHERE id = @id::uuid AND deleted = FALSE;

-- name: InsertAIGatewayPipelineVersion :one
INSERT INTO ai_gateway_pipeline_versions (
    id,
    pipeline_id,
    version_number,
    created_by
) VALUES (
    @id::uuid,
    @pipeline_id::uuid,
    @version_number::integer,
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: InsertAIGatewayPipelineVersionPolicy :one
INSERT INTO ai_gateway_pipeline_version_policies (
    id,
    pipeline_version_id,
    policy_version_id,
    hook,
    kind,
    fail_mode,
    enabled
) VALUES (
    @id::uuid,
    @pipeline_version_id::uuid,
    @policy_version_id::uuid,
    @hook::ai_gateway_hook,
    @kind::ai_gateway_policy_kind,
    @fail_mode::ai_gateway_fail_mode,
    @enabled::boolean
)
RETURNING *;

-- name: UpdateAIGatewayPipelineVersionPolicyEnabled :exec
-- Flips a policy member's enabled flag in place on a specific pipeline version.
-- Enable/disable is a live pause control, not a composition change, so it
-- mutates the membership row directly instead of minting a new pipeline version.
UPDATE ai_gateway_pipeline_version_policies
SET enabled = @enabled::boolean
WHERE pipeline_version_id = @pipeline_version_id::uuid
    AND policy_version_id = @policy_version_id::uuid
    AND hook = @hook::ai_gateway_hook;

-- name: GetAIGatewayPipelineVersionPolicies :many
SELECT * FROM ai_gateway_pipeline_version_policies
WHERE pipeline_version_id = @pipeline_version_id::uuid;

-- name: GetAIGatewayPipelineVersionsByPipelineID :many
SELECT * FROM ai_gateway_pipeline_versions
WHERE pipeline_id = @pipeline_id::uuid
ORDER BY version_number DESC;

-- name: GetAIGatewayPipelineVersionIDByProviderAndNumber :one
-- Resolves a pipeline version's id from the provider name and the logical
-- version number, for the version-targeted evaluation gate (§10.9). The
-- X-Coder-AI-Gateway-Pipeline-Version header carries the human-facing version
-- number, not the internal uuid, so this translates it. Returns no rows when no
-- live pipeline for the provider has that version number, which the gate maps to
-- a 4xx (unknown version). Provider scoping here means a foreign version number
-- can never resolve to another provider's pipeline.
SELECT pv.id
FROM ai_gateway_pipeline_versions pv
JOIN ai_gateway_pipelines pl ON pl.id = pv.pipeline_id
JOIN ai_providers prov ON prov.id = pl.provider_id
WHERE prov.name = @provider_name::text
    AND pv.version_number = @version_number::integer
    AND pl.deleted = FALSE
    AND prov.deleted = FALSE;

-- name: GetAIGatewayPipelineVersionProvider :one
-- Resolves the provider that owns a pipeline version, for the version-targeted
-- evaluation gate (§10.9). Returns no rows if the version does not exist or its
-- pipeline/provider is soft-deleted, which the gate maps to a 4xx (unknown or
-- foreign version).
SELECT prov.name AS provider_name, pv.version_number
FROM ai_gateway_pipeline_versions pv
JOIN ai_gateway_pipelines pl ON pl.id = pv.pipeline_id
JOIN ai_providers prov ON prov.id = pl.provider_id
WHERE pv.id = @pipeline_version_id::uuid
    AND pl.deleted = FALSE
    AND prov.deleted = FALSE;

-- name: GetAIGatewayPipelineVersionPolicySnapshot :many
-- Like GetActiveAIGatewayPipelinePolicies but resolves a specific (typically
-- unpromoted) pipeline version instead of the active one, for version-targeted
-- evaluation (§10.9). The pipeline's enabled flag is intentionally NOT checked:
-- a staged version is rehearsed against real traffic before it is promoted and
-- the pipeline is enabled. Disabled members and soft-deleted parents are still
-- excluded, matching the active snapshot.
SELECT
    pl.provider_id,
    prov.name AS provider_name,
    pl.id AS pipeline_id,
    pv.id AS pipeline_version_id,
    pv.version_number AS pipeline_version_number,
    m.hook,
    m.kind,
    m.fail_mode,
    p.id AS policy_id,
    p.name AS policy_name,
    pver.id AS policy_version_id,
    pver.rego,
    pver.input_schema_version,
    pver.output_schema_version
FROM ai_gateway_pipelines pl
JOIN ai_providers prov ON prov.id = pl.provider_id
JOIN ai_gateway_pipeline_versions pv ON pv.id = @pipeline_version_id::uuid AND pv.pipeline_id = pl.id
JOIN ai_gateway_pipeline_version_policies m ON m.pipeline_version_id = pv.id
JOIN ai_gateway_policy_versions pver ON pver.id = m.policy_version_id
JOIN ai_gateway_policies p ON p.id = pver.policy_id
WHERE pl.deleted = FALSE
    AND m.enabled = TRUE
    AND prov.deleted = FALSE
    AND p.deleted = FALSE
ORDER BY pl.provider_id, m.hook, m.kind, p.name;

-- name: GetActiveAIGatewayPipelinePolicies :many
-- The runtime snapshot load: every member policy of every live, enabled
-- pipeline's active version, joined to its pinned policy version. Disabled or
-- soft-deleted policies are excluded. Ordered by name so decide ordering is
-- stable. This is a single consistent read; callers should run it on the
-- primary to avoid replica lag against the post-commit reload notification.
SELECT
    pl.provider_id,
    prov.name AS provider_name,
    pl.id AS pipeline_id,
    pv.id AS pipeline_version_id,
    pv.version_number AS pipeline_version_number,
    m.hook,
    m.kind,
    m.fail_mode,
    p.id AS policy_id,
    p.name AS policy_name,
    pver.id AS policy_version_id,
    pver.rego,
    pver.input_schema_version,
    pver.output_schema_version
FROM ai_gateway_pipelines pl
JOIN ai_providers prov ON prov.id = pl.provider_id
JOIN ai_gateway_pipeline_versions pv ON pv.id = pl.active_version_id
JOIN ai_gateway_pipeline_version_policies m ON m.pipeline_version_id = pv.id
JOIN ai_gateway_policy_versions pver ON pver.id = m.policy_version_id
JOIN ai_gateway_policies p ON p.id = pver.policy_id
WHERE pl.deleted = FALSE
    AND pl.enabled = TRUE
    AND m.enabled = TRUE
    AND prov.deleted = FALSE
    AND p.deleted = FALSE
ORDER BY pl.provider_id, m.hook, m.kind, p.name;

-- name: CountAIGatewayPolicyVersionsInActivePipelines :one
-- Used to block soft-deleting a policy whose versions are referenced by an
-- active pipeline version. The operator must first remove it from the pipeline.
SELECT COUNT(*) FROM ai_gateway_pipeline_version_policies m
JOIN ai_gateway_pipelines pl ON pl.active_version_id = m.pipeline_version_id
JOIN ai_gateway_policy_versions pver ON pver.id = m.policy_version_id
WHERE pver.policy_id = @policy_id::uuid AND pl.deleted = FALSE;

-- name: GetAIGatewayPipelinesReferencingPolicy :many
-- Live pipelines whose TIP (latest) version pins any version of the given
-- policy. Used to propagate a newly activated policy version into the pipelines
-- that use it (assisted upgrade). Referencing the tip, not the active version,
-- means a policy added to a pipeline but not yet promoted still receives the
-- propagated edit, so a policy edit always mints a pipeline version on every
-- pipeline whose current composition uses it (matching guardrails).
SELECT DISTINCT pl.*
FROM ai_gateway_pipelines pl
JOIN ai_gateway_pipeline_versions tip ON tip.pipeline_id = pl.id
    AND tip.version_number = (
        SELECT MAX(v.version_number)
        FROM ai_gateway_pipeline_versions v
        WHERE v.pipeline_id = pl.id
    )
JOIN ai_gateway_pipeline_version_policies m ON m.pipeline_version_id = tip.id
JOIN ai_gateway_policy_versions pver ON pver.id = m.policy_version_id
WHERE pl.deleted = FALSE AND pver.policy_id = @policy_id::uuid;

-- name: GetAIGatewayPipelinePolicyDrift :many
-- Membership rows whose pinned policy version is not the policy's current active
-- version. Surfaced as a drift metric.
SELECT
    pl.provider_id,
    pl.id AS pipeline_id,
    p.id AS policy_id,
    p.name AS policy_name,
    m.policy_version_id AS pinned_version_id,
    p.active_version_id AS current_version_id
FROM ai_gateway_pipelines pl
JOIN ai_gateway_pipeline_version_policies m ON m.pipeline_version_id = pl.active_version_id
JOIN ai_gateway_policy_versions pver ON pver.id = m.policy_version_id
JOIN ai_gateway_policies p ON p.id = pver.policy_id
WHERE pl.deleted = FALSE
    AND p.deleted = FALSE
    AND (p.active_version_id IS NULL OR m.policy_version_id != p.active_version_id);
