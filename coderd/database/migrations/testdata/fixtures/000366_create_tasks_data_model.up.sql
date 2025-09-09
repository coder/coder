INSERT INTO public.tasks VALUES (
    'f5a1c3e4-8b2d-4f6a-9d7e-2a8b5c9e1f3d', -- id
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1', -- organization_id
    '30095c71-380b-457a-8995-97b8ee6e5307', -- owner_id
    'Test Task 1',                          -- name
    '3a9a1feb-e89d-457c-9d53-ac751b198ebe', -- workspace_id
    '920baba5-4c64-4686-8b7d-d1bef5683eae', -- template_version_id
    '{}'::JSONB,                            -- template_parameters
    'Create a React component for tasks',   -- prompt
    '2024-11-02 13:10:00.000000+02',        -- created_at
    NULL                                    -- deleted_at
) ON CONFLICT DO NOTHING;

INSERT INTO public.task_workspace_apps VALUES (
    'f5a1c3e4-8b2d-4f6a-9d7e-2a8b5c9e1f3d', -- task_id
    'a8c0b8c5-c9a8-4f33-93a4-8142e6858244', -- workspace_build_id
    '8fa17bbd-c48c-44c7-91ae-d4acbc755fad', -- workspace_agent_id
    'a47965a2-0a25-4810-8cc9-d283c86ab34c'  -- workspace_app_id
) ON CONFLICT DO NOTHING;
