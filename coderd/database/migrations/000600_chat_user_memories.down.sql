-- api_key_scope and resource_type enum values cannot be dropped.
ALTER TABLE chat_memory_cursors RENAME TO chat_project_memory_cursors;
COMMENT ON TABLE chat_project_memory_cursors IS 'Per-chat cursors for project memory extraction.';
DROP INDEX idx_chat_user_memories_user_organization_updated_at;
DROP INDEX idx_chat_user_memories_user_organization_lower_name;
DROP TABLE chat_user_memories;
