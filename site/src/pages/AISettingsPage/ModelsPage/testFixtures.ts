import type { ChatModelConfig, ChatProviderConfig } from "#/api/typesGenerated";
import type { ProviderState } from "#/modules/aiModels/providerStates";

const now = "2026-02-18T12:00:00.000Z";

const mockOpenAIProviderConfig: ChatProviderConfig = {
	id: "prov-openai",
	provider: "openai",
	display_name: "OpenAI",
	enabled: true,
	has_api_key: true,
	central_api_key_enabled: true,
	allow_user_api_key: false,
	allow_central_api_key_fallback: true,
	base_url: "",
	source: "database",
	created_at: now,
	updated_at: now,
};

const mockAnthropicProviderConfig: ChatProviderConfig = {
	...mockOpenAIProviderConfig,
	id: "prov-anthropic",
	provider: "anthropic",
	display_name: "Anthropic",
};

export const mockGPT5: ChatModelConfig = {
	id: "model-gpt5",
	provider: "openai",
	ai_provider_id: "prov-openai",
	model: "gpt-5",
	display_name: "GPT-5",
	enabled: true,
	is_default: true,
	context_limit: 200000,
	compression_threshold: 70,
	created_at: now,
	updated_at: now,
};

export const mockClaude: ChatModelConfig = {
	...mockGPT5,
	id: "model-claude",
	provider: "anthropic",
	ai_provider_id: "prov-anthropic",
	model: "claude-sonnet-4-5",
	display_name: "Claude Sonnet 4.5",
	is_default: false,
};

export const mockDisabledModel: ChatModelConfig = {
	...mockGPT5,
	id: "model-disabled",
	model: "gpt-4o-mini",
	display_name: "GPT-4o mini",
	is_default: false,
	enabled: false,
	context_limit: 128000,
};

export const mockOpenAIProviderState: ProviderState = {
	key: "prov-openai",
	provider: "openai",
	label: "OpenAI",
	providerConfig: mockOpenAIProviderConfig,
	modelConfigs: [mockGPT5, mockDisabledModel],
	catalogModelCount: 0,
	hasManagedAPIKey: true,
	hasCatalogAPIKey: true,
	hasEffectiveAPIKey: true,
	allowUserAPIKey: false,
	isEnvPreset: false,
	baseURL: "",
};

export const mockAnthropicProviderState: ProviderState = {
	...mockOpenAIProviderState,
	key: "prov-anthropic",
	provider: "anthropic",
	label: "Anthropic",
	providerConfig: mockAnthropicProviderConfig,
	modelConfigs: [mockClaude],
};

export const mockProviderStateWithoutKey: ProviderState = {
	...mockOpenAIProviderState,
	providerConfig: {
		...mockOpenAIProviderConfig,
		has_api_key: false,
		central_api_key_enabled: false,
		allow_user_api_key: false,
	},
	modelConfigs: [],
	hasManagedAPIKey: false,
	hasCatalogAPIKey: false,
	hasEffectiveAPIKey: false,
};
