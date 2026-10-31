ALTER TABLE mcp_server_configs
    ADD COLUMN oauth2_client_registration text NOT NULL DEFAULT 'manual'
    CONSTRAINT mcp_server_configs_oauth2_client_registration_check
        CHECK (oauth2_client_registration IN ('manual', 'dynamic'));
