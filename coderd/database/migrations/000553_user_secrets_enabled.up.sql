-- Add an explicit enabled flag to user_secrets.
--
-- A disabled secret stays visible and editable in the management UI, CLI,
-- and API, but is not injected into workspaces and does not satisfy any
-- "secret present" predicate. This is the single source of truth for
-- "not injected"; the agent manifest layer no longer skips rows based
-- on having both env_name and file_path empty.
--
-- Existing rows whose env_name and file_path are both empty are flipped
-- to enabled = false. Today those rows are silently skipped during agent
-- manifest assembly, so flipping them preserves observable behavior
-- while letting the manifest stop encoding the both-empty special case.
-- The write-time invariant (an enabled secret must have at least one of
-- env_name / file_path non-empty) is enforced at the API layer, so no
-- CHECK constraint is added here. Disabled secrets may have no targets;
-- bulk imports use that state for keys that cannot be env-injected.
ALTER TABLE user_secrets
    ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT true;

UPDATE user_secrets
SET    enabled = false
WHERE  env_name = '' AND file_path = '';
