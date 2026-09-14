-- Migration 473 adds allow_in_plan_mode with a default of false.
-- Flip the existing fixture row to true here so fixture data exercises
-- the non-default state only after the column exists.
UPDATE mcp_server_configs
SET allow_in_plan_mode = TRUE
WHERE id = 'a1b2c3d4-e5f6-7890-abcd-ef1234567890';
