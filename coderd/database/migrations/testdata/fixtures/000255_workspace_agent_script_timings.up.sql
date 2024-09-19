INSERT INTO workspace_agent_script_timings (script_id, display_name, started_at, ended_at, exit_code, ran_on_start, blocked_login)
VALUES
    ('a810e1ba-0b28-4db2-a5b6-1857ea72bf93', 'Startup Script', NOW() - INTERVAL '1 hour 55 minutes', NOW() - INTERVAL '1 hour 50 minutes', 0, true, false);
