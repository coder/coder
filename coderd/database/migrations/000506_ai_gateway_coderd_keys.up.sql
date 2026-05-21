CREATE TABLE ai_gateway_coderd_keys (
    id uuid PRIMARY KEY,
    created_at timestamptz NOT NULL,
    name varchar(64) NOT NULL,
    secret_prefix varchar(11) NOT NULL CHECK (length(secret_prefix) = 11),
    hashed_secret bytea NOT NULL,
    last_used_at timestamptz NULL,
    CONSTRAINT ai_gateway_coderd_keys_name_check CHECK (name ~ '^[a-z0-9]+(-[a-z0-9]+)*$')
);

COMMENT ON TABLE ai_gateway_coderd_keys IS 'Hashed bearer secrets used by AI Gateway standalone replicas to authenticate into coderd.';
COMMENT ON COLUMN ai_gateway_coderd_keys.secret_prefix IS 'Public token prefix for display and audit correlation. Auth uses hashed_secret.';

CREATE UNIQUE INDEX ai_gateway_coderd_keys_name_idx ON ai_gateway_coderd_keys USING btree (lower(name));
CREATE UNIQUE INDEX ai_gateway_coderd_keys_secret_prefix_idx ON ai_gateway_coderd_keys USING btree (secret_prefix);
CREATE UNIQUE INDEX ai_gateway_coderd_keys_hashed_secret_idx ON ai_gateway_coderd_keys USING btree (hashed_secret);

ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'ai_gateway_coderd_key';

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_coderd_key:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_coderd_key:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_coderd_key:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_coderd_key:read';
