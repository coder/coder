-- Reverse 000530: drop origin_kind, restore the source_path column name,
-- and drop the origin-kind enum. The workspace_agent_context_* enum
-- types from 000522 are left in place.
ALTER TABLE chat_context_resources DROP COLUMN IF EXISTS origin_kind;
ALTER TABLE workspace_agent_context_resources DROP COLUMN IF EXISTS origin_kind;

ALTER TABLE chat_context_resources
    RENAME COLUMN origin_root TO source_path;
ALTER TABLE workspace_agent_context_resources
    RENAME COLUMN origin_root TO source_path;

DROP TYPE IF EXISTS workspace_agent_context_origin_kind;

COMMENT ON COLUMN workspace_agent_context_resources.source_path IS 'User-declared scan root that produced this resource. Empty for built-in scan roots.';
COMMENT ON COLUMN chat_context_resources.source_path IS 'User-declared scan root that produced this resource. Empty for built-in scan roots.';
