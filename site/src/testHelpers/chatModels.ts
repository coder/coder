import type {
	AIModelPrice,
	ChatModel,
	ChatModelProviderDescriptor,
	ChatPersonalModelOverride,
	ChatProviderConfig,
	UserChatPersonalModelOverridesResponse,
} from "#/api/typesGenerated";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import { MOCK_TIMESTAMP } from "./chatEntities";
import { MockDefaultOrganization } from "./entities";

export const MockChatModel: ChatModel = {
	organization_id: "00000000-0000-0000-0000-000000000000",
	id: "model-1",
	ai_provider_id: "provider-1",
	model: "gpt-5",
	display_name: "gpt-5",
	enabled: true,
	is_default: false,
	context_limit: 200000,
	compression_threshold: 70,
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

export const MockDefaultChatModel: ChatModel = {
	...MockChatModel,
	id: "model-config-1",
	organization_id: MockDefaultOrganization.id,
	is_default: true,
};

export const MockPersonalDefaultChatModel: ChatModel = {
	...MockChatModel,
	id: "model-gpt-4.1-mini",
	model: "gpt-4.1-mini",
	display_name: "GPT 4.1 Mini",
	context_limit: 1_000_000,
	is_default: true,
};

export const MockPersonalClaudeChatModel: ChatModel = {
	...MockPersonalDefaultChatModel,
	id: "model-claude-sonnet-4",
	ai_provider_id: "provider-anthropic",
	model: "claude-sonnet-4",
	display_name: "Claude Sonnet 4",
	context_limit: 200_000,
	is_default: false,
};

export const MockPersonalReasoningChatModel: ChatModel = {
	...MockPersonalDefaultChatModel,
	id: "model-gpt-5",
	model: "gpt-5",
	display_name: "GPT-5",
	is_default: false,
	model_config: {
		reasoning_effort: { default: "medium", max: "high" },
	},
	reasoning_efforts: ["none", "minimal", "low", "medium", "high"],
};

export const MockPersonalModelOptions: ModelSelectorOption[] = [
	{
		id: MockPersonalDefaultChatModel.id,
		provider: "openai",
		model: MockPersonalDefaultChatModel.model,
		displayName: MockPersonalDefaultChatModel.display_name,
		contextLimit: MockPersonalDefaultChatModel.context_limit,
	},
	{
		id: MockPersonalClaudeChatModel.id,
		provider: "anthropic",
		model: MockPersonalClaudeChatModel.model,
		displayName: MockPersonalClaudeChatModel.display_name,
		contextLimit: MockPersonalClaudeChatModel.context_limit,
	},
	{
		id: MockPersonalReasoningChatModel.id,
		provider: "openai",
		model: MockPersonalReasoningChatModel.model,
		displayName: MockPersonalReasoningChatModel.display_name,
		contextLimit: MockPersonalReasoningChatModel.context_limit,
		reasoningEffortDefault: "medium",
		reasoningEfforts: ["none", "minimal", "low", "medium", "high"],
	},
];

export const MockChatProviderConfig: ChatProviderConfig = {
	id: "provider-1",
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
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

export const MockChatModelProviderDescriptor: ChatModelProviderDescriptor = {
	id: "provider-1",
	type: "openai",
	display_name: "OpenAI",
	icon: "",
	enabled: true,
	has_api_key: true,
	has_user_api_key: false,
	has_effective_api_key: true,
	allow_user_api_key: false,
	available: true,
};

const unsetChatPersonalModelOverride = (
	context: ChatPersonalModelOverride["context"],
): ChatPersonalModelOverride => ({
	context,
	// The API reports chat_default for an unset root override.
	mode: context === "root" ? "chat_default" : "deployment_default",
	model_config_id: "",
	is_set: false,
});

export const MockUnsetUserChatPersonalModelOverrides: UserChatPersonalModelOverridesResponse =
	{
		enabled: true,
		root: unsetChatPersonalModelOverride("root"),
		general: unsetChatPersonalModelOverride("general"),
		explore: unsetChatPersonalModelOverride("explore"),
		deployment_defaults: {
			general: { context: "general", model_config_id: "" },
			explore: { context: "explore", model_config_id: "" },
		},
	};

// Prices are micro-units per million tokens.
export const MockGPT5ModelPrice: AIModelPrice = {
	provider: "openai",
	model: "gpt-5",
	input_price: 1250000,
	output_price: 10000000,
	cache_read_price: 125000,
	cache_write_price: null,
	source: "default",
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

// An input price below $0.0001 per million tokens renders as a threshold
// rather than an exact value.
export const MockGPT5BelowThresholdModelPrice: AIModelPrice = {
	...MockGPT5ModelPrice,
	input_price: 50,
	output_price: null,
	cache_read_price: null,
};
