-- Create task_snapshots table for storing log snapshots when tasks are paused.
-- This table holds the conversation history from AgentAPI, allowing users to view
-- task logs even when the workspace is stopped.
CREATE TABLE task_snapshots (
	task_id         UUID        NOT NULL PRIMARY KEY REFERENCES tasks (id) ON DELETE CASCADE,
	log_snapshot    JSONB       NOT NULL,
	log_snapshot_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE task_snapshots IS 'Stores snapshots of task state when paused, currently limited to conversation history.';
COMMENT ON COLUMN task_snapshots.task_id IS 'The task this snapshot belongs to.';
COMMENT ON COLUMN task_snapshots.log_snapshot IS 'Task conversation history in JSON format, allowing users to view logs when the workspace is stopped.';
COMMENT ON COLUMN task_snapshots.log_snapshot_at IS 'When this snapshot was captured.';

-- Add build reasons for task lifecycle events.
-- These distinguish task pause/resume operations from regular workspace lifecycle events.
ALTER TYPE build_reason ADD VALUE IF NOT EXISTS 'task_auto_pause';
ALTER TYPE build_reason ADD VALUE IF NOT EXISTS 'task_manual_pause';
ALTER TYPE build_reason ADD VALUE IF NOT EXISTS 'task_resume';
