-- Bind persisted pre_tool_use decisions to the tool they reviewed.
-- Decision reuse additionally matches the effective reviewed input
-- (input_override when present, original_input otherwise), so a
-- decision recorded for one tool call can never be replayed for a
-- different tool or different input that reuses the same tool-use ID.
-- Pre-existing rows keep tool_name NULL and never match reuse; the
-- next attempt dispatches fresh, which is safe under the documented
-- at-least-once delivery contract.
ALTER TABLE chat_hook_dispatches ADD COLUMN tool_name text;

COMMENT ON COLUMN chat_hook_dispatches.tool_name IS 'Tool name reviewed by a pre_tool_use or post_tool_use dispatch. NULL for other events and for rows recorded before decision reuse was bound to the tool identity.';
