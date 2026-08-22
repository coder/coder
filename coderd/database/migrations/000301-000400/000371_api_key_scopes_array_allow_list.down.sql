-- Recreate single-scope column and collapse arrays
ALTER TABLE api_keys ADD COLUMN scope api_key_scope DEFAULT 'all'::api_key_scope NOT NULL;

-- Collapse logic: prefer 'all', else 'application_connect', else 'all'
UPDATE api_keys SET scope =
    CASE
        WHEN 'all'::api_key_scope = ANY(scopes) THEN 'all'::api_key_scope
        WHEN 'application_connect'::api_key_scope = ANY(scopes) THEN 'application_connect'::api_key_scope
        ELSE 'all'::api_key_scope
    END;

-- Drop new columns
ALTER TABLE api_keys DROP COLUMN allow_list;
ALTER TABLE api_keys DROP COLUMN scopes;

-- Note: We intentionally keep the expanded enum values to avoid dependency churn.
-- If strict narrowing is required, create a new type with only ('all','application_connect'),
-- cast column, drop the new type, and rename.
