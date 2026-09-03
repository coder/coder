-- RFC 8628 Device Authorization Grant. A device code is a short-lived
-- credential a client polls against while a user approves the request on a
-- separate device.

CREATE TABLE oauth2_provider_device_codes (
    id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,

    -- Stored like every other OAuth2 secret: a lookup prefix plus a SHA-256
    -- hash of the secret.
    secret_prefix bytea NOT NULL,
    hashed_secret bytea NOT NULL,

    -- The short code a human reads off the device and types on another one.
    user_code text NOT NULL,

    app_id uuid NOT NULL,
    -- NULL until a user approves or denies. Records who decided.
    user_id uuid,

    status text NOT NULL DEFAULT 'pending',

    scope text NOT NULL,
    -- RFC 8707 resource indicator, carried to the issued token's audience.
    resource_uri text,

    PRIMARY KEY (id),
    CONSTRAINT oauth2_provider_device_codes_secret_prefix_key UNIQUE (secret_prefix),
    CONSTRAINT oauth2_provider_device_codes_status_check
        CHECK (status IN ('pending', 'authorized', 'denied')),
    CONSTRAINT oauth2_provider_device_codes_scope_not_empty
        CHECK (scope <> ''),
    CONSTRAINT oauth2_provider_device_codes_app_id_fkey
        FOREIGN KEY (app_id) REFERENCES oauth2_provider_apps(id) ON DELETE CASCADE,
    CONSTRAINT oauth2_provider_device_codes_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- User codes are compared case-insensitively, so uniqueness is enforced on
-- the same normalized form the lookup uses.
CREATE UNIQUE INDEX idx_oauth2_provider_device_codes_user_code
    ON oauth2_provider_device_codes (upper(user_code));

-- Supports the expiry sweep a later dbpurge change will add.
CREATE INDEX idx_oauth2_provider_device_codes_expires_at
    ON oauth2_provider_device_codes (expires_at);

COMMENT ON TABLE oauth2_provider_device_codes IS 'RFC 8628 device authorization grant codes. A device code is exchanged for a token once a user approves the matching user code.';
COMMENT ON COLUMN oauth2_provider_device_codes.user_code IS 'Short human-typed code displayed by the device. Compared case-insensitively.';
COMMENT ON COLUMN oauth2_provider_device_codes.user_id IS 'The user who approved or denied. NULL while the request is pending.';
COMMENT ON COLUMN oauth2_provider_device_codes.scope IS 'Negotiated scope, persisted at authorization and applied to the issued API key at redemption.';
COMMENT ON COLUMN oauth2_provider_device_codes.resource_uri IS 'RFC 8707 resource parameter for audience restriction.';
