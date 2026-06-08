INSERT INTO ai_gateway_policies (id, name, display_name, kind) VALUES
    ('9f1c6e18-2b75-4c3f-9b35-9d1c6f4e1a01', 'model-allowlist', 'Model Allowlist (Fixture)', 'decide');

INSERT INTO ai_gateway_policy_versions (
    id, policy_id, version_number, rego, input_schema_version, output_schema_version, description
) VALUES
    (
        '9f1c6e18-2b75-4c3f-9b35-9d1c6f4e1b01',
        '9f1c6e18-2b75-4c3f-9b35-9d1c6f4e1a01',
        1,
        'default verdict := "ALLOW"',
        1,
        1,
        'Initial version (fixture).'
    );

UPDATE ai_gateway_policies
SET active_version_id = '9f1c6e18-2b75-4c3f-9b35-9d1c6f4e1b01'
WHERE id = '9f1c6e18-2b75-4c3f-9b35-9d1c6f4e1a01';

INSERT INTO ai_gateway_pipelines (id, provider_id, enabled) VALUES
    ('9f1c6e18-2b75-4c3f-9b35-9d1c6f4e1c01', '8e3c6e18-2b75-4c3f-9b35-9d1c6f4e1a01', TRUE);

INSERT INTO ai_gateway_pipeline_versions (id, pipeline_id, version_number) VALUES
    ('9f1c6e18-2b75-4c3f-9b35-9d1c6f4e1d01', '9f1c6e18-2b75-4c3f-9b35-9d1c6f4e1c01', 1);

INSERT INTO ai_gateway_pipeline_version_policies (
    id, pipeline_version_id, policy_version_id, hook, kind, fail_mode
) VALUES
    (
        '9f1c6e18-2b75-4c3f-9b35-9d1c6f4e1e01',
        '9f1c6e18-2b75-4c3f-9b35-9d1c6f4e1d01',
        '9f1c6e18-2b75-4c3f-9b35-9d1c6f4e1b01',
        'pre_req',
        'decide',
        'fail_closed'
    );

UPDATE ai_gateway_pipelines
SET active_version_id = '9f1c6e18-2b75-4c3f-9b35-9d1c6f4e1d01'
WHERE id = '9f1c6e18-2b75-4c3f-9b35-9d1c6f4e1c01';
