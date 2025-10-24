-- Defensively update any API keys with empty allow_list to have default '*:*'
-- This ensures all existing keys have at least one entry before adding the constraint
UPDATE api_keys
SET allow_list = ARRAY['*:*']
WHERE allow_list = ARRAY[]::text[] OR array_length(allow_list, 1) IS NULL;

-- Add CHECK constraint to ensure allow_list array is never empty
ALTER TABLE api_keys
ADD CONSTRAINT api_keys_allow_list_not_empty
CHECK (array_length(allow_list, 1) > 0);
