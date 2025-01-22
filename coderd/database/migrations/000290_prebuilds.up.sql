-- TODO: using "none" for login type produced this error: 'unsafe use of new value "none" of enum type login_type' -> not sure why
INSERT INTO users (id, email, username, name, created_at, updated_at, status, rbac_roles, hashed_password, is_system)
VALUES ('c42fdf75-3097-471c-8c33-fb52454d81c0', 'prebuilds@system', 'prebuilds', 'Prebuilds Owner', now(), now(),
		'active', '{}', 'none', true);

CREATE VIEW workspace_prebuilds AS
SELECT *
FROM workspaces
WHERE owner_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0';

CREATE VIEW workspace_prebuild_builds AS
SELECT workspace_id
FROM workspace_builds
WHERE initiator_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0';
