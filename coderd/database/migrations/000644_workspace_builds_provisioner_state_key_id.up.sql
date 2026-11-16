ALTER TABLE workspace_builds
ADD COLUMN IF NOT EXISTS provisioner_state_key_id text REFERENCES dbcrypt_keys (active_key_digest);

COMMENT ON COLUMN workspace_builds.provisioner_state_key_id IS 'The ID of the key used to encrypt the provisioner state. If this is NULL, the state is not encrypted. Terraform state embeds owner session tokens, git SSH private keys, OIDC access tokens, external auth tokens and coder_env values (SEC-1491).';
