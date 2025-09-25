-- Defensively update any API keys with zero or negative lifetime_seconds to default 86400 (24 hours)
-- This aligns with application logic that converts 0 to 86400
UPDATE api_keys
SET lifetime_seconds = 86400
WHERE lifetime_seconds <= 0;

-- Add CHECK constraint to ensure lifetime_seconds is positive
-- This enforces positive lifetimes at database level
ALTER TABLE api_keys
ADD CONSTRAINT api_keys_lifetime_seconds_positive
CHECK (lifetime_seconds > 0);

-- Defensivey update any API keys expires_at with its created_at value
-- This ensures all existing keys have a non-null expires_at before
-- adding the constraint.
UPDATE api_keys
SET expires_at = created_at
WHERE expires_at < created_at;

-- Add CHECK constraint to ensure expires_at is at or after created_at
-- This prevents illogical data where a key expires before it's created
ALTER TABLE api_keys
ADD CONSTRAINT api_keys_expires_at_after_created
CHECK (expires_at >= created_at);
