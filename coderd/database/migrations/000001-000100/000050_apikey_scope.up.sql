CREATE TYPE api_key_scope AS ENUM (
    'all',
    'application_connect'
);

ALTER TABLE api_keys ADD COLUMN scope api_key_scope NOT NULL DEFAULT 'all';
