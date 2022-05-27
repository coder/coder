ALTER TABLE ONLY workspaces DROP COLUMN ttl;
ALTER TABLE ONLY workspaces ADD COLUMN autostop_schedule text DEFAULT NULL;
