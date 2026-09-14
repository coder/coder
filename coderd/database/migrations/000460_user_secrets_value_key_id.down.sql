ALTER TABLE user_secrets
    DROP CONSTRAINT user_secrets_value_key_id_fkey,
    DROP COLUMN value_key_id;
