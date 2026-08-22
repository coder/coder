DROP INDEX IF EXISTS credential_api_key_key_id_idx;

ALTER TABLE credential_api_key
    DROP COLUMN key_id;
