DROP TABLE IF EXISTS ai_gateway_pipeline_version_guardrails CASCADE;
DROP TABLE IF EXISTS ai_gateway_guardrail_versions CASCADE;
DROP TABLE IF EXISTS ai_gateway_guardrails CASCADE;

-- The resource_type 'ai_gateway_guardrail' enum value is intentionally left in
-- place; Postgres cannot drop an enum value, matching the policy migration.
