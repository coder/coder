INSERT INTO chat_organization_model_overrides (
    organization_id,
    context,
    model_config_id,
    reasoning_effort
)
SELECT
    organization_id,
    'general',
    id,
    'high'
FROM chat_model_configs
WHERE id = '580c0001-0000-4000-8000-000000000001';

INSERT INTO chat_user_model_overrides (
    user_id,
    organization_id,
    context,
    mode
)
SELECT
    u.id,
    o.id,
    'root',
    'chat_default'
FROM users u
CROSS JOIN organizations o
WHERE o.is_default
ORDER BY u.id
LIMIT 1;
