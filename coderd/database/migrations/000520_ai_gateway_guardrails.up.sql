-- Guardrails are the networked head-of-hook stage of the policy engine. Unlike
-- a Rego policy they are config (not code) and may carry a secret credential, so
-- they live in their own tables rather than the policy tables. They mirror the
-- policy/version + active_version_id atomic-swap pattern.

-- ai_gateway_guardrails is the reusable parent for a versioned guardrail. The
-- adapter_type selects the wire adapter (e.g. 'presidio', 'generic_api').
CREATE TABLE ai_gateway_guardrails (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name              text NOT NULL CHECK (name ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    display_name      text,
    adapter_type      text NOT NULL,
    active_version_id uuid,
    enabled           boolean NOT NULL DEFAULT TRUE,
    deleted           boolean NOT NULL DEFAULT FALSE,
    created_at        timestamptz NOT NULL DEFAULT NOW(),
    updated_at        timestamptz NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX ai_gateway_guardrails_name_unique
    ON ai_gateway_guardrails (name) WHERE deleted = FALSE;

-- ai_gateway_guardrail_versions are immutable config snapshots. config holds the
-- adapter parameters (endpoints, entity actions, thresholds) as JSON. credential
-- holds the dbcrypt-encrypted secret (e.g. an API key); credential_key_id names
-- the encryption key (empty string when there is no secret).
CREATE TABLE ai_gateway_guardrail_versions (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    guardrail_id      uuid NOT NULL REFERENCES ai_gateway_guardrails (id) ON DELETE CASCADE,
    version_number    integer NOT NULL,
    config            jsonb NOT NULL,
    credential        text NOT NULL DEFAULT '',
    -- dbcrypt encryption key id for credential; NULL when the secret is stored
    -- in plaintext (no encryption configured) or there is no secret.
    credential_key_id text,
    description       text,
    created_at        timestamptz NOT NULL DEFAULT NOW(),
    created_by        uuid REFERENCES users (id) ON DELETE SET NULL,
    UNIQUE (guardrail_id, version_number),
    -- Composite target so active_version_id can be FK-bound to its own guardrail.
    UNIQUE (guardrail_id, id)
);

-- Composite FK: active_version_id must belong to THIS guardrail. NULL is allowed
-- (MATCH SIMPLE) until the first version is minted.
ALTER TABLE ai_gateway_guardrails
    ADD CONSTRAINT ai_gateway_guardrails_active_version_id_fkey
    FOREIGN KEY (id, active_version_id)
    REFERENCES ai_gateway_guardrail_versions (guardrail_id, id);

-- ai_gateway_pipeline_version_guardrails is the guardrail membership of a
-- pipeline version. It is separate from the policy membership table: guardrails
-- have no kind and no per-kind cardinality cap (many concurrent guardrails per
-- hook are allowed). Rows are immutable; composition changes mint a new pipeline
-- version. A guardrail's authority is intrinsic to its adapter output (block /
-- mask / annotate), so there is no per-membership mode.
CREATE TABLE ai_gateway_pipeline_version_guardrails (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_version_id  uuid NOT NULL REFERENCES ai_gateway_pipeline_versions (id) ON DELETE CASCADE,
    guardrail_version_id uuid NOT NULL REFERENCES ai_gateway_guardrail_versions (id),
    hook                 ai_gateway_hook NOT NULL,
    fail_mode            ai_gateway_fail_mode NOT NULL DEFAULT 'fail_closed',
    network_timeout_ms   integer NOT NULL DEFAULT 2000,
    enabled              boolean NOT NULL DEFAULT TRUE,
    -- position orders guardrails within a (version, hook): guardrails run as a
    -- sequential chain in ascending position, each seeing the body as rewritten
    -- by the prior one, so maskers compose deterministically instead of racing.
    -- Set from membership order at create and preserved across version mints.
    position             integer NOT NULL DEFAULT 0,
    -- A guardrail appears at most once per hook in a given version.
    UNIQUE (pipeline_version_id, guardrail_version_id, hook)
);

-- Audit support: allow ai_gateway_guardrails to appear in audit_log.resource_type.
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'ai_gateway_guardrail';

COMMENT ON TABLE ai_gateway_guardrails IS 'Reusable, versioned networked guardrails for the AI gateway policy engine.';
COMMENT ON TABLE ai_gateway_guardrail_versions IS 'Immutable adapter config + dbcrypt-encrypted credential for each guardrail version.';
COMMENT ON TABLE ai_gateway_pipeline_version_guardrails IS 'Guardrail membership of a pipeline version: which guardrail versions run at which hook.';
