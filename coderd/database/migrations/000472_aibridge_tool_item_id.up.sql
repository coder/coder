ALTER TABLE aibridge_tool_usages
ADD COLUMN provider_item_id text NULL;

COMMENT ON COLUMN aibridge_tool_usages.provider_item_id IS 'The unique output item ID assigned by the provider. Always present for Responses API items. Distinct from provider_tool_call_id, which is the correlation ID used by agentic tools.';
