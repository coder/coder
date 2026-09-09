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
        '8e3c6e18-2b75-4c3f-9b35-9d1c6f4e1a04',
        'bedrock',
        'bedrock-inference-profile',
        'Bedrock via Application Inference Profile (Fixture)',
        TRUE,
        FALSE,
        'https://bedrock-runtime.us-west-2.amazonaws.com/',
        '{"_type":"bedrock","_version":1,"region":"us-west-2","model":"arn:aws:bedrock:us-west-2:123456789012:application-inference-profile/fixtureprofile","small_fast_model":"anthropic.claude-3-5-haiku-20241022-v1:0","access_key":"fixture-bedrock-access-key","access_key_secret":"fixture-bedrock-access-key-secret"}'
    );

INSERT INTO ai_provider_bedrock_resolved_models (
    ai_provider_id,
    resolved_model,
    resolved_small_fast_model
) VALUES
    (
        '8e3c6e18-2b75-4c3f-9b35-9d1c6f4e1a04',
        'anthropic.claude-sonnet-4-5-20250929-v1:0',
        'anthropic.claude-3-5-haiku-20241022-v1:0'
    );
