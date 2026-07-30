ALTER TABLE mcp_server_configs
    ADD COLUMN allow_in_plan_mode BOOLEAN NOT NULL DEFAULT false;
