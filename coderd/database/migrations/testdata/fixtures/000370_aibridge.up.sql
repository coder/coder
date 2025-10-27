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
        '30095c71-380b-457a-8995-97b8ee6e5307', -- admin@coder.com, from 000022_initial_v0.6.6.up.sql
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

-- For a later migration, we'll add an invalid interception without a valid
-- initiator_id.
INSERT INTO
    aibridge_interceptions (
        id,
        initiator_id,
        provider,
        model,
        started_at
    )
VALUES (
        'c6d29c6e-26a3-4137-bb2e-9dfeef3c1c26',
        'cab8d56a-8922-4999-81a9-046b43ac1312', -- user does not exist
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
        '5650db6c-0b7c-49e3-bb26-9b2ba0107e11',
        'c6d29c6e-26a3-4137-bb2e-9dfeef3c1c26',
        'chatcmpl-CG2s28QlpKIoooUtXuLTmGbdtyS1k',
        10950,
        118,
        '{}',
        '2025-09-15 12:45:21.674413+00'
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
        '1e76cb5b-7c34-4160-b604-a4256f856169',
        'c6d29c6e-26a3-4137-bb2e-9dfeef3c1c26',
        'chatcmpl-CG2s28QlpKIoooUtXuLTmGbdtyS1k',
        'how many workspaces do i have',
        '{}',
        '2025-09-15 12:45:21.674335+00'
    );
INSERT INTO
    aibridge_tool_usages (
        id,
        interception_id,
        provider_response_id,
        tool,
        server_url,
        input,
        injected,
        invocation_error,
        metadata,
        created_at
    )
VALUES (
        '351b440f-d605-4f37-8ceb-011f0377b695',
        'c6d29c6e-26a3-4137-bb2e-9dfeef3c1c26',
        'chatcmpl-CG2s28QlpKIoooUtXuLTmGbdtyS1k',
        'coder_list_workspaces',
        'http://localhost:3000/api/experimental/mcp/http',
        '{}',
        true,
        NULL,
        '{}',
        '2025-09-15 12:45:21.674413+00'
    );
