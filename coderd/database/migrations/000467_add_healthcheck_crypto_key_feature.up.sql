CREATE TYPE new_crypto_key_feature AS ENUM (
    'workspace_apps_token',
    'workspace_apps_api_key',
    'oidc_convert',
    'tailnet_resume',
    'healthcheck'
);

ALTER TABLE crypto_keys
    ALTER COLUMN feature TYPE new_crypto_key_feature
    USING (feature::text::new_crypto_key_feature);

DROP TYPE crypto_key_feature;

ALTER TYPE new_crypto_key_feature RENAME TO crypto_key_feature;
