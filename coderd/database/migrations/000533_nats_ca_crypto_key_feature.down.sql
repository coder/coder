DELETE FROM crypto_keys WHERE feature = 'nats_ca';

CREATE TYPE old_crypto_key_feature AS ENUM (
    'workspace_apps_token',
    'workspace_apps_api_key',
    'oidc_convert',
    'tailnet_resume'
);

ALTER TABLE crypto_keys
    ALTER COLUMN feature TYPE old_crypto_key_feature
    USING (feature::text::old_crypto_key_feature);

DROP TYPE crypto_key_feature;

ALTER TYPE old_crypto_key_feature RENAME TO crypto_key_feature;
