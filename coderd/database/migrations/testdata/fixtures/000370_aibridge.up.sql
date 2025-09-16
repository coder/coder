INSERT INTO
    aibridge_interceptions (
        id,
        initiator_id,
        provider,
        model,
        started_at
    )
VALUES (
        'be003e1e-b38f-43bf-847d-928074dd0aa8',
        '30095c71-380b-457a-8995-97b8ee6e5307',
        'openai',
        'gpt-5',
        '2025-09-15 12:45:13.921148+00'
    );

INSERT INTO
    aibridge_token_usages (
        id,
        interception_id,
        provider_response_id,
        input_tokens,
        output_tokens,
        metadata,
        created_at
    )
VALUES (
        'c56ca89d-af65-47b0-871f-0b9cd2af6575',
        'be003e1e-b38f-43bf-847d-928074dd0aa8',
        'chatcmpl-CG2s28QlpKIoooUtXuLTmGbdtyS1k',
        10950,
        118,
        '{"prompt_audio": 0, "prompt_cached": 5376, "completion_audio": 0, "completion_reasoning": 64, "completion_accepted_prediction": 0, "completion_rejected_prediction": 0}',
        '2025-09-15 12:45:21.674413+00'
    );

INSERT INTO
    aibridge_tool_usages (
        id,
        interception_id,
        provider_response_id,
        server_url,
        tool,
        input,
        injected,
        invocation_error,
        metadata,
        created_at
    )
VALUES (
        '613b4cfa-a257-4e88-99e6-4d2e99ea25f0',
        'be003e1e-b38f-43bf-847d-928074dd0aa8',
        'chatcmpl-CG2ryDxMp6n53aMjgo7P6BHno3fTr',
        'http://localhost:3000/api/experimental/mcp/http',
        'coder_list_workspaces',
        '{}',
        true,
        NULL,
        '{}',
        '2025-09-15 12:45:17.65274+00'
    );

INSERT INTO
    aibridge_user_prompts (
        id,
        interception_id,
        provider_response_id,
        prompt,
        metadata,
        created_at
    )
VALUES (
        'ac1ea8c3-5109-4105-9b62-489fca220ef7',
        'be003e1e-b38f-43bf-847d-928074dd0aa8',
        'chatcmpl-CG2s28QlpKIoooUtXuLTmGbdtyS1k',
        'how many workspaces do i have',
        '{}',
        '2025-09-15 12:45:21.674335+00'
    );
