-- These partial indexes served only the FinalizeStaleChatDebugRows query,
-- which was deleted with the stale debug-run sweep.
DROP INDEX idx_chat_debug_runs_stale;
DROP INDEX idx_chat_debug_steps_stale;
