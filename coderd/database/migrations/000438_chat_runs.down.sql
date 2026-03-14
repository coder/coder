-- Reverse 000434_chat_runs.up.sql

-- Drop views (reverse order of creation).
DROP VIEW IF EXISTS chats_with_status;
DROP VIEW IF EXISTS chat_runs_with_status;
DROP VIEW IF EXISTS chat_run_steps_with_status;

-- Remove run/step linkage from messages.
ALTER TABLE chat_messages DROP COLUMN IF EXISTS chat_run_step_id;
ALTER TABLE chat_messages DROP COLUMN IF EXISTS chat_run_id;

-- Drop triggers and functions.
DROP TRIGGER IF EXISTS tg_chat_run_step_chat_id ON chat_run_steps;
DROP FUNCTION IF EXISTS tg_enforce_chat_run_step_chat_id();
DROP TRIGGER IF EXISTS tg_chat_run_step_number ON chat_run_steps;
DROP FUNCTION IF EXISTS tg_assign_chat_run_step_number();
DROP TRIGGER IF EXISTS tg_chat_run_number ON chat_runs;
DROP FUNCTION IF EXISTS tg_assign_chat_run_number();

-- Drop tables.
DROP TABLE IF EXISTS chat_run_steps;
DROP TABLE IF EXISTS chat_runs;

-- Re-create the chat_status enum (must exist before the column).
CREATE TYPE chat_status AS ENUM (
    'waiting',
    'pending',
    'running',
    'paused',
    'completed',
    'error'
);

-- Restore columns dropped from chats.
ALTER TABLE chats ADD COLUMN status chat_status NOT NULL DEFAULT 'waiting';
ALTER TABLE chats ADD COLUMN worker_id UUID;
ALTER TABLE chats ADD COLUMN started_at TIMESTAMPTZ;
ALTER TABLE chats ADD COLUMN heartbeat_at TIMESTAMPTZ;
ALTER TABLE chats ADD COLUMN last_error TEXT;
CREATE INDEX idx_chats_pending ON chats(status) WHERE status = 'pending';

-- Remove the run tracking column.
ALTER TABLE chats DROP COLUMN last_run_number;
