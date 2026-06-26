ALTER TABLE gitsshkeys
    ADD COLUMN private_key_key_id TEXT;

ALTER TABLE ONLY gitsshkeys
    ADD CONSTRAINT gitsshkeys_private_key_key_id_fkey FOREIGN KEY (private_key_key_id) REFERENCES dbcrypt_keys(active_key_digest);

COMMENT ON COLUMN gitsshkeys.private_key_key_id IS 'The ID of the key used to encrypt the private key. If this is NULL, the private key is not encrypted.';
