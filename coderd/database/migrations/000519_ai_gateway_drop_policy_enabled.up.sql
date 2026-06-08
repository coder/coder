-- A policy is a reusable definition; "enabled" only has meaning for a policy
-- within a pipeline. Enable/disable is expressed per pipeline+policy tuple via
-- ai_gateway_pipeline_version_policies.enabled, so the global flag is dropped.
ALTER TABLE ai_gateway_policies DROP COLUMN enabled;
