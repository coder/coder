-- Enum additions to resource_type and api_key_scope are intentionally not
-- reverted because Postgres cannot drop enum values safely.
DROP INDEX IF EXISTS ai_gateway_keys_hashed_secret_idx;
DROP INDEX IF EXISTS ai_gateway_keys_secret_prefix_idx;
DROP INDEX IF EXISTS ai_gateway_keys_name_idx;
DROP TABLE IF EXISTS ai_gateway_keys;
