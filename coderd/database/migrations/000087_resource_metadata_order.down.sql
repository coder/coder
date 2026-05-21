ALTER TABLE workspace_resource_metadata DROP COLUMN id;
ALTER TABLE workspace_resource_metadata DROP CONSTRAINT workspace_resource_metadata_name;
ALTER TABLE workspace_resource_metadata ADD CONSTRAINT workspace_resource_metadata_pkey PRIMARY KEY (workspace_resource_id, key);

