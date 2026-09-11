-- api_key_scope and resource_type enum values cannot be dropped.
DROP TABLE chat_project_memory_cursors;
DROP INDEX idx_chat_project_memories_project_updated_at;
DROP INDEX idx_chat_project_memories_project_lower_name;
DROP TABLE chat_project_memories;
