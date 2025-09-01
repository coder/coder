CREATE TABLE IF NOT EXISTS aibridge_sessions (
    id UUID PRIMARY KEY,
	initiator_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

CREATE INDEX idx_aibridge_sessions_provider ON aibridge_sessions(provider);
CREATE INDEX idx_aibridge_sessions_model ON aibridge_sessions(model);

CREATE TABLE IF NOT EXISTS aibridge_token_usages (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES aibridge_sessions(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    input_tokens BIGINT NOT NULL,
    output_tokens BIGINT NOT NULL,
    metadata JSONB DEFAULT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

CREATE INDEX idx_aibridge_token_usages_session_id ON aibridge_token_usages(session_id);
CREATE INDEX idx_aibridge_token_usages_session_provider_id ON aibridge_token_usages(session_id, provider_id);

CREATE TABLE IF NOT EXISTS aibridge_user_prompts (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES aibridge_sessions(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    prompt TEXT NOT NULL,
    metadata JSONB DEFAULT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

CREATE INDEX idx_aibridge_user_prompts_session_id ON aibridge_user_prompts(session_id);
CREATE INDEX idx_aibridge_user_prompts_session_provider_id ON aibridge_user_prompts(session_id, provider_id);

CREATE TABLE IF NOT EXISTS aibridge_tool_usages (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES aibridge_sessions(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    tool TEXT NOT NULL,
    input TEXT NOT NULL,
    injected BOOLEAN NOT NULL DEFAULT FALSE,
    metadata JSONB DEFAULT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

CREATE INDEX idx_aibridge_tool_usages_session_id ON aibridge_tool_usages(session_id);
CREATE INDEX idx_aibridge_tool_usages_tool ON aibridge_tool_usages(tool);
CREATE INDEX idx_aibridge_tool_usages_session_provider_id ON aibridge_tool_usages(session_id, provider_id);
