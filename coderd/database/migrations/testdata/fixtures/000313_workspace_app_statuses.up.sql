INSERT INTO workspace_app_statuses (
    id,
    created_at,
    agent_id,
    app_id,
    workspace_id,
    state,
    needs_user_attention,
    message
) VALUES (
    gen_random_uuid(),
    NOW(),
    '7a1ce5f8-8d00-431c-ad1b-97a846512804',
	'36b65d0c-042b-4653-863a-655ee739861c',
	'3a9a1feb-e89d-457c-9d53-ac751b198ebe',
    'working',
    false,
    'Creating SQL queries for test data!'
);
