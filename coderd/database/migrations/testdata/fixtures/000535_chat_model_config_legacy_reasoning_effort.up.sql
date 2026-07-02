-- Chat model configs carrying legacy per-provider reasoning effort
-- values inside options. Inserted at 000535 so the 000536 data
-- migration rewrites them into the top-level reasoning_effort config
-- ({default, max}) and strips the legacy keys.
INSERT INTO ai_providers (
    id,
    type,
    name,
    display_name,
    enabled,
    deleted,
    base_url,
    settings
) VALUES
    (
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6e01',
        'openai',
        'openai-effort-fixture',
        'OpenAI (Reasoning Effort Fixture)',
        TRUE,
        FALSE,
        'https://api.openai.com/v1/',
        ''
    ),
    (
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6e02',
        'anthropic',
        'anthropic-effort-fixture',
        'Anthropic (Reasoning Effort Fixture)',
        TRUE,
        FALSE,
        'https://api.anthropic.com/',
        ''
    );

INSERT INTO chat_model_configs (
    id,
    model,
    display_name,
    enabled,
    is_default,
    deleted,
    context_limit,
    compression_threshold,
    options,
    ai_provider_id,
    created_at,
    updated_at
) VALUES
    (
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6f01',
        'gpt-5.1',
        'GPT-5.1 (Legacy Effort)',
        TRUE,
        FALSE,
        FALSE,
        200000,
        70,
        '{"provider_options": {"openai": {"reasoning_effort": "high", "reasoning_summary": "auto"}}}',
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6e01',
        '2024-01-01 00:00:00+00',
        '2024-01-01 00:00:00+00'
    ),
    (
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6f02',
        'claude-opus-4-6',
        'Claude Opus (Legacy Effort)',
        TRUE,
        FALSE,
        FALSE,
        200000,
        70,
        '{"provider_options": {"anthropic": {"effort": "max", "send_reasoning": true}}}',
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6e02',
        '2024-01-01 00:00:00+00',
        '2024-01-01 00:00:00+00'
    ),
    (
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6f03',
        'gpt-5.1-empty-effort',
        'GPT-5.1 (Empty Legacy Effort)',
        TRUE,
        FALSE,
        FALSE,
        200000,
        70,
        '{"provider_options": {"openai": {"reasoning_effort": ""}}}',
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6e01',
        '2024-01-01 00:00:00+00',
        '2024-01-01 00:00:00+00'
    );
