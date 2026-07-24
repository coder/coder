-- Migration 491 adds forward_coder_headers with a default of false.
-- Flip the existing fixture row to true here so fixture data exercises
-- the non-default state only after the column exists.
UPDATE mcp_server_configs
SET forward_coder_headers = TRUE
WHERE id = 'a1b2c3d4-e5f6-7890-abcd-ef1234567890';
