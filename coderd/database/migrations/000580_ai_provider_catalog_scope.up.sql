-- Low-level API key scopes for the new ai_provider_catalog RBAC
-- resource (read-only, non-secret provider catalog). Internal-only:
-- intentionally not exposed in the external scope catalog.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_provider_catalog:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_provider_catalog:read';
