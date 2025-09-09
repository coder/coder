CREATE TABLE IF NOT EXISTS aibridge_interceptions (
    id UUID PRIMARY KEY,
    initiator_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    started_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX idx_aibridge_interceptions_initiator_id ON aibridge_interceptions (initiator_id);

CREATE TABLE IF NOT EXISTS aibridge_token_usages (
    id UUID PRIMARY KEY,
    interception_id UUID NOT NULL REFERENCES aibridge_interceptions (id) ON DELETE CASCADE,
    provider_response_id TEXT NOT NULL, -- The ID for the response in which the tokens were used, produced by the provider.
    input_tokens BIGINT NOT NULL,
    output_tokens BIGINT NOT NULL,
    metadata JSONB DEFAULT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX idx_aibridge_token_usages_interception_id ON aibridge_token_usages (interception_id);

CREATE INDEX idx_aibridge_token_usages_provider_response_id ON aibridge_token_usages (provider_response_id);

CREATE TABLE IF NOT EXISTS aibridge_user_prompts (
    id UUID PRIMARY KEY,
    interception_id UUID NOT NULL REFERENCES aibridge_interceptions (id) ON DELETE CASCADE,
    provider_response_id TEXT NOT NULL, -- The ID for the response in which the tokens were used, produced by the provider.
    prompt TEXT NOT NULL,
    metadata JSONB DEFAULT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX idx_aibridge_user_prompts_interception_id ON aibridge_user_prompts (interception_id);

CREATE INDEX idx_aibridge_user_prompts_provider_response_id ON aibridge_user_prompts (provider_response_id);

CREATE TABLE IF NOT EXISTS aibridge_tool_usages (
    id UUID PRIMARY KEY,
    interception_id UUID NOT NULL REFERENCES aibridge_interceptions (id) ON DELETE CASCADE,
    provider_response_id TEXT NOT NULL, -- The ID for the response in which the tokens were used, produced by the provider.
    server_url TEXT NULL, -- The name of the MCP server against which this tool was invoked. May be NULL, in which case the tool was defined by the client, not injected.
    tool TEXT NOT NULL,
    input TEXT NOT NULL,
    injected BOOLEAN NOT NULL DEFAULT FALSE, -- Whether this tool was injected; i.e. Bridge injected these tools into the request from an MCP server. If false it means a tool was defined by the client and already existed in the request (MCP or built-in).
    metadata JSONB DEFAULT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX idx_aibridge_tool_usages_interception_id ON aibridge_tool_usages (interception_id);

CREATE INDEX idx_aibridge_tool_usagesprovider_response_id ON aibridge_tool_usages (provider_response_id);
