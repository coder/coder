import type {
	AIModelPrice,
	ChatModel,
	ChatModelProvider,
	ChatProviderConfig,
} from "#/api/typesGenerated";
import { MOCK_TIMESTAMP } from "./chatEntities";

export const MockChatModel: ChatModel = {
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

export const MockChatModelProvider: ChatModelProvider = {
	provider: "openai",
	available: true,
	models: [],
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
