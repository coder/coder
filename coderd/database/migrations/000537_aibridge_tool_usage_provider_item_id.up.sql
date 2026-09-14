ALTER TABLE aibridge_tool_usages
ADD COLUMN provider_item_id text NULL; -- nullable to allow existing data to remain valid

COMMENT ON COLUMN aibridge_tool_usages.provider_item_id IS 'Specific to the OpenAI Responses API: the unique id of the output item that carried the tool call. Distinct from provider_tool_call_id (the call_id correlation key), which is empty for hosted tools. Empty for the chat completions and Anthropic messages APIs, which have no separate item id.';
