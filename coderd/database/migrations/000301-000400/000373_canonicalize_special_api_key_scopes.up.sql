-- Canonicalize special API key scopes to coder:* namespace
-- Rename enum values: 'all' -> 'coder:all', 'application_connect' -> 'coder:application_connect'

ALTER TYPE api_key_scope RENAME VALUE 'all' TO 'coder:all';
ALTER TYPE api_key_scope RENAME VALUE 'application_connect' TO 'coder:application_connect';
