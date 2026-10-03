CREATE TABLE ai_gateway_keys (
    id uuid PRIMARY KEY,
    created_at timestamptz NOT NULL,
    name text NOT NULL,
    secret_prefix varchar(11) NOT NULL,
    hashed_secret bytea NOT NULL,
    last_used_at timestamptz NULL,
    CONSTRAINT ai_gateway_keys_name_check CHECK (length(name) <= 64 AND name ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    CONSTRAINT ai_gateway_keys_secret_prefix_check CHECK (length(secret_prefix) = 11),
    CONSTRAINT ai_gateway_keys_hashed_secret_check CHECK (length(hashed_secret) > 0)
);

COMMENT ON TABLE ai_gateway_keys IS 'Hashed bearer secrets used by AI Gateway standalone replicas to authenticate into coderd.';
COMMENT ON COLUMN ai_gateway_keys.secret_prefix IS 'Public token prefix for display and audit correlation. Auth uses hashed_secret.';

CREATE UNIQUE INDEX ai_gateway_keys_name_idx ON ai_gateway_keys USING btree (lower(name));
CREATE UNIQUE INDEX ai_gateway_keys_secret_prefix_idx ON ai_gateway_keys USING btree (secret_prefix);
CREATE UNIQUE INDEX ai_gateway_keys_hashed_secret_idx ON ai_gateway_keys USING btree (hashed_secret);

ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'ai_gateway_key';

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_key:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_key:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_key:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_key:read';
