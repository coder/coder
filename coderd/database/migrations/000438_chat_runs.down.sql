-- Reverse of 000438_chat_runs.up.sql

-- Drop views first (they depend on tables).
DROP VIEW IF EXISTS chats_with_status;
DROP VIEW IF EXISTS chat_runs_with_status;
DROP VIEW IF EXISTS chat_run_steps_with_status;

-- Remove run/step FK columns from chat_messages.
ALTER TABLE chat_messages DROP COLUMN chat_run_step_id;
ALTER TABLE chat_messages DROP COLUMN chat_run_id;

-- Restore token/cost columns on chat_messages.
ALTER TABLE chat_messages ADD COLUMN input_tokens BIGINT;
ALTER TABLE chat_messages ADD COLUMN output_tokens BIGINT;
ALTER TABLE chat_messages ADD COLUMN total_tokens BIGINT;
ALTER TABLE chat_messages ADD COLUMN reasoning_tokens BIGINT;
ALTER TABLE chat_messages ADD COLUMN cache_creation_tokens BIGINT;
ALTER TABLE chat_messages ADD COLUMN cache_read_tokens BIGINT;
ALTER TABLE chat_messages ADD COLUMN context_limit BIGINT;
ALTER TABLE chat_messages ADD COLUMN total_cost_micros BIGINT;

-- Drop triggers and functions.
DROP TRIGGER IF EXISTS tg_chat_run_step_chat_id ON chat_run_steps;
DROP FUNCTION IF EXISTS tg_enforce_chat_run_step_chat_id();
DROP TRIGGER IF EXISTS tg_chat_run_step_number ON chat_run_steps;
DROP FUNCTION IF EXISTS tg_assign_chat_run_step_number();
DROP TRIGGER IF EXISTS tg_chat_run_number ON chat_runs;
DROP FUNCTION IF EXISTS tg_assign_chat_run_number();

-- Drop tables (steps before runs due to FK).
DROP TABLE IF EXISTS chat_run_steps;
DROP TABLE IF EXISTS chat_runs;

-- Re-create the chat_status enum (must exist before the column).
CREATE TYPE chat_status AS ENUM (
    'waiting',
    'pending',
    'running',
    'error'
);

-- Restore columns on chats.
ALTER TABLE chats ADD COLUMN status chat_status NOT NULL DEFAULT 'waiting';
ALTER TABLE chats ADD COLUMN worker_id UUID;
ALTER TABLE chats ADD COLUMN started_at TIMESTAMPTZ;
ALTER TABLE chats ADD COLUMN heartbeat_at TIMESTAMPTZ;
ALTER TABLE chats ADD COLUMN last_error TEXT;
ALTER TABLE chats DROP COLUMN last_run_number;

-- Re-create the pending index.
CREATE INDEX idx_chats_pending ON chats(updated_at ASC)
    WHERE status = 'pending';
