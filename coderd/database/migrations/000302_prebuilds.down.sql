-- Revert prebuild views
DROP VIEW IF EXISTS workspace_prebuild_builds;
DROP VIEW IF EXISTS workspace_prebuilds;
DROP VIEW IF EXISTS workspace_latest_build;

-- Undo the restriction on deleting system users
DROP TRIGGER IF EXISTS prevent_system_user_updates ON users;
DROP TRIGGER IF EXISTS prevent_system_user_deletions ON users;
DROP FUNCTION IF EXISTS prevent_system_user_changes();

-- Revert user operations
-- c42fdf75-3097-471c-8c33-fb52454d81c0 is the identifier for the system user responsible for prebuilds.
DELETE FROM user_status_changes WHERE user_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0';
DELETE FROM users WHERE id = 'c42fdf75-3097-471c-8c33-fb52454d81c0';
