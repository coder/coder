import type { ChatModelConfig, ChatProviderConfig } from "#/api/typesGenerated";

const MOCK_TIMESTAMP = "2024-01-01T00:00:00Z";

/**
 * A ChatModelConfig for tests and stories: an enabled, non-default OpenAI
 * model. Spread it and override the fields a case cares about, e.g.
 * `{ ...MockChatModelConfig, model: "gpt-4o", display_name: "GPT-4o" }`.
 */
export const MockChatModelConfig: ChatModelConfig = {
	id: "model-1",
	provider: "openai",
	model: "gpt-5",
	display_name: "gpt-5",
	enabled: true,
	is_default: false,
	context_limit: 200000,
	compression_threshold: 70,
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

/**
 * A ChatProviderConfig for tests and stories: an enabled, database-sourced
 * OpenAI provider with a managed API key. Spread it and override the fields a
 * case cares about.
 */
export const MockChatProviderConfig: ChatProviderConfig = {
	id: "provider-1",
	provider: "openai",
	display_name: "OpenAI",
	enabled: true,
	has_api_key: true,
	central_api_key_enabled: true,
	allow_user_api_key: false,
	allow_central_api_key_fallback: true,
	base_url: "",
	source: "database",
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};
