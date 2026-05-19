CREATE TABLE ai_gateway_coderd_keys (
    id uuid PRIMARY KEY,
    created_at timestamptz NOT NULL,
    name varchar(64) NOT NULL,
    key_prefix varchar(10) NOT NULL,
    hashed_secret bytea NOT NULL,
    last_used_at timestamptz NULL
);

CREATE UNIQUE INDEX ai_gateway_coderd_keys_name_idx ON ai_gateway_coderd_keys USING btree (lower(name));

ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'ai_gateway_coderd_key';

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_coderd_key:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_coderd_key:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_coderd_key:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_coderd_key:read';
