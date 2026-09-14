DROP TABLE IF EXISTS ai_provider_keys;
DROP TABLE IF EXISTS ai_providers;
DROP TYPE IF EXISTS ai_provider_type;
-- No-op for ALTER TYPE resource_type / api_key_scope ADD VALUE:
-- Postgres does not allow removing enum values safely.
