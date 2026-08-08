ALTER TABLE user_secrets
    ADD COLUMN value_key_id TEXT;

ALTER TABLE ONLY user_secrets
    ADD CONSTRAINT user_secrets_value_key_id_fkey FOREIGN KEY (value_key_id) REFERENCES dbcrypt_keys(active_key_digest);
