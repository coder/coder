-- Inject test data for Boundary Audit Logs and AI Bridge Interceptions
-- Usage: psql "postgres://coder:coder@localhost:5432/coder?sslmode=disable" -f scripts/inject-test-audit-data.sql

DO $$
DECLARE
    v_org_id uuid;
    v_user_id uuid;
    v_workspace_id uuid;
    v_workspace_name text;
    v_api_key_id text;
    v_agent_id uuid := gen_random_uuid();
    v_interception_id uuid;
BEGIN
    -- Get first organization
    SELECT id INTO v_org_id FROM organizations LIMIT 1;
    IF v_org_id IS NULL THEN
        RAISE EXCEPTION 'No organizations found. Start coder server first.';
    END IF;
    RAISE NOTICE 'Using organization: %', v_org_id;

    -- Get first user
    SELECT id INTO v_user_id FROM users LIMIT 1;
    IF v_user_id IS NULL THEN
        RAISE EXCEPTION 'No users found. Create a user first.';
    END IF;
    RAISE NOTICE 'Using user: %', v_user_id;

    -- Get first workspace (or use placeholder if none)
    SELECT id, name INTO v_workspace_id, v_workspace_name FROM workspaces LIMIT 1;
    IF v_workspace_id IS NULL THEN
        v_workspace_id := gen_random_uuid();
        v_workspace_name := 'test-workspace';
        RAISE NOTICE 'No workspaces found, using generated ID: %', v_workspace_id;
    ELSE
        RAISE NOTICE 'Using workspace: % (%)', v_workspace_name, v_workspace_id;
    END IF;

    -- Get first API key (for AI Bridge)
    SELECT id INTO v_api_key_id FROM api_keys WHERE user_id = v_user_id LIMIT 1;
    IF v_api_key_id IS NULL THEN
        SELECT id INTO v_api_key_id FROM api_keys LIMIT 1;
    END IF;
    RAISE NOTICE 'Using API key: %', COALESCE(v_api_key_id, 'none');

    -------------------------------------------------------------------------
    -- BOUNDARY AUDIT LOGS
    -------------------------------------------------------------------------
    RAISE NOTICE 'Inserting boundary audit logs...';

    -- Recent allowed requests
    INSERT INTO boundary_audit_logs (id, time, organization_id, workspace_id, workspace_owner_id, workspace_name, agent_id, agent_name, resource_type, resource, operation, decision)
    VALUES
        (gen_random_uuid(), NOW() - interval '5 minutes', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'network', 'https://api.openai.com/v1/chat/completions', 'POST', 'allow'),
        (gen_random_uuid(), NOW() - interval '10 minutes', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'network', 'https://api.anthropic.com/v1/messages', 'POST', 'allow'),
        (gen_random_uuid(), NOW() - interval '15 minutes', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'network', 'https://github.com/api/v3/repos/coder/coder', 'GET', 'allow'),
        (gen_random_uuid(), NOW() - interval '20 minutes', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'network', 'https://registry.npmjs.org/typescript', 'GET', 'allow'),
        (gen_random_uuid(), NOW() - interval '25 minutes', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'network', 'https://pypi.org/simple/requests', 'GET', 'allow');

    -- Recent denied requests
    INSERT INTO boundary_audit_logs (id, time, organization_id, workspace_id, workspace_owner_id, workspace_name, agent_id, agent_name, resource_type, resource, operation, decision)
    VALUES
        (gen_random_uuid(), NOW() - interval '7 minutes', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'network', 'https://evil-malware-site.com/payload', 'GET', 'deny'),
        (gen_random_uuid(), NOW() - interval '12 minutes', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'network', 'https://data-exfiltration.io/upload', 'POST', 'deny'),
        (gen_random_uuid(), NOW() - interval '18 minutes', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'network', 'http://suspicious-ip.ru:8080/beacon', 'GET', 'deny'),
        (gen_random_uuid(), NOW() - interval '30 minutes', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'file', '/etc/shadow', 'read', 'deny'),
        (gen_random_uuid(), NOW() - interval '35 minutes', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'file', '/root/.ssh/id_rsa', 'read', 'deny');

    -- Older logs (yesterday)
    INSERT INTO boundary_audit_logs (id, time, organization_id, workspace_id, workspace_owner_id, workspace_name, agent_id, agent_name, resource_type, resource, operation, decision)
    VALUES
        (gen_random_uuid(), NOW() - interval '1 day 2 hours', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'network', 'https://api.github.com/user', 'GET', 'allow'),
        (gen_random_uuid(), NOW() - interval '1 day 3 hours', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'network', 'https://cdn.jsdelivr.net/npm/react', 'GET', 'allow'),
        (gen_random_uuid(), NOW() - interval '1 day 4 hours', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'network', 'https://crypto-miner.xyz/mine', 'POST', 'deny'),
        (gen_random_uuid(), NOW() - interval '2 days 1 hour', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'network', 'https://slack.com/api/chat.postMessage', 'POST', 'allow'),
        (gen_random_uuid(), NOW() - interval '2 days 2 hours', v_org_id, v_workspace_id, v_user_id, v_workspace_name, v_agent_id, 'main', 'network', 'https://webhook.site/unknown-endpoint', 'POST', 'deny');

    RAISE NOTICE 'Inserted 15 boundary audit logs';

    -------------------------------------------------------------------------
    -- AI BRIDGE INTERCEPTIONS
    -------------------------------------------------------------------------
    IF v_api_key_id IS NOT NULL THEN
        RAISE NOTICE 'Inserting AI Bridge interceptions...';

        -- OpenAI interceptions
        v_interception_id := gen_random_uuid();
        INSERT INTO aibridge_interceptions (id, api_key_id, initiator_id, provider, model, metadata, started_at, ended_at)
        VALUES (v_interception_id, v_api_key_id, v_user_id, 'openai', 'gpt-4o', '{"source": "test"}', NOW() - interval '10 minutes', NOW() - interval '9 minutes');
        INSERT INTO aibridge_token_usages (id, interception_id, provider_response_id, input_tokens, output_tokens, metadata, created_at)
        VALUES (gen_random_uuid(), v_interception_id, 'chatcmpl-test-1', 2500, 800, '{}', NOW() - interval '9 minutes');
        INSERT INTO aibridge_user_prompts (id, interception_id, provider_response_id, prompt, metadata, created_at)
        VALUES (gen_random_uuid(), v_interception_id, 'chatcmpl-test-1', 'Help me write a function to sort an array', '{}', NOW() - interval '10 minutes');

        v_interception_id := gen_random_uuid();
        INSERT INTO aibridge_interceptions (id, api_key_id, initiator_id, provider, model, metadata, started_at, ended_at)
        VALUES (v_interception_id, v_api_key_id, v_user_id, 'openai', 'gpt-4o-mini', '{"source": "test"}', NOW() - interval '30 minutes', NOW() - interval '29 minutes');
        INSERT INTO aibridge_token_usages (id, interception_id, provider_response_id, input_tokens, output_tokens, metadata, created_at)
        VALUES (gen_random_uuid(), v_interception_id, 'chatcmpl-test-2', 500, 150, '{}', NOW() - interval '29 minutes');

        -- Anthropic interceptions
        v_interception_id := gen_random_uuid();
        INSERT INTO aibridge_interceptions (id, api_key_id, initiator_id, provider, model, metadata, started_at, ended_at)
        VALUES (v_interception_id, v_api_key_id, v_user_id, 'anthropic', 'claude-3-5-sonnet-20241022', '{"source": "test"}', NOW() - interval '1 hour', NOW() - interval '59 minutes');
        INSERT INTO aibridge_token_usages (id, interception_id, provider_response_id, input_tokens, output_tokens, metadata, created_at)
        VALUES (gen_random_uuid(), v_interception_id, 'msg-test-1', 3500, 1200, '{}', NOW() - interval '59 minutes');
        INSERT INTO aibridge_user_prompts (id, interception_id, provider_response_id, prompt, metadata, created_at)
        VALUES (gen_random_uuid(), v_interception_id, 'msg-test-1', 'Review this code for security vulnerabilities', '{}', NOW() - interval '1 hour');
        INSERT INTO aibridge_tool_usages (id, interception_id, provider_response_id, tool, server_url, input, injected, invocation_error, metadata, created_at)
        VALUES (gen_random_uuid(), v_interception_id, 'msg-test-1', 'read_file', 'mcp://filesystem', '{"path": "/src/main.go"}', false, '', '{}', NOW() - interval '59 minutes');

        v_interception_id := gen_random_uuid();
        INSERT INTO aibridge_interceptions (id, api_key_id, initiator_id, provider, model, metadata, started_at, ended_at)
        VALUES (v_interception_id, v_api_key_id, v_user_id, 'anthropic', 'claude-3-5-haiku-20241022', '{"source": "test"}', NOW() - interval '2 hours', NOW() - interval '1 hour 58 minutes');
        INSERT INTO aibridge_token_usages (id, interception_id, provider_response_id, input_tokens, output_tokens, metadata, created_at)
        VALUES (gen_random_uuid(), v_interception_id, 'msg-test-2', 800, 200, '{}', NOW() - interval '1 hour 58 minutes');

        -- Older interceptions
        v_interception_id := gen_random_uuid();
        INSERT INTO aibridge_interceptions (id, api_key_id, initiator_id, provider, model, metadata, started_at, ended_at)
        VALUES (v_interception_id, v_api_key_id, v_user_id, 'openai', 'gpt-4o', '{"source": "test"}', NOW() - interval '1 day', NOW() - interval '1 day' + interval '2 minutes');
        INSERT INTO aibridge_token_usages (id, interception_id, provider_response_id, input_tokens, output_tokens, metadata, created_at)
        VALUES (gen_random_uuid(), v_interception_id, 'chatcmpl-test-3', 5000, 2000, '{}', NOW() - interval '1 day' + interval '2 minutes');

        RAISE NOTICE 'Inserted 5 AI Bridge interceptions with token usage, prompts, and tool usage';
    ELSE
        RAISE NOTICE 'Skipping AI Bridge data - no API keys found. Create an API key in the UI first.';
    END IF;

    RAISE NOTICE 'Done! Test data injected successfully.';
END $$;

-- Show summary
SELECT 'Boundary Audit Logs' as table_name, COUNT(*) as count FROM boundary_audit_logs
UNION ALL
SELECT 'AI Bridge Interceptions', COUNT(*) FROM aibridge_interceptions
UNION ALL
SELECT 'AI Bridge Token Usages', COUNT(*) FROM aibridge_token_usages
UNION ALL
SELECT 'AI Bridge User Prompts', COUNT(*) FROM aibridge_user_prompts
UNION ALL
SELECT 'AI Bridge Tool Usages', COUNT(*) FROM aibridge_tool_usages;
