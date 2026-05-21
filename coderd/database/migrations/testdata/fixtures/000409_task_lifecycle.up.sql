INSERT INTO task_snapshots VALUES (
    'f5a1c3e4-8b2d-4f6a-9d7e-2a8b5c9e1f3d', -- task_id (references existing task from 000366)
    '{
        "format": "agentapi",
        "data": {
            "messages": [
                {"id": 0, "type": "output", "content": "Starting task execution...", "time": "2024-11-02T13:10:05Z"},
                {"id": 1, "type": "input", "content": "Create a React component for tasks", "time": "2024-11-02T13:10:06Z"},
                {"id": 2, "type": "output", "content": "Creating component structure...", "time": "2024-11-02T13:10:10Z"}
            ],
            "truncated": false,
            "total_count": 3
        }
    }'::JSONB,                              -- log_snapshot
    '2024-11-02 13:15:00.000000+02'         -- log_snapshot_at
) ON CONFLICT DO NOTHING;
