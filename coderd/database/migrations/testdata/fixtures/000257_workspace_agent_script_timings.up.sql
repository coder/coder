INSERT INTO workspace_agent_script_timings (script_id, started_at, ended_at, exit_code, stage, status)
VALUES
    ((SELECT id FROM workspace_agent_scripts LIMIT 1), NOW() - INTERVAL '1 hour 55 minutes', NOW() - INTERVAL '1 hour 50 minutes', 0, 'start', 'ok');
