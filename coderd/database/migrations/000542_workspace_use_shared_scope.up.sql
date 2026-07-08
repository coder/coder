-- Add low-level API key scopes for the new workspace use_shared action.
-- These scopes are internal-only and not exposed in the external scope
-- catalog; they exist so the api_key_scope enum stays in sync with the
-- RBAC policy actions.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace:use_shared';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_dormant:use_shared';
