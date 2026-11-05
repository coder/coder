DROP TRIGGER IF EXISTS trigger_clear_workspace_acls_on_sharing_setting_change ON organizations;
DROP FUNCTION IF EXISTS clear_workspace_acls_on_sharing_setting_change();
DROP TRIGGER IF EXISTS trigger_clear_workspace_acls_when_sharing_disallowed ON workspaces;
DROP FUNCTION IF EXISTS clear_workspace_acls_when_sharing_disallowed();
DROP FUNCTION IF EXISTS workspace_sharing_allowed(uuid, uuid);
