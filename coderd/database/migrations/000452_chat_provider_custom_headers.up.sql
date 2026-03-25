ALTER TABLE chat_providers
    ADD COLUMN custom_headers TEXT NOT NULL DEFAULT '{}',
    ADD COLUMN custom_headers_key_id TEXT REFERENCES dbcrypt_keys(active_key_digest);
