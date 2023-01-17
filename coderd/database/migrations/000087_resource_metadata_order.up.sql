ALTER TABLE workspace_resource_metadata DROP CONSTRAINT workspace_resource_metadata_pkey;

ALTER TABLE workspace_resource_metadata ADD COLUMN id BIGSERIAL PRIMARY KEY; 

ALTER TABLE workspace_resource_metadata ADD CONSTRAINT workspace_resource_metadata_name UNIQUE(workspace_resource_id, key);
