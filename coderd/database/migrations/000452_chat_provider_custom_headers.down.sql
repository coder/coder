ALTER TABLE chat_providers
    DROP COLUMN IF EXISTS custom_headers_key_id,
    DROP COLUMN IF EXISTS custom_headers;
