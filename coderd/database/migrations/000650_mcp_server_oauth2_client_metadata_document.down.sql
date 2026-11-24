UPDATE mcp_server_configs
    SET oauth2_client_registration = 'dynamic'
    WHERE oauth2_client_registration = 'metadata_document';

ALTER TABLE mcp_server_configs
    DROP CONSTRAINT mcp_server_configs_oauth2_client_registration_check,
    ADD CONSTRAINT mcp_server_configs_oauth2_client_registration_check
        CHECK (oauth2_client_registration IN ('manual', 'dynamic'));
