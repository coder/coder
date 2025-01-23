-- Revert prebuild views
DROP VIEW IF EXISTS workspace_prebuild_builds;
DROP VIEW IF EXISTS workspace_prebuilds;
DROP VIEW IF EXISTS workspace_latest_build;

-- Revert user operations
DELETE FROM user_status_changes WHERE user_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0';
DELETE FROM users WHERE id = 'c42fdf75-3097-471c-8c33-fb52454d81c0';
