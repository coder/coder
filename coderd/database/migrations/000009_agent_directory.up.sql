ALTER TABLE ONLY workspace_agents
    -- UNIX paths are a maximum length of 4096.
    ADD COLUMN IF NOT EXISTS directory varchar(4096) DEFAULT '' NOT NULL;
