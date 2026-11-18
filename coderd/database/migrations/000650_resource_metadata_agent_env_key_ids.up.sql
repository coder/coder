ALTER TABLE workspace_resource_metadata
ADD COLUMN IF NOT EXISTS value_key_id text REFERENCES dbcrypt_keys (active_key_digest);

COMMENT ON COLUMN workspace_resource_metadata.value_key_id IS 'The ID of the key used to encrypt the value. If this is NULL, the value is not encrypted. Only values of metadata marked sensitive are encrypted.';

ALTER TABLE workspace_agents
ADD COLUMN IF NOT EXISTS environment_variables_key_id text REFERENCES dbcrypt_keys (active_key_digest);

COMMENT ON COLUMN workspace_agents.environment_variables_key_id IS 'The ID of the key used to encrypt the environment variable values. If this is NULL, the values are not encrypted. Variable names are never encrypted.';
