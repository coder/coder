import type {
	AIModelPrice,
	ChatModelConfig,
	ChatProviderConfig,
} from "#/api/typesGenerated";
import type { ProviderState } from "#/modules/aiModels/providerStates";

const now = "2026-02-18T12:00:00.000Z";

const MockOpenAIProviderConfig: ChatProviderConfig = {
	id: "prov-openai",
	provider: "openai",
	display_name: "OpenAI",
	icon: "",
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

const MockAnthropicProviderConfig: ChatProviderConfig = {
	...MockOpenAIProviderConfig,
	id: "prov-anthropic",
	provider: "anthropic",
	display_name: "Anthropic",
};

export const mockGPT5: ChatModelConfig = {
	id: "model-gpt5",
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

export const mockGPT5Pricing: AIModelPrice = {
	provider: "openai",
	model: mockGPT5.model,
	input_price: 1_250_000,
	output_price: 10_000_000,
	cache_read_price: 125_000,
	cache_write_price: null,
	is_default: true,
	created_at: now,
	updated_at: now,
};

export const mockCustomModel: ChatModelConfig = {
	...mockGPT5,
	id: "model-custom",
	model: "custom-model",
	display_name: "Custom Model",
	is_default: false,
};

export const mockCustomModelPricing: AIModelPrice = {
	...mockGPT5Pricing,
	model: mockCustomModel.model,
	input_price: 2_500_000,
	output_price: 8_000_000,
	cache_read_price: null,
	cache_write_price: 3_125_000,
	is_default: false,
};

export const mockRefetchedCustomModelPricing: AIModelPrice = {
	...mockCustomModelPricing,
	input_price: 5_500_000,
};

export const mockClaude: ChatModelConfig = {
	...mockGPT5,
	id: "model-claude",
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

export const MockOpenAIProviderState: ProviderState = {
	key: "prov-openai",
	provider: "openai",
	label: "OpenAI",
	providerConfig: MockOpenAIProviderConfig,
	modelConfigs: [mockGPT5, mockDisabledModel],
	catalogModelCount: 0,
	hasManagedAPIKey: true,
	hasCatalogAPIKey: true,
	hasEffectiveAPIKey: true,
	allowUserAPIKey: false,
	isEnvPreset: false,
	baseURL: "",
};

export const MockOpenAICompatProviderState: ProviderState = {
	...MockOpenAIProviderState,
	key: "prov-openai-compat",
	provider: "openai-compat",
	label: "OpenAI-compatible",
	providerConfig: {
		...MockOpenAIProviderConfig,
		id: "prov-openai-compat",
		provider: "openai-compat",
		display_name: "OpenAI-compatible",
	},
	modelConfigs: [
		{
			...mockCustomModel,
			ai_provider_id: "prov-openai-compat",
		},
	],
};

export const MockAnthropicProviderState: ProviderState = {
	...MockOpenAIProviderState,
	key: "prov-anthropic",
	provider: "anthropic",
	label: "Anthropic",
	providerConfig: MockAnthropicProviderConfig,
	modelConfigs: [mockClaude],
};

const MockBedrockProviderConfig: ChatProviderConfig = {
	...MockOpenAIProviderConfig,
	id: "prov-bedrock",
	provider: "bedrock",
	display_name: "AWS Bedrock",
};

export const mockBedrockClaude: ChatModelConfig = {
	...mockClaude,
	id: "model-bedrock-claude",
	ai_provider_id: "prov-bedrock",
	model: "anthropic.claude-sonnet-4-5",
	display_name: "Claude Sonnet 4.5 (Bedrock)",
};

export const MockBedrockProviderState: ProviderState = {
	...MockOpenAIProviderState,
	key: "prov-bedrock",
	provider: "bedrock",
	label: "AWS Bedrock",
	providerConfig: MockBedrockProviderConfig,
	modelConfigs: [mockBedrockClaude],
};

export const MockAzureProviderState: ProviderState = {
	...MockOpenAIProviderState,
	key: "prov-azure",
	provider: "azure",
	label: "Azure OpenAI",
	providerConfig: {
		...MockOpenAIProviderConfig,
		id: "prov-azure",
		provider: "azure",
		display_name: "Azure OpenAI",
	},
	modelConfigs: [],
};

const MockDisabledProviderConfig: ChatProviderConfig = {
	...MockOpenAIProviderConfig,
	id: "prov-openai-disabled",
	display_name: "OpenAI Secondary",
	enabled: false,
};

export const mockProviderDisabledModel: ChatModelConfig = {
	...mockGPT5,
	id: "model-provider-disabled",
	ai_provider_id: "prov-openai-disabled",
	model: "gpt-4o-secondary",
	display_name: "GPT-4o Secondary",
	is_default: false,
};

export const MockDisabledProviderState: ProviderState = {
	...MockOpenAIProviderState,
	key: "prov-openai-disabled",
	provider: "openai",
	label: "OpenAI Secondary",
	providerConfig: MockDisabledProviderConfig,
	modelConfigs: [mockProviderDisabledModel],
};

export const MockCopilotProviderState: ProviderState = {
	...MockOpenAIProviderState,
	key: "prov-copilot",
	provider: "copilot",
	label: "GitHub Copilot",
	providerConfig: {
		...MockOpenAIProviderConfig,
		id: "prov-copilot",
		provider: "copilot",
		display_name: "GitHub Copilot",
	},
	modelConfigs: [],
};

// A model whose provider row has been deleted. In production such models
// still appear in the top-level model list, but `deriveProviderStates`
// drops them from every providerState.modelConfigs. Stories should feed
// this fixture through `models` alone; do not add it to a provider state.
export const mockOrphanedModel: ChatModelConfig = {
	...mockGPT5,
	id: "model-orphaned",
	ai_provider_id: "prov-orphaned",
	model: "gpt-4o-orphaned",
	display_name: "Orphaned Model",
	is_default: false,
	enabled: true,
};
