CREATE TABLE aibridge_model_thoughts (
    interception_id UUID NOT NULL,
    content TEXT NOT NULL,
    metadata jsonb,
    created_at TIMESTAMPTZ NOT NULL
);

COMMENT ON TABLE aibridge_model_thoughts IS 'Audit log of model thinking in intercepted requests in AI Bridge';

CREATE INDEX idx_aibridge_model_thoughts_interception_id ON aibridge_model_thoughts(interception_id);
