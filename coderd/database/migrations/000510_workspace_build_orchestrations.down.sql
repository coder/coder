DROP TABLE IF EXISTS workspace_build_orchestrations;

ALTER TABLE workspace_builds
    DROP CONSTRAINT IF EXISTS workspace_builds_id_workspace_id_key;
