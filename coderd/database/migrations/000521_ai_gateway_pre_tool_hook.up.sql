-- Add the pre-tool hook: fires once per assembled, client-bound tool call,
-- before the call is released to the client. Only classify and decide are valid
-- there (enforced in application code), mirroring pre-auth.
ALTER TYPE ai_gateway_hook ADD VALUE IF NOT EXISTS 'pre_tool';
