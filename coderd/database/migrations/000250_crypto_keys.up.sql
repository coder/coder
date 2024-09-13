CREATE TYPE crypto_key_feature AS ENUM (
    'workspace_apps',
    'oidc_convert',
    'tailnet_resume'
);

CREATE TABLE crypto_keys (
    feature crypto_key_feature NOT NULL,
    sequence integer NOT NULL,
    secret text NULL,
    secret_key_id text NULL REFERENCES dbcrypt_keys(active_key_digest),
    starts_at timestamptz NOT NULL,
    deletes_at timestamptz NULL,
    PRIMARY KEY (feature, sequence)
);

