-- Drop the unique indexes first (in reverse order of creation)
DROP INDEX IF EXISTS user_secrets_user_file_path_idx;
DROP INDEX IF EXISTS user_secrets_user_env_name_idx;
DROP INDEX IF EXISTS user_secrets_user_name_idx;

-- Drop the table
DROP TABLE IF EXISTS user_secrets;
