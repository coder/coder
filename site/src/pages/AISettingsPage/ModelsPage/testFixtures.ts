import type {
	ChatModel,
	ChatModelProviderDescriptor,
} from "#/api/typesGenerated";
import type { ProviderState } from "#/modules/aiModels/providerStates";
import { MockChatModelProviderDescriptor } from "#/testHelpers/chatModels";

const now = "2026-02-18T12:00:00.000Z";

const MockOpenAIProviderDescriptor: ChatModelProviderDescriptor = {
	...MockChatModelProviderDescriptor,
	id: "prov-openai",
};

const MockAnthropicProviderDescriptor: ChatModelProviderDescriptor = {
	...MockOpenAIProviderDescriptor,
	id: "prov-anthropic",
	type: "anthropic",
	display_name: "Anthropic",
};

export const mockGPT5: ChatModel = {
	id: "model-gpt5",
	organization_id: "org-1",
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

export const mockClaude: ChatModel = {
	...mockGPT5,
	id: "model-claude",
	ai_provider_id: "prov-anthropic",
	model: "claude-sonnet-4-5",
	display_name: "Claude Sonnet 4.5",
	is_default: false,
};

export const mockDisabledModel: ChatModel = {
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
	providerDescriptor: MockOpenAIProviderDescriptor,
	models: [mockGPT5, mockDisabledModel],
	catalogModelCount: 0,
	hasEffectiveAPIKey: true,
	allowUserAPIKey: false,
};

export const MockAnthropicProviderState: ProviderState = {
	...MockOpenAIProviderState,
	key: "prov-anthropic",
	provider: "anthropic",
	label: "Anthropic",
	providerDescriptor: MockAnthropicProviderDescriptor,
	models: [mockClaude],
};

const MockGoogleProviderDescriptor: ChatModelProviderDescriptor = {
	...MockOpenAIProviderDescriptor,
	id: "prov-google",
	type: "google",
	display_name: "Google",
};

export const MockGoogleProviderState: ProviderState = {
	...MockOpenAIProviderState,
	key: "prov-google",
	provider: "google",
	label: "Google",
	providerDescriptor: MockGoogleProviderDescriptor,
	models: [],
};

const MockBedrockProviderDescriptor: ChatModelProviderDescriptor = {
	...MockOpenAIProviderDescriptor,
	id: "prov-bedrock",
	type: "bedrock",
	display_name: "AWS Bedrock",
};

export const mockBedrockClaude: ChatModel = {
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
	providerDescriptor: MockBedrockProviderDescriptor,
	models: [mockBedrockClaude],
};

export const MockAzureProviderState: ProviderState = {
	...MockOpenAIProviderState,
	key: "prov-azure",
	provider: "azure",
	label: "Azure OpenAI",
	providerDescriptor: {
		...MockOpenAIProviderDescriptor,
		id: "prov-azure",
		type: "azure",
		display_name: "Azure OpenAI",
	},
	models: [],
};

const MockDisabledProviderDescriptor: ChatModelProviderDescriptor = {
	...MockOpenAIProviderDescriptor,
	id: "prov-openai-disabled",
	display_name: "OpenAI Secondary",
	enabled: false,
};

export const mockProviderDisabledModel: ChatModel = {
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
	providerDescriptor: MockDisabledProviderDescriptor,
	models: [mockProviderDisabledModel],
};

export const MockCopilotProviderState: ProviderState = {
	...MockOpenAIProviderState,
	key: "prov-copilot",
	provider: "copilot",
	label: "GitHub Copilot",
	providerDescriptor: {
		...MockOpenAIProviderDescriptor,
		id: "prov-copilot",
		type: "copilot",
		display_name: "GitHub Copilot",
	},
	models: [],
};

export const mockOrphanedModel: ChatModel = {
	...mockGPT5,
	id: "model-orphaned",
	ai_provider_id: "prov-orphaned",
	model: "gpt-4o-orphaned",
	display_name: "Orphaned Model",
	is_default: false,
	enabled: true,
};
