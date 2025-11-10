INSERT INTO public.task_workspace_apps VALUES (
    'f5a1c3e4-8b2d-4f6a-9d7e-2a8b5c9e1f3d', -- task_id
    NULL,                                   -- workspace_agent_id
    NULL,                                   -- workspace_app_id
    99                                      -- workspace_build_number
) ON CONFLICT DO NOTHING;
