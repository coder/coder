CREATE TABLE IF NOT EXISTS aibridge_interceptions (
    id UUID PRIMARY KEY,
    initiator_id uuid NOT NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    started_at TIMESTAMP WITH TIME ZONE NOT NULL
);

COMMENT ON TABLE aibridge_interceptions IS 'Audit log of requests intercepted by AI Bridge';
COMMENT ON COLUMN aibridge_interceptions.initiator_id IS 'Relates to a users record, but FK is elided for performance.';

CREATE INDEX idx_aibridge_interceptions_initiator_id ON aibridge_interceptions (initiator_id);

CREATE TABLE IF NOT EXISTS aibridge_token_usages (
    id UUID PRIMARY KEY,
    interception_id UUID NOT NULL,
    provider_response_id TEXT NOT NULL,
    input_tokens BIGINT NOT NULL,
    output_tokens BIGINT NOT NULL,
    metadata JSONB DEFAULT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);

COMMENT ON TABLE aibridge_token_usages IS 'Audit log of tokens used by intercepted requests in AI Bridge';
COMMENT ON COLUMN aibridge_token_usages.provider_response_id IS 'The ID for the response in which the tokens were used, produced by the provider.';

CREATE INDEX idx_aibridge_token_usages_interception_id ON aibridge_token_usages (interception_id);

CREATE INDEX idx_aibridge_token_usages_provider_response_id ON aibridge_token_usages (provider_response_id);

CREATE TABLE IF NOT EXISTS aibridge_user_prompts (
    id UUID PRIMARY KEY,
    interception_id UUID NOT NULL,
    provider_response_id TEXT NOT NULL,
    prompt TEXT NOT NULL,
    metadata JSONB DEFAULT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);

COMMENT ON TABLE aibridge_user_prompts IS 'Audit log of prompts used by intercepted requests in AI Bridge';
COMMENT ON COLUMN aibridge_user_prompts.provider_response_id IS 'The ID for the response to the given prompt, produced by the provider.';

CREATE INDEX idx_aibridge_user_prompts_interception_id ON aibridge_user_prompts (interception_id);

CREATE INDEX idx_aibridge_user_prompts_provider_response_id ON aibridge_user_prompts (provider_response_id);

CREATE TABLE IF NOT EXISTS aibridge_tool_usages (
    id UUID PRIMARY KEY,
    interception_id UUID NOT NULL,
    provider_response_id TEXT NOT NULL,
    server_url TEXT NULL,
    tool TEXT NOT NULL,
    input TEXT NOT NULL,
    injected BOOLEAN NOT NULL DEFAULT FALSE,
    invocation_error TEXT NULL,
    metadata JSONB DEFAULT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);

COMMENT ON TABLE aibridge_tool_usages IS 'Audit log of tool calls in intercepted requests in AI Bridge';
COMMENT ON COLUMN aibridge_tool_usages.provider_response_id IS 'The ID for the response in which the tools were used, produced by the provider.';
COMMENT ON COLUMN aibridge_tool_usages.server_url IS 'The name of the MCP server against which this tool was invoked. May be NULL, in which case the tool was defined by the client, not injected.';
COMMENT ON COLUMN aibridge_tool_usages.injected IS 'Whether this tool was injected; i.e. Bridge injected these tools into the request from an MCP server. If false it means a tool was defined by the client and already existed in the request (MCP or built-in).';
COMMENT ON COLUMN aibridge_tool_usages.invocation_error IS 'Only injected tools are invoked.';

CREATE INDEX idx_aibridge_tool_usages_interception_id ON aibridge_tool_usages (interception_id);

CREATE INDEX idx_aibridge_tool_usagesprovider_response_id ON aibridge_tool_usages (provider_response_id);
