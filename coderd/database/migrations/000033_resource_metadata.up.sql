CREATE TABLE IF NOT EXISTS workspace_resource_metadata (
	workspace_resource_id uuid NOT NULL,
	key varchar(1024) NOT NULL,
	value varchar(65536),
	sensitive boolean NOT NULL,
	PRIMARY KEY (workspace_resource_id, key),
	FOREIGN KEY (workspace_resource_id) REFERENCES workspace_resources (id) ON DELETE CASCADE
);
