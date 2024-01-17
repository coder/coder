CREATE TABLE user_pinned_workspaces (
	user_id uuid NOT NULL,
	workspace_id uuid NOT NULL,
	UNIQUE(user_id, workspace_id)
);
