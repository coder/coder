DROP TRIGGER IF EXISTS trigger_workspace_secrets_per_build_limits ON workspace_secrets;
DROP FUNCTION IF EXISTS enforce_workspace_secrets_per_build_limits();
DROP TABLE IF EXISTS workspace_secrets;
DROP TYPE IF EXISTS workspace_secret_source;
-- The workspace_secret api_key_scope values are left in place; enum values
-- cannot be dropped.
