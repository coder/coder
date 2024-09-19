INSERT INTO workspace_agent_script_timings (agent_id, display_name, started_at, ended_at, exit_code, ran_on_start, blocked_login)
VALUES
    ('45e89705-e09d-4850-bcec-f9a937f5d78d', 'Startup Script', NOW() - INTERVAL '1 hour 55 minutes', NOW() - INTERVAL '1 hour 50 minutes', 0, true, false);
