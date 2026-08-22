ALTER TABLE templates ADD COLUMN display_name VARCHAR(64) NOT NULL DEFAULT '';

COMMENT ON COLUMN templates.display_name
IS 'Display name is a custom, human-friendly template name that user can set.';
