-- Add run tracking to chats.
ALTER TABLE chats ADD COLUMN last_run_number INTEGER NOT NULL DEFAULT 0;

-- chat_runs: one row per user-triggered invocation.
CREATE TABLE chat_runs (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id          UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    number           INTEGER     NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    worker_id        UUID,
    last_step_number INTEGER     NOT NULL DEFAULT 0,
    UNIQUE (chat_id, number)
);

-- Auto-assign run number by atomically incrementing chats.last_run_number.
CREATE FUNCTION tg_assign_chat_run_number() RETURNS trigger AS $$
BEGIN
    UPDATE chats SET last_run_number = last_run_number + 1
        WHERE id = NEW.chat_id
        RETURNING last_run_number INTO NEW.number;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tg_chat_run_number
    BEFORE INSERT ON chat_runs
    FOR EACH ROW EXECUTE FUNCTION tg_assign_chat_run_number();

-- chat_run_steps: one row per LLM call within a run.
CREATE TABLE chat_run_steps (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_run_id           UUID        NOT NULL REFERENCES chat_runs(id) ON DELETE CASCADE,
    chat_id               UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    number                INTEGER     NOT NULL,
    model_config_id       UUID        REFERENCES chat_model_configs(id),

    started_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    heartbeat_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at          TIMESTAMPTZ,
    interrupted_at        TIMESTAMPTZ,
    error                 TEXT,

    -- Why the run continued after this step (NULL = terminal step).
    continuation_reason   TEXT,

    -- Token usage.
    input_tokens          INTEGER,
    output_tokens         INTEGER,
    total_tokens          INTEGER,
    reasoning_tokens      INTEGER,
    cache_creation_tokens INTEGER,
    cache_read_tokens     INTEGER,
    context_limit         INTEGER,

    -- Tool call summary.
    tool_calls_total      INTEGER     NOT NULL DEFAULT 0,
    tool_calls_completed  INTEGER     NOT NULL DEFAULT 0,
    tool_calls_errored    INTEGER     NOT NULL DEFAULT 0,

    UNIQUE (chat_run_id, number)
);

-- Auto-assign step number by atomically incrementing
-- chat_runs.last_step_number.
CREATE FUNCTION tg_assign_chat_run_step_number() RETURNS trigger AS $$
BEGIN
    UPDATE chat_runs SET last_step_number = last_step_number + 1
        WHERE id = NEW.chat_run_id
        RETURNING last_step_number INTO NEW.number;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tg_chat_run_step_number
    BEFORE INSERT ON chat_run_steps
    FOR EACH ROW EXECUTE FUNCTION tg_assign_chat_run_step_number();

-- Enforce that chat_run_steps.chat_id matches the parent
-- chat_runs.chat_id. A mismatch would bypass the single-active-step
-- partial unique index.
CREATE FUNCTION tg_enforce_chat_run_step_chat_id() RETURNS trigger AS $$
DECLARE
    run_chat_id UUID;
BEGIN
    SELECT chat_id INTO run_chat_id FROM chat_runs WHERE id = NEW.chat_run_id;
    IF run_chat_id IS DISTINCT FROM NEW.chat_id THEN
        RAISE EXCEPTION 'chat_run_steps.chat_id (%) does not match chat_runs.chat_id (%)',
            NEW.chat_id, run_chat_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tg_chat_run_step_chat_id
    BEFORE INSERT ON chat_run_steps
    FOR EACH ROW EXECUTE FUNCTION tg_enforce_chat_run_step_chat_id();

-- Enforce at most one active step per chat. This is the database-level
-- guarantee that prevents concurrent runs on the same chat.
CREATE UNIQUE INDEX chat_run_steps_single_active
    ON chat_run_steps (chat_id)
    WHERE completed_at IS NULL
      AND error IS NULL
      AND interrupted_at IS NULL;

-- Non-partial index for FK cascade performance when deleting chats.
CREATE INDEX idx_chat_run_steps_chat_id ON chat_run_steps(chat_id);

-- Link messages to their originating run and step.
ALTER TABLE chat_messages ADD COLUMN chat_run_id UUID REFERENCES chat_runs(id) ON DELETE CASCADE;
ALTER TABLE chat_messages ADD COLUMN chat_run_step_id UUID REFERENCES chat_run_steps(id) ON DELETE CASCADE;

-- Drop columns from chats that are now tracked on runs/steps.
DROP INDEX IF EXISTS idx_chats_pending;
ALTER TABLE chats DROP COLUMN status;
ALTER TABLE chats DROP COLUMN worker_id;
ALTER TABLE chats DROP COLUMN started_at;
ALTER TABLE chats DROP COLUMN heartbeat_at;
ALTER TABLE chats DROP COLUMN last_error;
DROP TYPE IF EXISTS chat_status;

-- Status views: derive status from step lifecycle columns rather
-- than storing it directly. Historical runs keep their terminal
-- state forever.

CREATE VIEW chat_run_steps_with_status AS
SELECT *,
    CASE
        WHEN error IS NOT NULL THEN 'error'
        WHEN interrupted_at IS NOT NULL THEN 'interrupted'
        WHEN completed_at IS NOT NULL THEN 'completed'
        WHEN heartbeat_at < NOW() - INTERVAL '5 minutes' THEN 'stalled'
        ELSE 'running'
    END AS status
FROM chat_run_steps;

CREATE VIEW chat_runs_with_status AS
SELECT
    r.*,
    s.status AS step_status,
    s.error AS step_error,
    COALESCE(s.completed_at, s.interrupted_at, s.heartbeat_at, s.started_at) AS updated_at
FROM chat_runs r
LEFT JOIN chat_run_steps_with_status s
    ON s.chat_run_id = r.id AND s.number = r.last_step_number;

CREATE VIEW chats_with_status AS
SELECT
    c.*,
    CASE
        -- No runs yet, or last run fully completed.
        WHEN r.step_status IS NULL THEN 'waiting'
        WHEN r.step_status IN ('error', 'stalled') THEN 'error'
        WHEN r.step_status = 'interrupted' THEN 'waiting'
        WHEN r.step_status = 'completed' THEN 'waiting'
        -- Active step exists but no worker has claimed it.
        WHEN r.step_status = 'running' AND r.worker_id IS NULL THEN 'pending'
        -- Active step being processed by a worker.
        WHEN r.step_status = 'running' THEN 'running'
        ELSE 'waiting'
    END AS computed_status,
    r.step_error AS last_run_error,
    r.id AS last_run_id
FROM chats c
LEFT JOIN chat_runs_with_status r
    ON r.chat_id = c.id AND r.number = c.last_run_number;
