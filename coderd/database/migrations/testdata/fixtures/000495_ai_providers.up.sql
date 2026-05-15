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
        '8e3c6e18-2b75-4c3f-9b35-9d1c6f4e1a01',
        'openai',
        'openai',
        'OpenAI (Fixture)',
        TRUE,
        FALSE,
        'https://api.openai.com/v1/',
        ''
    ),
    (
        '8e3c6e18-2b75-4c3f-9b35-9d1c6f4e1a02',
        'anthropic',
        'anthropic-bedrock',
        'Anthropic via Bedrock (Fixture)',
        TRUE,
        FALSE,
        'https://bedrock-runtime.us-west-2.amazonaws.com/',
        '{"bedrock_region":"us-west-2","bedrock_model":"global.anthropic.claude-sonnet-4-5-20250929-v1:0","bedrock_access_key":"fixture-bedrock-access-key","bedrock_access_key_secret":"fixture-bedrock-access-key-secret"}'
    ),
    (
        '8e3c6e18-2b75-4c3f-9b35-9d1c6f4e1a03',
        'openai',
        'openai-deleted',
        'OpenAI (Deleted Fixture)',
        FALSE,
        TRUE,
        'https://api.openai.com/v1/',
        ''
    );

INSERT INTO ai_provider_keys (
    id,
    provider_id,
    api_key
) VALUES
    (
        '8e3c6e18-2b75-4c3f-9b35-9d1c6f4e1b01',
        '8e3c6e18-2b75-4c3f-9b35-9d1c6f4e1a01',
        'fixture-openai-key'
    ),
    (
        '8e3c6e18-2b75-4c3f-9b35-9d1c6f4e1b02',
        '8e3c6e18-2b75-4c3f-9b35-9d1c6f4e1a01',
        'fixture-openai-key-failover'
    );
