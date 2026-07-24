import type { ChatModelConfig, ChatProviderConfig } from "#/api/typesGenerated";
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
