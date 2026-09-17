ALTER TABLE chat_context_resources
    DROP COLUMN discovered;

COMMENT ON COLUMN chat_context_resources.source_path IS 'User-declared scan root that produced this resource. Empty for built-in scan roots.';
