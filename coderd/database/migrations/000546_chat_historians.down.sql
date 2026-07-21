DROP INDEX IF EXISTS chats_historian_candidates_idx;
DROP TABLE IF EXISTS chat_historian_states;

-- Enum additions are intentionally not reverted because PostgreSQL cannot
-- safely remove individual enum values.
