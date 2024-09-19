INSERT INTO workspace_agent_script_timings (script_id, display_name, started_at, ended_at, exit_code, ran_on_start, blocked_login)
VALUES
    ((SELECT id FROM workspace_agent_scripts LIMIT 1), 'Startup Script', NOW() - INTERVAL '1 hour 55 minutes', NOW() - INTERVAL '1 hour 50 minutes', 0, true, false);
