DROP TRIGGER IF EXISTS trigger_update_users ON users;
DROP FUNCTION IF EXISTS delete_deleted_user_api_keys;

DROP TRIGGER IF EXISTS trigger_insert_apikeys ON api_keys;
DROP FUNCTION IF EXISTS insert_apikey_fail_if_user_deleted;
