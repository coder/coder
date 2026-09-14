COMMENT ON COLUMN api_keys.hashed_secret
IS 'hashed_secret contains a SHA256 hash of the key secret. This is considered a secret and MUST NOT be returned from the API as it is used for API key encryption in app proxying code.';
