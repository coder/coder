-- Per-membership enable flag so a policy can be disabled within a specific
-- pipeline without disabling it globally. Disabling is expressed by a new
-- pipeline version whose member row has enabled = FALSE; disabled members are
-- excluded from the runtime snapshot.
ALTER TABLE ai_gateway_pipeline_version_policies
    ADD COLUMN enabled boolean NOT NULL DEFAULT TRUE;
