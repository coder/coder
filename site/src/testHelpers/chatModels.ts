import type { ChatModelConfig, ChatProviderConfig } from "#/api/typesGenerated";

const MOCK_TIMESTAMP = "2024-01-01T00:00:00Z";

/**
 * Builds a ChatModelConfig for tests and stories. Defaults model an enabled,
 * non-default OpenAI model; pass overrides for the fields a case cares about.
 * display_name defaults to the (possibly overridden) model identifier.
 */
export const makeChatModelConfig = (
	overrides: Partial<ChatModelConfig> = {},
): ChatModelConfig => {
	const model = overrides.model ?? "gpt-5";
	return {
		id: "model-1",
		provider: "openai",
		model,
		display_name: model,
		enabled: true,
		is_default: false,
		context_limit: 200000,
		compression_threshold: 70,
		created_at: MOCK_TIMESTAMP,
		updated_at: MOCK_TIMESTAMP,
		...overrides,
	};
};

/**
 * Builds a ChatProviderConfig for tests and stories. Defaults to an enabled
 * database-sourced OpenAI provider with a managed API key; pass overrides for
 * the fields a case cares about.
 */
export const makeChatProviderConfig = (
	overrides: Partial<ChatProviderConfig> = {},
): ChatProviderConfig => ({
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
	...overrides,
});
