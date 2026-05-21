-- Revert canonicalization of special API key scopes
-- Rename enum values back: 'coder:all' -> 'all', 'coder:application_connect' -> 'application_connect'

ALTER TYPE api_key_scope RENAME VALUE 'coder:all' TO 'all';
ALTER TYPE api_key_scope RENAME VALUE 'coder:application_connect' TO 'application_connect';
