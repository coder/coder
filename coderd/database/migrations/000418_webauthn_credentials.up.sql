-- Add webauthn_connect to crypto_key_feature enum for signing connection JWTs.
CREATE TYPE new_crypto_key_feature AS ENUM (
    'workspace_apps_token',
    'workspace_apps_api_key',
    'oidc_convert',
    'tailnet_resume',
    'webauthn_connect'
);

ALTER TABLE crypto_keys
    ALTER COLUMN feature TYPE new_crypto_key_feature
    USING (feature::text::new_crypto_key_feature);

DROP TYPE crypto_key_feature;

ALTER TYPE new_crypto_key_feature RENAME TO crypto_key_feature;

-- Stores WebAuthn credentials registered by users for FIDO2
-- hardware key authentication on sensitive workspace operations.
CREATE TABLE webauthn_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id BYTEA NOT NULL,
    public_key BYTEA NOT NULL,
    attestation_type TEXT NOT NULL,
    aaguid BYTEA NOT NULL,
    sign_count BIGINT NOT NULL DEFAULT 0,
    name TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP WITH TIME ZONE
);

CREATE UNIQUE INDEX webauthn_credentials_credential_id_idx ON webauthn_credentials(credential_id);
CREATE INDEX webauthn_credentials_user_id_idx ON webauthn_credentials(user_id);
