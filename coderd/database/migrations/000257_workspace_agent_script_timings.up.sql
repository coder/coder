ALTER TABLE workspace_agent_scripts ADD COLUMN id uuid UNIQUE NOT NULL DEFAULT gen_random_uuid();

CREATE TYPE workspace_agent_script_timing_stage AS ENUM (
    'start',
    'stop',
    'cron'
);

COMMENT ON TYPE workspace_agent_script_timing_stage IS 'What stage the script was ran in.';

CREATE TYPE workspace_agent_script_timing_status AS ENUM (
    'ok',
    'exit_failure',
    'timed_out',
    'pipes_left_open'
);

COMMENT ON TYPE workspace_agent_script_timing_status IS 'What the exit status of the script is.';

CREATE TABLE workspace_agent_script_timings
(
    script_id     uuid                                 NOT NULL REFERENCES workspace_agent_scripts (id) ON DELETE CASCADE,
    started_at    timestamp with time zone             NOT NULL,
    ended_at      timestamp with time zone             NOT NULL,
    exit_code     int                                  NOT NULL,
    stage         workspace_agent_script_timing_stage  NOT NULL,
    status        workspace_agent_script_timing_status NOT NULL,
    UNIQUE (script_id, started_at)
);

COMMENT ON TYPE workspace_agent_script_timings IS 'Timing and execution information about a script run.';
