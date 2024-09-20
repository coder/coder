ALTER TABLE workspace_agent_scripts ADD COLUMN id           uuid UNIQUE NOT NULL DEFAULT gen_random_uuid();

CREATE TYPE workspace_agent_script_timing_stage AS ENUM (
    'start',
    'stop',
    'cron'
    );

CREATE TABLE workspace_agent_script_timings
(
    script_id     uuid                                NOT NULL REFERENCES workspace_agent_scripts (id) ON DELETE CASCADE,
    display_name  text                                NOT NULL,
    started_at    timestamp with time zone            NOT NULL,
    ended_at      timestamp with time zone            NOT NULL,
    exit_code     int                                 NOT NULL,
    stage         workspace_agent_script_timing_stage NOT NULL,
    timed_out     bool                                NOT NULL
);
