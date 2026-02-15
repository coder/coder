DROP INDEX IF EXISTS webauthn_credentials_user_id_idx;
DROP INDEX IF EXISTS webauthn_credentials_credential_id_idx;
DROP TABLE IF EXISTS webauthn_credentials;

-- Remove webauthn_connect from crypto_key_feature enum.
CREATE TYPE old_crypto_key_feature AS ENUM (
    'workspace_apps_token',
    'workspace_apps_api_key',
    'oidc_convert',
    'tailnet_resume'
);

DELETE FROM crypto_keys WHERE feature = 'webauthn_connect';

ALTER TABLE crypto_keys
    ALTER COLUMN feature TYPE old_crypto_key_feature
    USING (feature::text::old_crypto_key_feature);

DROP TYPE crypto_key_feature;

ALTER TYPE old_crypto_key_feature RENAME TO crypto_key_feature;
