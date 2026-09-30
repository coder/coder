DROP TRIGGER IF EXISTS trigger_workspace_secrets_per_workspace_limits ON workspace_secrets;
DROP FUNCTION IF EXISTS enforce_workspace_secrets_per_workspace_limits();
DROP TABLE IF EXISTS workspace_secrets;
