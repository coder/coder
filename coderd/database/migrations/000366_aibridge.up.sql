CREATE TABLE IF NOT EXISTS aibridge_sessions (
    id UUID PRIMARY KEY,
	initiator_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    started_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX idx_aibridge_sessions_initiator_id ON aibridge_sessions(initiator_id);
CREATE INDEX idx_aibridge_sessions_provider ON aibridge_sessions(provider);
CREATE INDEX idx_aibridge_sessions_model ON aibridge_sessions(model);

CREATE OR REPLACE FUNCTION check_user_can_create_session()
RETURNS TRIGGER AS $$
BEGIN
    -- Check if the user exists and is not deleted or a system user.
    IF EXISTS (
        SELECT 1 FROM users
        WHERE id = NEW.initiator_id
        AND (deleted = true OR is_system = true)
    ) THEN
        RAISE EXCEPTION 'Cannot create session: user is deleted or is a system user';
    END IF;

    -- If user doesn't exist at all, the foreign key constraint will handle it.
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER validate_user_before_session
    BEFORE INSERT OR UPDATE OF initiator_id
    ON aibridge_sessions
    FOR EACH ROW
    EXECUTE FUNCTION check_user_can_create_session();

CREATE TABLE IF NOT EXISTS aibridge_token_usages (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES aibridge_sessions(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL, -- The ID for the session in which the tokens were used, produced by the provider.
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
    provider_id TEXT NOT NULL, -- The ID for the session in which the tokens were used, produced by the provider.
    prompt TEXT NOT NULL,
    metadata JSONB DEFAULT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

CREATE INDEX idx_aibridge_user_prompts_session_id ON aibridge_user_prompts(session_id);
CREATE INDEX idx_aibridge_user_prompts_session_provider_id ON aibridge_user_prompts(session_id, provider_id);

CREATE TABLE IF NOT EXISTS aibridge_tool_usages (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES aibridge_sessions(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL, -- The ID for the session in which the tokens were used, produced by the provider.
    tool TEXT NOT NULL,
    input TEXT NOT NULL,
    injected BOOLEAN NOT NULL DEFAULT FALSE, -- Whether this tool was injected or was defined by the client.
    metadata JSONB DEFAULT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

CREATE INDEX idx_aibridge_tool_usages_session_id ON aibridge_tool_usages(session_id);
CREATE INDEX idx_aibridge_tool_usages_tool ON aibridge_tool_usages(tool);
CREATE INDEX idx_aibridge_tool_usages_session_provider_id ON aibridge_tool_usages(session_id, provider_id);
