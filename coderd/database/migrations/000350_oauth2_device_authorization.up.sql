-- Add OAuth2 Device Authorization Grant support (RFC 8628)

-- Add resource type for audit logging
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'oauth2_provider_device_code';

-- Create the status enum type
CREATE TYPE oauth2_device_status AS ENUM ('pending', 'authorized', 'denied');

CREATE TABLE oauth2_provider_device_codes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at timestamptz NOT NULL DEFAULT NOW(),
    expires_at timestamptz NOT NULL,

    -- Device code (hashed for security)
    device_code_hash bytea NOT NULL UNIQUE,
    -- Device code prefix: 8 chars for efficient lookup while maintaining security
    device_code_prefix text NOT NULL UNIQUE CHECK (length(device_code_prefix) = 8),

    -- User code: RFC 8628 recommends 6-8 characters, formatted as XXXX-XXXX for readability (9 chars)
    user_code text NOT NULL CHECK (length(user_code) >= 6 AND length(user_code) <= 9),

    -- Client and authorization info
    client_id uuid NOT NULL REFERENCES oauth2_provider_apps(id) ON DELETE CASCADE,
    user_id uuid REFERENCES users(id) ON DELETE CASCADE, -- NULL until authorized

    -- Authorization state (using enum for better data integrity)
    status oauth2_device_status NOT NULL DEFAULT 'pending',

    -- RFC 8628 parameters
    verification_uri text NOT NULL,
    verification_uri_complete text,
    scope text DEFAULT '',
    resource_uri text, -- RFC 8707 resource parameter
    polling_interval integer NOT NULL DEFAULT 5 -- polling interval in seconds
);

-- Indexes for performance
CREATE INDEX idx_oauth2_provider_device_codes_client_id ON oauth2_provider_device_codes(client_id);
CREATE INDEX idx_oauth2_provider_device_codes_expires_at ON oauth2_provider_device_codes(expires_at);
CREATE INDEX idx_oauth2_provider_device_codes_device_code_hash ON oauth2_provider_device_codes(device_code_hash);

-- Cleanup expired device codes (for background cleanup job)
CREATE INDEX idx_oauth2_provider_device_codes_cleanup ON oauth2_provider_device_codes(expires_at);

-- RFC 8628: Enforce case-insensitive uniqueness on user_code
CREATE UNIQUE INDEX oauth2_device_codes_user_code_ci_idx
    ON oauth2_provider_device_codes (UPPER(user_code));

-- Comments for documentation
COMMENT ON TABLE oauth2_provider_device_codes IS 'RFC 8628: OAuth2 Device Authorization Grant device codes';
COMMENT ON COLUMN oauth2_provider_device_codes.device_code_hash IS 'Hashed device code for security';
COMMENT ON COLUMN oauth2_provider_device_codes.device_code_prefix IS 'Device code prefix for lookup (first 8 chars)';
COMMENT ON COLUMN oauth2_provider_device_codes.user_code IS 'Human-readable code shown to user (6-8 characters)';
COMMENT ON COLUMN oauth2_provider_device_codes.verification_uri IS 'URI where user enters user_code';
COMMENT ON COLUMN oauth2_provider_device_codes.verification_uri_complete IS 'Optional complete URI with user_code embedded';
COMMENT ON COLUMN oauth2_provider_device_codes.polling_interval IS 'Minimum polling interval in seconds (RFC 8628)';
COMMENT ON COLUMN oauth2_provider_device_codes.resource_uri IS 'RFC 8707 resource parameter for audience restriction';
COMMENT ON COLUMN oauth2_provider_device_codes.status IS 'Current authorization status: pending (awaiting user action), authorized (user approved), or denied (user rejected)';
