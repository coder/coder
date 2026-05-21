CREATE TABLE tasks (
	id                  UUID        NOT NULL PRIMARY KEY,
	organization_id     UUID        NOT NULL REFERENCES organizations     (id) ON DELETE CASCADE,
	owner_id            UUID        NOT NULL REFERENCES users             (id) ON DELETE CASCADE,
	name                TEXT        NOT NULL,
	workspace_id        UUID                 REFERENCES workspaces        (id) ON DELETE CASCADE,
	template_version_id UUID        NOT NULL REFERENCES template_versions (id) ON DELETE CASCADE,
	template_parameters JSONB       NOT NULL DEFAULT '{}'::JSONB,
	prompt              TEXT        NOT NULL,
	created_at          TIMESTAMPTZ NOT NULL,
	deleted_at          TIMESTAMPTZ
);

CREATE TABLE task_workspace_apps (
	task_id            UUID NOT NULL REFERENCES tasks            (id) ON DELETE CASCADE,
	workspace_build_id UUID NOT NULL REFERENCES workspace_builds (id) ON DELETE CASCADE,
	workspace_agent_id UUID NOT NULL REFERENCES workspace_agents (id) ON DELETE CASCADE,
	workspace_app_id   UUID NOT NULL REFERENCES workspace_apps   (id) ON DELETE CASCADE
);
