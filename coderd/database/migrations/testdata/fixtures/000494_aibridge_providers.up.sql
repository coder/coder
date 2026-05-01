INSERT INTO ai_providers (
    id,
    type,
    name,
    display_name,
    enabled,
    deleted,
    base_url,
    api_key,
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
        'fixture-openai-key',
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
        'fixture-bedrock-access-key',
        '{"bedrock_region":"us-west-2","bedrock_model":"global.anthropic.claude-sonnet-4-5-20250929-v1:0"}'
    ),
    (
        '8e3c6e18-2b75-4c3f-9b35-9d1c6f4e1a03',
        'openai',
        'openai-deleted',
        'OpenAI (Deleted Fixture)',
        FALSE,
        TRUE,
        'https://api.openai.com/v1/',
        '',
        ''
    );
