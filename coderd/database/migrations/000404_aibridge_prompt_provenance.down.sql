DROP INDEX IF EXISTS idx_aibridge_tool_usages_provider_tool_call_id;

ALTER TABLE aibridge_tool_usages
DROP COLUMN provider_tool_call_id;

DROP INDEX IF EXISTS idx_aibridge_interceptions_parent_id;

ALTER TABLE aibridge_interceptions
DROP COLUMN parent_id;
