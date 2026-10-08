INSERT INTO agents_acp_sessions (
    id, organization_id, chat_id, workspace_id,
    working_directory, harness_slug, harness_display_name, session_id
)
SELECT
    'bb604160-03ad-4c52-b188-ecae31e1f9fc', chats.organization_id, chats.id, workspaces.id,
    '/home/coder/project', 'test-harness', 'Test Harness', 'native-session'
FROM chats CROSS JOIN workspaces
LIMIT 1;
