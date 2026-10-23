CREATE INDEX idx_chat_debug_runs_stale ON chat_debug_runs(updated_at) WHERE finished_at IS NULL;
CREATE INDEX idx_chat_debug_steps_stale ON chat_debug_steps(updated_at) WHERE finished_at IS NULL;
