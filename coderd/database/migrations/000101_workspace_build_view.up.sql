BEGIN;
-- workspace_builds_rbac includes the linked workspace information
-- required to perform RBAC checks on workspace builds without needing
-- to fetch the workspace.
CREATE VIEW workspace_builds_rbac AS
SELECT
	workspace_builds.*,
	workspaces.organization_id AS organization_id,
	workspaces.owner_id AS workspace_owner_id
FROM
	workspace_builds
INNER JOIN
	workspaces ON workspace_builds.workspace_id = workspaces.id;
COMMIT;
