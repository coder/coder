CREATE TYPE ai_gateway_policy_kind AS ENUM ('annotate', 'route', 'decide', 'transform');

-- v1 hooks only; 'post_resp' is added in a later phase via ALTER TYPE ... ADD VALUE.
CREATE TYPE ai_gateway_hook AS ENUM ('pre_auth', 'pre_req');

CREATE TYPE ai_gateway_fail_mode AS ENUM ('fail_open', 'fail_closed');

-- ai_gateway_policies is the reusable parent for a versioned Rego policy. The
-- kind is intrinsic to the policy (which entrypoint rule its Rego binds) and is
-- immutable across versions.
CREATE TABLE ai_gateway_policies (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name              text NOT NULL CHECK (name ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    display_name      text,
    kind              ai_gateway_policy_kind NOT NULL,
    active_version_id uuid,
    enabled           boolean NOT NULL DEFAULT TRUE,
    deleted           boolean NOT NULL DEFAULT FALSE,
    created_at        timestamptz NOT NULL DEFAULT NOW(),
    updated_at        timestamptz NOT NULL DEFAULT NOW()
);

-- Unique among live rows only; soft-delete preserves audit/FK history.
CREATE UNIQUE INDEX ai_gateway_policies_name_unique
    ON ai_gateway_policies (name) WHERE deleted = FALSE;

-- ai_gateway_policy_versions are immutable snapshots of a policy's Rego text and
-- its frozen schema bindings. Edits insert a new version; rows are never mutated.
CREATE TABLE ai_gateway_policy_versions (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_id             uuid NOT NULL REFERENCES ai_gateway_policies (id) ON DELETE CASCADE,
    version_number        integer NOT NULL,
    rego                  text NOT NULL,
    input_schema_version  integer NOT NULL,
    output_schema_version integer NOT NULL,
    description           text,
    created_at            timestamptz NOT NULL DEFAULT NOW(),
    created_by            uuid REFERENCES users (id) ON DELETE SET NULL,
    UNIQUE (policy_id, version_number),
    -- Composite target so active_version_id can be FK-bound to its own policy.
    UNIQUE (policy_id, id)
);

-- Composite FK: active_version_id must belong to THIS policy. NULL is allowed
-- (MATCH SIMPLE) until the first version is minted.
ALTER TABLE ai_gateway_policies
    ADD CONSTRAINT ai_gateway_policies_active_version_id_fkey
    FOREIGN KEY (id, active_version_id)
    REFERENCES ai_gateway_policy_versions (policy_id, id);

-- ai_gateway_pipelines attaches at most one pipeline to a provider. The
-- active_version_id is the atomic, non-disruptive swap point.
CREATE TABLE ai_gateway_pipelines (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id       uuid NOT NULL REFERENCES ai_providers (id) ON DELETE CASCADE,
    active_version_id uuid,
    enabled           boolean NOT NULL DEFAULT TRUE,
    deleted           boolean NOT NULL DEFAULT FALSE,
    created_at        timestamptz NOT NULL DEFAULT NOW(),
    updated_at        timestamptz NOT NULL DEFAULT NOW()
);

-- At most one live pipeline per provider.
CREATE UNIQUE INDEX ai_gateway_pipelines_provider_unique
    ON ai_gateway_pipelines (provider_id) WHERE deleted = FALSE;

-- ai_gateway_pipeline_versions are immutable composition snapshots; they are the
-- pipeline's version history.
CREATE TABLE ai_gateway_pipeline_versions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_id    uuid NOT NULL REFERENCES ai_gateway_pipelines (id) ON DELETE CASCADE,
    version_number integer NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT NOW(),
    created_by     uuid REFERENCES users (id) ON DELETE SET NULL,
    UNIQUE (pipeline_id, version_number),
    UNIQUE (pipeline_id, id)
);

ALTER TABLE ai_gateway_pipelines
    ADD CONSTRAINT ai_gateway_pipelines_active_version_id_fkey
    FOREIGN KEY (id, active_version_id)
    REFERENCES ai_gateway_pipeline_versions (pipeline_id, id);

-- ai_gateway_pipeline_version_policies is the membership of a pipeline version.
-- Rows are immutable (composition changes mint a new pipeline version). kind is
-- denormalized from ai_gateway_policies so the per-hook cardinality constraints
-- below can be partial unique indexes (Postgres cannot index across a join);
-- this is safe because kind is immutable on a policy.
CREATE TABLE ai_gateway_pipeline_version_policies (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_version_id uuid NOT NULL REFERENCES ai_gateway_pipeline_versions (id) ON DELETE CASCADE,
    policy_version_id   uuid NOT NULL REFERENCES ai_gateway_policy_versions (id),
    hook                ai_gateway_hook NOT NULL,
    kind                ai_gateway_policy_kind NOT NULL,
    fail_mode           ai_gateway_fail_mode NOT NULL DEFAULT 'fail_closed',
    -- A policy appears at most once per hook in a given version.
    UNIQUE (pipeline_version_id, policy_version_id, hook)
);

-- At most one annotate / route / transform per (version, hook). decide is
-- unconstrained (many, reduced).
CREATE UNIQUE INDEX ai_gateway_pvp_one_annotate
    ON ai_gateway_pipeline_version_policies (pipeline_version_id, hook) WHERE kind = 'annotate';
CREATE UNIQUE INDEX ai_gateway_pvp_one_route
    ON ai_gateway_pipeline_version_policies (pipeline_version_id, hook) WHERE kind = 'route';
CREATE UNIQUE INDEX ai_gateway_pvp_one_transform
    ON ai_gateway_pipeline_version_policies (pipeline_version_id, hook) WHERE kind = 'transform';

-- Audit support: allow ai_gateway_policies and ai_gateway_pipelines to appear in
-- audit_log.resource_type.
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'ai_gateway_policy';
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'ai_gateway_pipeline';

COMMENT ON TABLE ai_gateway_policies IS 'Reusable, versioned Rego policies for the AI gateway policy engine.';
COMMENT ON TABLE ai_gateway_policy_versions IS 'Immutable Rego text + frozen schema bindings for each policy version.';
COMMENT ON TABLE ai_gateway_pipelines IS 'At most one policy pipeline per AI provider; active_version_id is the atomic swap point.';
COMMENT ON TABLE ai_gateway_pipeline_versions IS 'Immutable composition snapshots forming a pipeline''s version history.';
COMMENT ON TABLE ai_gateway_pipeline_version_policies IS 'Membership of a pipeline version: which policy versions run at which hook.';
