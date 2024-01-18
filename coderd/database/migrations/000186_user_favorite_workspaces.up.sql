CREATE TABLE favorite_workspaces (
	user_id uuid NOT NULL,
	workspace_id uuid NOT NULL,
	UNIQUE(user_id, workspace_id)
);

ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'favorite_workspace';
