-- Drop all CHECK constraints added in the up migration
ALTER TABLE api_keys
DROP CONSTRAINT api_keys_allow_list_not_empty;
